// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

//go:build integration

package mcoa

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	ocinfrav1 "github.com/openshift/api/config/v1"
	cmomanifests "github.com/openshift/cluster-monitoring-operator/pkg/manifests"
	prometheusv1alpha1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1alpha1"
	"github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/controllers/observabilityendpoint"
	operatorconfig "github.com/stolostron/multicluster-observability-operator/operators/pkg/config"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/apimachinery/pkg/types"
	kubescheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	yamltool "sigs.k8s.io/yaml"
)

var (
	testEnv   *envtest.Environment
	cfg       *rest.Config
	s         = runtime.NewScheme()
	namespace = "test-mcoa-agent-integration"
)

func TestMain(m *testing.M) {
	opts := zap.Options{
		Development: true,
	}
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	cvCRD := readCRDFiles(
		filepath.Join("..", "observabilityendpoint", "testdata", "crd", "clusterversions-crd.yaml"),
		filepath.Join("crds", "monitoring.rhobs_scrapeconfigs.yaml"),
		filepath.Join("crds", "monitoring.rhobs_prometheusagents.yaml"),
	)

	testEnv = &envtest.Environment{
		CRDs: cvCRD,
	}

	var err error
	cfg, err = testEnv.Start()
	if err != nil {
		panic(fmt.Sprintf("Failed to start test environment: %v", err))
	}

	code := m.Run()

	err = testEnv.Stop()
	if err != nil {
		panic(fmt.Sprintf("Failed to stop test environment: %v", err))
	}

	os.Exit(code)
}

func readCRDFiles(crdPaths ...string) []*apiextensionsv1.CustomResourceDefinition {
	ret := []*apiextensionsv1.CustomResourceDefinition{}

	for _, crdPath := range crdPaths {
		crdYamlData, err := os.ReadFile(crdPath)
		if err != nil {
			panic(fmt.Sprintf("Failed to read CRD file: %v", err))
		}

		dec := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
		var crd apiextensionsv1.CustomResourceDefinition
		_, _, err = dec.Decode(crdYamlData, nil, &crd)
		if err != nil {
			panic(fmt.Sprintf("Failed to decode CRD: %v", err))
		}

		ret = append(ret, &crd)
	}

	return ret
}

func TestMCOAAgentIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	require.NoError(t, kubescheme.AddToScheme(s))
	require.NoError(t, ocinfrav1.AddToScheme(s))
	require.NoError(t, apiextensionsv1.AddToScheme(s))

	// Register ScrapeConfig for the custom monitoring.rhobs/v1alpha1 API Group
	addRhobsToScheme(t, s)

	hubInfo := &operatorconfig.HubInfo{
		AlertmanagerEndpoint: "https://hub-am.example.com",
		HubClusterID:         "hub-id",
	}

	// Initialize Manager with surgical filtered cache configuration identical to main.go
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: s,
		Cache:  GetCacheOptions(),
	})
	require.NoError(t, err)

	caSecretName := "hub-alertmanager-router-ca"
	if hubInfo.HubClusterID != "" {
		caSecretName = "hub-alertmanager-router-ca-" + hubInfo.HubClusterID
	}

	// Default active production-like reconciler configuration
	reconciler := NewMCOAAgentReconciler(
		mgr.GetClient(),
		mgr.GetLogger(),
		mgr.GetScheme(),
		mgr.GetEventRecorder("mcoa-agent-test"),
		namespace,
		"test-cluster-id",
		"test-cluster-name",
		hubInfo.AlertmanagerEndpoint, // HubAlertmanagerURL
		caSecretName,
		"obs-alertmanager-mtls-cert",
		"observability-alertmanager-accessor",
		true, // enablePlatformAlertForwarding
		true, // enableUWLAlertForwarding
	)
	err = reconciler.SetupWithManager(mgr)
	require.NoError(t, err)

	directClient, err := client.New(cfg, client.Options{Scheme: s})
	require.NoError(t, err)

	// Deploy standard OBO CRDs first to ensure watched types exist before manager start
	require.NoError(t, DeployCRDs(ctx, directClient))

	// Wait for CRDs to be fully established on the API server
	require.Eventually(t, func() bool {
		for _, name := range []string{"scrapeconfigs.monitoring.rhobs", "prometheusagents.monitoring.rhobs"} {
			crd := &apiextensionsv1.CustomResourceDefinition{}
			err := directClient.Get(ctx, types.NamespacedName{Name: name}, crd)
			if err != nil {
				return false
			}
			established := false
			for _, cond := range crd.Status.Conditions {
				if cond.Type == apiextensionsv1.Established && cond.Status == apiextensionsv1.ConditionTrue {
					established = true
					break
				}
			}
			if !established {
				return false
			}
		}
		return true
	}, 10*time.Second, 100*time.Millisecond)

	mgrCtx, mgrCancel := context.WithCancel(ctx)
	mgrStopped := make(chan struct{})
	go func() {
		defer close(mgrStopped)
		if err := mgr.Start(mgrCtx); err != nil && mgrCtx.Err() == nil {
			fmt.Printf("Manager failed: %v\n", err)
		}
	}()

	// Wait for cache to be fully synced and started before executing any cached client queries
	require.True(t, mgr.GetCache().WaitForCacheSync(ctx), "Failed to sync cache before running tests")

	k8sClient := mgr.GetClient()

	// Setup required platform resources
	setupResources := []client.Object{
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: operatorconfig.OCPClusterMonitoringNamespace}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: operatorconfig.OCPUserWorkloadMonitoringNamespace}},
		&ocinfrav1.ClusterVersion{
			ObjectMeta: metav1.ObjectMeta{Name: "version"},
			Spec:       ocinfrav1.ClusterVersionSpec{ClusterID: "test-cluster-id"},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      observabilityendpoint.AppendHubClusterID(observabilityendpoint.HubAmAccessorSecretName, hubInfo.HubClusterID),
				Namespace: operatorconfig.OCPClusterMonitoringNamespace,
			},
			Data: map[string][]byte{"token": []byte("test-token")},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      observabilityendpoint.AppendHubClusterID(observabilityendpoint.HubAmRouterCASecretName, hubInfo.HubClusterID),
				Namespace: operatorconfig.OCPClusterMonitoringNamespace,
			},
			Data: map[string][]byte{"service-ca.crt": []byte("test-ca")},
		},
		newTestPrometheusAgent("test-agent-platform", namespace, platformMetricsCollectorComponent, "https://hub-am.example.com", caSecretName, "test-cert-secret"),
		newTestPrometheusAgent("test-agent-uwl", namespace, userWorkloadMetricsCollectorComponent, "https://hub-am.example.com", caSecretName, "test-cert-secret"),
	}
	for _, obj := range setupResources {
		err = directClient.Create(ctx, obj)
		if err != nil && !apierrors.IsAlreadyExists(err) {
			require.NoError(t, err)
		}
	}

	t.Run("Surgical cache filter: Unmanaged ConfigMap is invisible to the cached client", func(t *testing.T) {
		unmanagedName := "unmanaged-cm"
		unmanagedCM := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: unmanagedName, Namespace: operatorconfig.OCPClusterMonitoringNamespace},
			Data:       map[string]string{"foo": "bar"},
		}
		require.NoError(t, directClient.Create(ctx, unmanagedCM))

		found := &corev1.ConfigMap{}
		err = k8sClient.Get(ctx, types.NamespacedName{Name: unmanagedName, Namespace: operatorconfig.OCPClusterMonitoringNamespace}, found)
		require.Error(t, err)
		require.True(t, apierrors.IsNotFound(err))
	})

	t.Run("Reconcile CMO Config: Managed ConfigMap is successfully populated with Alertmanager configuration", func(t *testing.T) {
		cmoCM := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      operatorconfig.OCPClusterMonitoringConfigMapName,
				Namespace: operatorconfig.OCPClusterMonitoringNamespace,
			},
			Data: map[string]string{
				observabilityendpoint.ClusterMonitoringConfigDataKey: "prometheusK8s: {}",
			},
		}
		err := directClient.Create(ctx, cmoCM, client.FieldOwner(observabilityendpoint.EndpointMonitoringOperatorMgr))
		if apierrors.IsAlreadyExists(err) {
			existing := &corev1.ConfigMap{}
			require.NoError(t, directClient.Get(ctx, types.NamespacedName{
				Name:      operatorconfig.OCPClusterMonitoringConfigMapName,
				Namespace: operatorconfig.OCPClusterMonitoringNamespace,
			}, existing))
			existing.Data = map[string]string{
				observabilityendpoint.ClusterMonitoringConfigDataKey: "prometheusK8s: {}",
			}
			require.NoError(t, directClient.Update(ctx, existing))
		} else {
			require.NoError(t, err)
		}

		// Wait for the reconciler to populate our Alertmanager configs
		require.Eventually(t, func() bool {
			found := &corev1.ConfigMap{}
			err := directClient.Get(ctx, types.NamespacedName{
				Name:      operatorconfig.OCPClusterMonitoringConfigMapName,
				Namespace: operatorconfig.OCPClusterMonitoringNamespace,
			}, found)
			if err != nil {
				return false
			}
			data := found.Data[observabilityendpoint.ClusterMonitoringConfigDataKey]
			return strings.Contains(data, "hub-alertmanager-router-ca-hub-id") &&
				strings.Contains(data, "hub-am.example.com") &&
				strings.Contains(data, "managed_cluster_name: test-cluster-name")
		}, 5*time.Second, 100*time.Millisecond)
	})

	t.Run("Reconcile UWL Config: Managed ConfigMap is successfully populated with Alertmanager configuration", func(t *testing.T) {
		uwlCM := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      operatorconfig.OCPUserWorkloadMonitoringConfigMap,
				Namespace: operatorconfig.OCPUserWorkloadMonitoringNamespace,
			},
			Data: map[string]string{
				uwlMonitoringConfigDataKey: defaultUWLConfigYAML,
			},
		}
		err := directClient.Create(ctx, uwlCM, client.FieldOwner(observabilityendpoint.EndpointMonitoringOperatorMgr))
		if apierrors.IsAlreadyExists(err) {
			existing := &corev1.ConfigMap{}
			require.NoError(t, directClient.Get(ctx, types.NamespacedName{
				Name:      operatorconfig.OCPUserWorkloadMonitoringConfigMap,
				Namespace: operatorconfig.OCPUserWorkloadMonitoringNamespace,
			}, existing))
			existing.Data = map[string]string{
				uwlMonitoringConfigDataKey: defaultUWLConfigYAML,
			}
			require.NoError(t, directClient.Update(ctx, existing))
		} else {
			require.NoError(t, err)
		}

		// Wait for the reconciler to populate our Alertmanager configs in UWL ConfigMap
		require.Eventually(t, func() bool {
			found := &corev1.ConfigMap{}
			err := directClient.Get(ctx, types.NamespacedName{
				Name:      operatorconfig.OCPUserWorkloadMonitoringConfigMap,
				Namespace: operatorconfig.OCPUserWorkloadMonitoringNamespace,
			}, found)
			if err != nil {
				return false
			}
			data := found.Data[uwlMonitoringConfigDataKey]
			return strings.Contains(data, "hub-alertmanager-router-ca-hub-id") && strings.Contains(data, "hub-am.example.com")
		}, 5*time.Second, 100*time.Millisecond)
	})

	// Stop the background manager before the revert tests so it cannot interfere
	// with the direct Reconcile calls and the state assertions that follow.
	mgrCancel()
	<-mgrStopped

	t.Run("Reconcile Revert path: Empty AlertmanagerEndpoint cleanly reverts the Alertmanager configuration", func(t *testing.T) {
		// Instantiate a brand-new, private local reconciler with alert forwarding disabled (HubAlertmanagerURL = "") to simulate config update
		revertReconciler := NewMCOAAgentReconciler(
			directClient,
			mgr.GetLogger(),
			mgr.GetScheme(),
			mgr.GetEventRecorder("mcoa-agent-test"),
			namespace,
			"test-cluster-id",
			"test-cluster-name",
			"", // empty HubAlertmanagerURL disables forwarding
			caSecretName,
			"obs-alertmanager-mtls-cert",
			"observability-alertmanager-accessor",
			false, // enablePlatformAlertForwarding
			false, // enableUWLAlertForwarding
		)

		// Trigger reconcile by directly calling the Reconcile method on this private reconciler
		_, err = revertReconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      operatorconfig.OCPClusterMonitoringConfigMapName,
				Namespace: operatorconfig.OCPClusterMonitoringNamespace,
			},
		})
		require.NoError(t, err)

		foundCM := &corev1.ConfigMap{}
		err = directClient.Get(ctx, types.NamespacedName{
			Name:      operatorconfig.OCPClusterMonitoringConfigMapName,
			Namespace: operatorconfig.OCPClusterMonitoringNamespace,
		}, foundCM)
		if !apierrors.IsNotFound(err) {
			require.NoError(t, err)
			data := foundCM.Data[observabilityendpoint.ClusterMonitoringConfigDataKey]
			require.False(t, strings.Contains(data, "hub-alertmanager-router-ca-hub-id"), "expected AM CA secret to be removed from CMO CM after revert")
			require.False(t, strings.Contains(data, "hub-am.example.com"), "expected AM URL to be removed from CMO CM after revert")
		}
	})

	t.Run("Reconcile UWL Revert path: Disabled EnableUWLAlertForwarding cleanly reverts the Alertmanager configuration", func(t *testing.T) {
		// Pre-condition: the UWL CM must still carry the AM config at this point.
		// This proves that the CMO revert above only touched cluster-monitoring-config
		// and did not accidentally clean user-workload-monitoring-config as well.
		preCM := &corev1.ConfigMap{}
		require.NoError(t, directClient.Get(ctx, types.NamespacedName{
			Name:      operatorconfig.OCPUserWorkloadMonitoringConfigMap,
			Namespace: operatorconfig.OCPUserWorkloadMonitoringNamespace,
		}, preCM))
		preData := preCM.Data[uwlMonitoringConfigDataKey]
		require.True(t, strings.Contains(preData, "hub-alertmanager-router-ca-hub-id"), "pre-condition: UWL CM should still have AM config before the revert")
		require.True(t, strings.Contains(preData, "hub-am.example.com"), "pre-condition: UWL CM should still have AM config before the revert")

		// Instantiate a brand-new, private local reconciler with UWL alert forwarding disabled to simulate config update
		revertReconciler := NewMCOAAgentReconciler(
			directClient,
			mgr.GetLogger(),
			mgr.GetScheme(),
			mgr.GetEventRecorder("mcoa-agent-test"),
			namespace,
			"test-cluster-id",
			"test-cluster-name",
			"https://hub-am.example.com", // HubAlertmanagerURL
			caSecretName,
			"obs-alertmanager-mtls-cert",
			"observability-alertmanager-accessor",
			true,  // enablePlatformAlertForwarding
			false, // disabled UWL alert forwarding
		)

		// Trigger reconcile by directly calling the Reconcile method on this private reconciler
		_, err = revertReconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      operatorconfig.OCPUserWorkloadMonitoringConfigMap,
				Namespace: operatorconfig.OCPUserWorkloadMonitoringNamespace,
			},
		})
		require.NoError(t, err)

		foundCM := &corev1.ConfigMap{}
		err = directClient.Get(ctx, types.NamespacedName{
			Name:      operatorconfig.OCPUserWorkloadMonitoringConfigMap,
			Namespace: operatorconfig.OCPUserWorkloadMonitoringNamespace,
		}, foundCM)
		if !apierrors.IsNotFound(err) {
			require.NoError(t, err)
			data := foundCM.Data[uwlMonitoringConfigDataKey]
			require.False(t, strings.Contains(data, "hub-alertmanager-router-ca-hub-id"), "expected AM CA secret to be removed from UWL CM after revert")
			require.False(t, strings.Contains(data, "hub-am.example.com"), "expected AM URL to be removed from UWL CM after revert")
		}
	})

	// Restart with a fresh manager for the remaining tests. A controller-runtime manager
	// cannot be restarted after it is stopped, so we create a new instance.
	// Disable the metrics server to avoid a port conflict with the first manager.
	mgr2, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:  s,
		Cache:   GetCacheOptions(),
		Metrics: metricsserver.Options{BindAddress: "0"},
		// Controller name "configmap" is already registered in the process-global metrics
		// registry by the first manager; skip re-registration validation.
		Controller: config.Controller{SkipNameValidation: ptr.To(true)},
	})
	require.NoError(t, err)
	require.NoError(t, NewMCOAAgentReconciler(
		mgr2.GetClient(),
		mgr2.GetLogger(),
		mgr2.GetScheme(),
		mgr2.GetEventRecorder("mcoa-agent-test"),
		namespace,
		"test-cluster-id",
		"test-cluster-name",
		hubInfo.AlertmanagerEndpoint,
		caSecretName,
		"obs-alertmanager-mtls-cert",
		"observability-alertmanager-accessor",
		true, // enablePlatformAlertForwarding
		true, // enableUWLAlertForwarding
	).SetupWithManager(mgr2))
	go func() {
		if err := mgr2.Start(ctx); err != nil && ctx.Err() == nil {
			fmt.Printf("Manager2 failed: %v\n", err)
		}
	}()
	require.True(t, mgr2.GetCache().WaitForCacheSync(ctx), "Failed to sync cache for manager2")

	t.Run("Reconcile Raw Metrics ScrapeConfig: updates CMO and UWL with transpiled RemoteWrite", func(t *testing.T) {
		// Re-create the mock PrometheusAgents if they were wiped out by the CRD deletion test
		agentPlatform := newTestPrometheusAgent("test-agent-platform", namespace, platformMetricsCollectorComponent, "https://hub-am.example.com", caSecretName, "test-cert-secret")
		err := directClient.Create(ctx, agentPlatform)
		if err != nil && !apierrors.IsAlreadyExists(err) {
			require.NoError(t, err)
		}

		agentUWL := newTestPrometheusAgent("test-agent-uwl", namespace, userWorkloadMetricsCollectorComponent, "https://hub-am.example.com", caSecretName, "test-cert-secret")
		err = directClient.Create(ctx, agentUWL)
		if err != nil && !apierrors.IsAlreadyExists(err) {
			require.NoError(t, err)
		}

		// Create a platform-metrics-collector-raw ScrapeConfig
		scPlatform := newRawScrapeConfig("test-raw-platform", namespace, platformMetricsCollectorRawComponent)
		require.NoError(t, directClient.Create(ctx, scPlatform))

		// Wait for raw platform metrics RemoteWrite injection in CMO ConfigMap and assert exactly one spec with correct name exists
		require.Eventually(t, func() bool {
			found := &corev1.ConfigMap{}
			err := directClient.Get(ctx, types.NamespacedName{
				Name:      operatorconfig.OCPClusterMonitoringConfigMapName,
				Namespace: operatorconfig.OCPClusterMonitoringNamespace,
			}, found)
			if err != nil {
				return false
			}
			data := found.Data[observabilityendpoint.ClusterMonitoringConfigDataKey]
			if !strings.Contains(data, "https://hub-am.example.com") {
				return false
			}

			// Parse and assert exact length to prevent regression of duplication bugs
			parsed := &cmomanifests.ClusterMonitoringConfiguration{}
			if err := yamltool.Unmarshal([]byte(data), parsed); err != nil {
				return false
			}
			if parsed.PrometheusK8sConfig == nil || len(parsed.PrometheusK8sConfig.RemoteWrite) != 1 {
				return false
			}
			return strings.HasPrefix(parsed.PrometheusK8sConfig.RemoteWrite[0].Name, "mcoa-raw-test-raw-platform")
		}, 5*time.Second, 100*time.Millisecond)

		// Create a user-workload-metrics-collector-raw ScrapeConfig
		scUWL := newRawScrapeConfig("test-raw-uwl", namespace, userWorkloadMetricsCollectorRawComponent)
		require.NoError(t, directClient.Create(ctx, scUWL))

		// Wait for raw UWL metrics RemoteWrite injection in UWL ConfigMap and assert exactly one spec with correct name exists
		require.Eventually(t, func() bool {
			found := &corev1.ConfigMap{}
			err := directClient.Get(ctx, types.NamespacedName{
				Name:      operatorconfig.OCPUserWorkloadMonitoringConfigMap,
				Namespace: operatorconfig.OCPUserWorkloadMonitoringNamespace,
			}, found)
			if err != nil {
				return false
			}
			data := found.Data[uwlMonitoringConfigDataKey]
			if !strings.Contains(data, "https://hub-am.example.com") {
				return false
			}

			// Parse and assert exact length to prevent regression of duplication bugs
			parsed := &cmomanifests.UserWorkloadConfiguration{}
			if err := yamltool.Unmarshal([]byte(data), parsed); err != nil {
				return false
			}
			if parsed.Prometheus == nil || len(parsed.Prometheus.RemoteWrite) != 1 {
				return false
			}
			return strings.HasPrefix(parsed.Prometheus.RemoteWrite[0].Name, "mcoa-raw-test-raw-uwl")
		}, 5*time.Second, 100*time.Millisecond)
	})

	t.Run("Reconcile Raw Metrics ScrapeConfig Deletion: cleanly removes transpiled RemoteWrite from ConfigMap", func(t *testing.T) {
		// Fetch the platform ScrapeConfig we created in the previous subtest
		scPlatform := &prometheusv1alpha1.ScrapeConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-raw-platform",
				Namespace: namespace,
			},
		}
		require.NoError(t, directClient.Delete(ctx, scPlatform))

		// Wait for raw platform metrics RemoteWrite to be cleanly removed from CMO ConfigMap
		require.Eventually(t, func() bool {
			found := &corev1.ConfigMap{}
			err := directClient.Get(ctx, types.NamespacedName{
				Name:      operatorconfig.OCPClusterMonitoringConfigMapName,
				Namespace: operatorconfig.OCPClusterMonitoringNamespace,
			}, found)
			if err != nil {
				return false
			}
			data := found.Data[observabilityendpoint.ClusterMonitoringConfigDataKey]
			return !strings.Contains(data, "test-cert-secret")
		}, 5*time.Second, 100*time.Millisecond)

		// Fetch the UWL ScrapeConfig we created in the previous subtest
		scUWL := &prometheusv1alpha1.ScrapeConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-raw-uwl",
				Namespace: namespace,
			},
		}
		require.NoError(t, directClient.Delete(ctx, scUWL))

		// Wait for raw UWL metrics RemoteWrite to be cleanly removed from UWL ConfigMap
		require.Eventually(t, func() bool {
			found := &corev1.ConfigMap{}
			err := directClient.Get(ctx, types.NamespacedName{
				Name:      operatorconfig.OCPUserWorkloadMonitoringConfigMap,
				Namespace: operatorconfig.OCPUserWorkloadMonitoringNamespace,
			}, found)
			if err != nil {
				return false
			}
			data := found.Data[uwlMonitoringConfigDataKey]
			return !strings.Contains(data, "test-cert-secret")
		}, 5*time.Second, 100*time.Millisecond)
	})

	t.Run("CRD watch: deleting an OBO CRD triggers immediate restoration via the metadata-only watch", func(t *testing.T) {
		// Install the OBO CRDs so the watch has something to react to.
		require.NoError(t, DeployCRDs(ctx, directClient))

		target := "prometheusagents.monitoring.rhobs"

		crd := &apiextensionsv1.CustomResourceDefinition{}
		require.NoError(t, directClient.Get(ctx, types.NamespacedName{Name: target}, crd))

		// Delete it — the WatchesMetadata watch must fire and call DeployCRDs within seconds.
		require.NoError(t, directClient.Delete(ctx, crd))

		require.Eventually(t, func() bool {
			restored := &apiextensionsv1.CustomResourceDefinition{}
			if err := directClient.Get(ctx, types.NamespacedName{Name: target}, restored); err != nil {
				return false
			}
			return restored.Labels[ManagedByLabelKey] == ManagedByLabelValue
		}, 10*time.Second, 100*time.Millisecond, "CRD %s was not restored after deletion", target)
	})
}
