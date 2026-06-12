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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	testEnv *envtest.Environment
	cfg     *rest.Config
)

func TestMain(m *testing.M) {
	opts := zap.Options{
		Development: true,
	}
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	cvCRD := readCRDFiles(
		filepath.Join("..", "observabilityendpoint", "testdata", "crd", "clusterversions-crd.yaml"),
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
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()

	s := runtime.NewScheme()
	require.NoError(t, kubescheme.AddToScheme(s))
	require.NoError(t, ocinfrav1.AddToScheme(s))

	namespace := "test-mcoa-agent"
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

	reconciler := NewMCOAAgentReconciler(
		mgr.GetClient(),
		mgr.GetLogger(),
		mgr.GetScheme(),
		mgr.GetEventRecorderFor("mcoa-agent-test"),
		namespace,
		"test-cluster-id",
		hubInfo.AlertmanagerEndpoint,
		caSecretName,
		"obs-alertmanager-mtls-cert",
		"observability-alertmanager-accessor",
		true,
	)
	err = reconciler.SetupWithManager(mgr)
	require.NoError(t, err)

	go func() {
		if err := mgr.Start(ctx); err != nil {
			fmt.Printf("Manager failed: %v\n", err)
		}
	}()

	k8sClient := mgr.GetClient()
	directClient, err := client.New(cfg, client.Options{Scheme: s})
	require.NoError(t, err)

	// Setup required platform resources
	setupResources := []client.Object{
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
		require.NoError(t, directClient.Create(ctx, cmoCM, client.FieldOwner(observabilityendpoint.EndpointMonitoringOperatorMgr)))

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
			return strings.Contains(data, "hub-alertmanager-router-ca-hub-id") && strings.Contains(data, "hub-am.example.com")
		}, 5*time.Second, 100*time.Millisecond)
	})

	t.Run("Reconcile Revert path: Empty AlertmanagerEndpoint cleanly reverts the Alertmanager configuration", func(t *testing.T) {
		// Disable alert forwarding on the active reconciler
		reconciler.AlertmanagerEndpoint = ""

		// Trigger reconcile by directly calling the Reconcile method on the reconciler, emulating the real flow.
		_, err = reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      operatorconfig.OCPClusterMonitoringConfigMapName,
				Namespace: operatorconfig.OCPClusterMonitoringNamespace,
			},
		})
		require.NoError(t, err)

		// Wait for the reconciler to clean up our Alertmanager configs (or cleanly delete the empty ConfigMap)
		require.Eventually(t, func() bool {
			foundCM := &corev1.ConfigMap{}
			err := directClient.Get(ctx, types.NamespacedName{
				Name:      operatorconfig.OCPClusterMonitoringConfigMapName,
				Namespace: operatorconfig.OCPClusterMonitoringNamespace,
			}, foundCM)
			if apierrors.IsNotFound(err) {
				return true
			}
			if err != nil {
				fmt.Printf("Get ConfigMap error: %v\n", err)
				return false
			}
			data := foundCM.Data[observabilityendpoint.ClusterMonitoringConfigDataKey]
			return !strings.Contains(data, "hub-alertmanager-router-ca-hub-id") && !strings.Contains(data, "hub-am.example.com")
		}, 5*time.Second, 100*time.Millisecond)
	})

	t.Run("Reconcile UWL Config: Managed ConfigMap is successfully populated with Alertmanager configuration", func(t *testing.T) {
		// Set AlertmanagerEndpoint and EnableUWLAlertForwarding back to active state
		reconciler.AlertmanagerEndpoint = "https://hub-am.example.com"
		reconciler.EnableUWLAlertForwarding = true

		uwlCM := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      operatorconfig.OCPUserWorkloadMonitoringConfigMap,
				Namespace: operatorconfig.OCPUserWorkloadMonitoringNamespace,
			},
			Data: map[string]string{
				"config.yaml": "prometheus: {}",
			},
		}
		require.NoError(t, directClient.Create(ctx, uwlCM, client.FieldOwner(observabilityendpoint.EndpointMonitoringOperatorMgr)))

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
			data := found.Data["config.yaml"]
			return strings.Contains(data, "hub-alertmanager-router-ca-hub-id") && strings.Contains(data, "hub-am.example.com")
		}, 5*time.Second, 100*time.Millisecond)
	})

	t.Run("Reconcile UWL Revert path: Disabled EnableUWLAlertForwarding cleanly reverts the Alertmanager configuration", func(t *testing.T) {
		// Disable UWL alert forwarding on the active reconciler
		reconciler.EnableUWLAlertForwarding = false

		// Trigger reconcile by directly calling the Reconcile method on the reconciler, emulating the real flow.
		_, err = reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      operatorconfig.OCPUserWorkloadMonitoringConfigMap,
				Namespace: operatorconfig.OCPUserWorkloadMonitoringNamespace,
			},
		})
		require.NoError(t, err)

		// Wait for the reconciler to clean up our Alertmanager configs from UWL ConfigMap
		require.Eventually(t, func() bool {
			foundCM := &corev1.ConfigMap{}
			err := directClient.Get(ctx, types.NamespacedName{
				Name:      operatorconfig.OCPUserWorkloadMonitoringConfigMap,
				Namespace: operatorconfig.OCPUserWorkloadMonitoringNamespace,
			}, foundCM)
			if apierrors.IsNotFound(err) {
				return true
			}
			if err != nil {
				fmt.Printf("Get ConfigMap error: %v\n", err)
				return false
			}
			data := foundCM.Data["config.yaml"]
			return !strings.Contains(data, "hub-alertmanager-router-ca-hub-id") && !strings.Contains(data, "hub-am.example.com")
		}, 5*time.Second, 100*time.Millisecond)
	})
}
