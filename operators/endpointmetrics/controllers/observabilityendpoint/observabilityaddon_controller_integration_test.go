// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

//go:build integration

package observabilityendpoint

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	yaml2 "github.com/ghodss/yaml"
	ocinfrav1 "github.com/openshift/api/config/v1"
	cmomanifests "github.com/openshift/cluster-monitoring-operator/pkg/manifests"
	hyperv1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/pkg/hypershift"
	"github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/pkg/util"
	observabilityshared "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/shared"
	oav1beta1 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	"github.com/stolostron/multicluster-observability-operator/operators/pkg/rendering/templates"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	kubescheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

var (
	testEnvSpoke *envtest.Environment
	restCfgSpoke *rest.Config
	testEnvHub   *envtest.Environment
	restCfgHub   *rest.Config
)

func TestCMOConfigWatching(t *testing.T) {
	namespace := "test-cmo-config"

	scheme := createBaseScheme()
	ocinfrav1.AddToScheme(scheme)

	k8sClient, err := client.New(restCfgHub, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatal(err)
	}

	defer tearDownCommonHubResources(t, k8sClient, namespace)

	// Create resources required for the cmo case
	resourcesDeps := []client.Object{
		makeNamespace(promNamespace),
		makeNamespace(namespace),
		newImagesCM(namespace),
		newHubInfoSecret([]byte(`
endpoint: "http://test-endpoint"
alertmanager-endpoint: "http://test-alertamanger-endpoint"
alertmanager-router-ca: |
    -----BEGIN CERTIFICATE-----
    xxxxxxxxxxxxxxxxxxxxxxxxxxx
    -----END CERTIFICATE-----
`), namespace),
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      promSvcName,
				Namespace: promNamespace,
			},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{{Name: "metrics", Port: 9090}},
			},
			Status: corev1.ServiceStatus{},
		},
		&ocinfrav1.ClusterVersion{
			ObjectMeta: metav1.ObjectMeta{
				Name: "version",
			},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      hubAmAccessorSecretName,
				Namespace: namespace,
			},
			Immutable:  nil,
			Data:       nil,
			StringData: map[string]string{hubAmAccessorSecretKey: "lol"},
		},
		&oav1beta1.ObservabilityAddon{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "observability-addon",
				Namespace: namespace,
			},
		},
	}
	if err := createResources(k8sClient, resourcesDeps...); err != nil {
		t.Fatalf("Failed to create resources: %v", err)
	}

	mgr, err := ctrl.NewManager(testEnvHub.Config, ctrl.Options{
		Scheme:  k8sClient.Scheme(),
		Metrics: metricsserver.Options{BindAddress: "0"}, // Avoids port conflict with the default port 8080
	})
	assert.NoError(t, err)

	hubClientWithReload, err := util.NewReloadableHubClientWithReloadFunc(func() (client.Client, error) {
		return k8sClient, nil
	})
	assert.NoError(t, err)
	reconciler := ObservabilityAddonReconciler{
		Client:                k8sClient,
		HubClient:             hubClientWithReload,
		IsHubMetricsCollector: true,
		Scheme:                scheme,
		Namespace:             namespace,
		HubNamespace:          "local-cluster",
		ServiceAccountName:    "endpoint-monitoring-operator",
		InstallPrometheus:     false,
	}

	err = reconciler.SetupWithManager(mgr)
	assert.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		err = mgr.Start(ctx)
		assert.NoError(t, err)
	}()

	cm := &corev1.ConfigMap{}
	err = wait.Poll(1*time.Second, time.Minute, func() (bool, error) {
		err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: promNamespace, Name: clusterMonitoringConfigName}, cm)
		if err != nil && errors.IsNotFound(err) {
			return false, nil
		}

		return true, err
	})
	assert.NoError(t, err)

	foundClusterMonitoringConfiguration := &cmomanifests.ClusterMonitoringConfiguration{}
	err = yaml2.Unmarshal([]byte(cm.Data[clusterMonitoringConfigDataKey]), foundClusterMonitoringConfiguration)
	assert.NoError(t, err)

	assert.Len(t, foundClusterMonitoringConfiguration.PrometheusK8sConfig.AlertmanagerConfigs, 1)
	assert.Equal(t, foundClusterMonitoringConfiguration.PrometheusK8sConfig.AlertmanagerConfigs[0].Scheme, "https")

	foundClusterMonitoringConfiguration.PrometheusK8sConfig.AlertmanagerConfigs[0].Scheme = "http"
	foundClusterMonitoringConfiguration.PrometheusK8sConfig.Retention = "infinity-and-beyond"

	b, err := yaml2.Marshal(foundClusterMonitoringConfiguration)
	assert.NoError(t, err)
	cm.Data[clusterMonitoringConfigDataKey] = string(b)
	err = k8sClient.Update(context.Background(), cm)
	assert.NoError(t, err)

	// repeat the test and expect a partial revert
	err = wait.Poll(1*time.Second, time.Minute, func() (bool, error) {
		updated := &corev1.ConfigMap{}
		err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: promNamespace, Name: clusterMonitoringConfigName}, updated)
		if err != nil && errors.IsNotFound(err) {
			return false, nil
		}

		foundUpdatedClusterMonitoringConfiguration := &cmomanifests.ClusterMonitoringConfiguration{}
		err = yaml2.Unmarshal([]byte(updated.Data[clusterMonitoringConfigDataKey]), foundUpdatedClusterMonitoringConfiguration)
		if err != nil {
			return false, nil
		}

		if foundUpdatedClusterMonitoringConfiguration.PrometheusK8sConfig.AlertmanagerConfigs[0].Scheme != "https" {
			return false, nil
		}

		if foundUpdatedClusterMonitoringConfiguration.PrometheusK8sConfig.Retention != "infinity-and-beyond" {
			return false, nil
		}

		return true, err
	})
	assert.NoError(t, err)

}

// TestIntegrationReconcileHypershift tests the reconcile function for hypershift CRDs.
func TestIntegrationReconcileHypershift(t *testing.T) {
	testNamespace := "test-ns"

	scheme := createBaseScheme()
	hyperv1.AddToScheme(scheme)

	k8sClient, err := client.New(restCfgHub, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatal(err)
	}

	setupCommonHubResources(t, k8sClient, testNamespace)
	defer tearDownCommonHubResources(t, k8sClient, testNamespace)

	hostedClusterNs := "hosted-cluster-ns"
	hostedClusterName := "myhostedcluster"
	hostedCluster := newHostedCluster(hostedClusterName, hostedClusterNs)

	// Create resources required for the hypershift case
	resourcesDeps := []client.Object{
		makeNamespace(hostedClusterNs),
		makeNamespace(hypershift.HostedClusterNamespace(hostedCluster)),
		hostedCluster,
		newServiceMonitor(hypershift.EtcdSmName, hypershift.HostedClusterNamespace(hostedCluster)),
		newServiceMonitor(hypershift.ApiServerSmName, hypershift.HostedClusterNamespace(hostedCluster)),
	}
	if err := createResources(k8sClient, resourcesDeps...); err != nil {
		t.Fatalf("Failed to create resources: %v", err)
	}

	mgr, err := ctrl.NewManager(testEnvHub.Config, ctrl.Options{
		Scheme:  k8sClient.Scheme(),
		Metrics: metricsserver.Options{BindAddress: "0"}, // Avoids port conflict with the default port 8080
	})
	assert.NoError(t, err)

	hubClientWithReload, err := util.NewReloadableHubClientWithReloadFunc(func() (client.Client, error) {
		return k8sClient, nil
	})
	assert.NoError(t, err)
	reconciler := ObservabilityAddonReconciler{
		Client:                k8sClient,
		HubClient:             hubClientWithReload,
		IsHubMetricsCollector: true,
		Scheme:                scheme,
		Namespace:             testNamespace,
		HubNamespace:          "local-cluster",
		ServiceAccountName:    "endpoint-monitoring-operator",
		InstallPrometheus:     false,
	}

	err = reconciler.SetupWithManager(mgr)
	assert.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		err = mgr.Start(ctx)
		assert.NoError(t, err)
	}()

	// Hypershift service monitors must be created
	err = wait.Poll(1*time.Second, 5*time.Second, func() (bool, error) {
		hypershiftEtcdSm := &promv1.ServiceMonitor{}
		err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: hostedClusterNs + "-" + hostedClusterName, Name: hypershift.AcmEtcdSmName}, hypershiftEtcdSm)
		if err != nil && errors.IsNotFound(err) {
			return false, nil
		}

		return true, err
	})
	assert.NoError(t, err)
}

// TestIntegrationReconcileHypershift tests the reconcile function for hypershift CRDs.
func TestIntegrationReconcileMicroshift(t *testing.T) {
	testNamespace := "open-cluster-management-addon-observability"
	hubNamespace := "microshift-cluster-a"

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working dir %v", err)
	}
	os.Setenv(templates.TemplatesPathEnvVar, filepath.Join(filepath.Dir(filepath.Dir(wd)), "manifests"))

	scheme := createBaseScheme()

	k8sClientSpoke, err := client.New(restCfgSpoke, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatal(err)
	}

	setupCommonSpokeResources(t, k8sClientSpoke)
	defer tearDownCommonSpokeResources(t, k8sClientSpoke)

	resourcesDeps := []client.Object{
		newObservabilityAddonBis("observability-addon", testNamespace),
		newMicroshiftVersionCM("kube-public"),
		newMetricsAllowlistCM(testNamespace),
	}
	if err := createResources(k8sClientSpoke, resourcesDeps...); err != nil {
		t.Fatalf("Failed to create resources: %v", err)
	}

	k8sHubClient, err := client.New(restCfgHub, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatal(err)
	}

	setupCommonHubResources(t, k8sHubClient, testNamespace)
	defer tearDownCommonHubResources(t, k8sHubClient, testNamespace)

	// Create resources required for the microshift case on the hub
	resourcesDeps = []client.Object{
		makeNamespace(hubNamespace),
		newObservabilityAddonBis("observability-addon", hubNamespace),
	}
	if err := createResources(k8sHubClient, resourcesDeps...); err != nil {
		t.Fatalf("Failed to create resources on hub: %v", err)
	}

	mgr, err := ctrl.NewManager(testEnvSpoke.Config, ctrl.Options{
		Scheme:  k8sClientSpoke.Scheme(),
		Metrics: metricsserver.Options{BindAddress: "0"}, // Avoids port conflict with the default port 8080
	})
	assert.NoError(t, err)

	hubClientWithReload, err := util.NewReloadableHubClientWithReloadFunc(func() (client.Client, error) {
		return k8sHubClient, nil
	})
	assert.NoError(t, err)
	reconciler := ObservabilityAddonReconciler{
		Client:                k8sClientSpoke,
		HubClient:             hubClientWithReload,
		Scheme:                scheme,
		IsHubMetricsCollector: false,
		Namespace:             testNamespace,
		HubNamespace:          hubNamespace,
		ServiceAccountName:    "endpoint-monitoring-operator",
		InstallPrometheus:     true,
	}

	err = reconciler.SetupWithManager(mgr)
	assert.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		err = mgr.Start(ctx)
		assert.NoError(t, err)
	}()

	// Microshift resources must be created
	// Checking the etcd service monitor that is specific to microshift
	err = wait.Poll(1*time.Second, 5*time.Second, func() (bool, error) {
		etcdSm := &promv1.ServiceMonitor{}
		err := k8sClientSpoke.Get(context.Background(), types.NamespacedName{Namespace: testNamespace, Name: "etcd"}, etcdSm)
		if err != nil && errors.IsNotFound(err) {
			return false, nil
		}

		return true, err
	})
	assert.NoError(t, err)
}

func TestMain(m *testing.M) {
	opts := zap.Options{
		Development: true,
	}
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	rootPath := filepath.Join("..", "..", "..")
	spokeCrds := readCRDFiles(
		filepath.Join(rootPath, "multiclusterobservability", "config", "crd", "bases", "observability.open-cluster-management.io_observabilityaddons.yaml"),
	)
	testEnvSpoke = &envtest.Environment{
		CRDDirectoryPaths:       []string{filepath.Join("testdata", "crd"), filepath.Join("..", "..", "config", "crd", "bases")},
		CRDs:                    spokeCrds,
		ControlPlaneStopTimeout: 5 * time.Minute,
	}

	var err error
	restCfgSpoke, err = testEnvSpoke.Start()
	if err != nil {
		panic(fmt.Sprintf("Failed to start spoke test environment: %v", err))
	}

	hubCRDs := readCRDFiles(
		filepath.Join(rootPath, "multiclusterobservability", "config", "crd", "bases", "observability.open-cluster-management.io_multiclusterobservabilities.yaml"),
		filepath.Join(rootPath, "endpointmetrics", "manifests", "prometheus", "crd", "servicemonitor_crd_0_53_1.yaml"),
	)
	hubCRDs = append(hubCRDs, spokeCrds...)

	testEnvHub = &envtest.Environment{
		CRDDirectoryPaths:       []string{filepath.Join("testdata", "crd"), filepath.Join("..", "..", "..", "config", "crd", "bases")},
		CRDs:                    hubCRDs,
		ControlPlaneStopTimeout: 5 * time.Minute,
	}

	restCfgHub, err = testEnvHub.Start()
	if err != nil {
		panic(fmt.Sprintf("Failed to start hub test environment: %v", err))
	}

	code := m.Run()

	err = testEnvSpoke.Stop()
	if err != nil {
		panic(fmt.Sprintf("Failed to stop spoke test environment: %v", err))
	}

	err = testEnvHub.Stop()
	if err != nil {
		panic(fmt.Sprintf("Failed to stop hub test environment: %v", err))
	}

	os.Exit(code)
}

func createBaseScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	kubescheme.AddToScheme(scheme)
	promv1.AddToScheme(scheme)
	oav1beta1.AddToScheme(scheme)
	mcov1beta2.AddToScheme(scheme)
	return scheme
}

func setupCommonHubResources(t *testing.T, k8sClient client.Client, ns string) {
	// Create resources required for the observability addon controller
	resourcesDeps := []client.Object{
		makeNamespace(ns),
		newHubInfoSecret([]byte{}, ns),
		newImagesCM(ns),
	}
	if err := createResources(k8sClient, resourcesDeps...); err != nil {
		t.Fatalf("Failed to create resources: %v", err)
	}
}

func tearDownCommonHubResources(t *testing.T, k8sClient client.Client, ns string) {
	// Delete resources required for the observability addon controller
	resourcesDeps := []client.Object{
		makeNamespace(ns),
	}
	for _, resource := range resourcesDeps {
		if err := k8sClient.Delete(context.Background(), resource); err != nil {
			t.Fatalf("Failed to delete resource: %v", err)
		}
	}
}

func setupCommonSpokeResources(t *testing.T, k8sClient client.Client) {
	// Create resources required for the observability addon controller
	resourcesDeps := []client.Object{
		makeNamespace("open-cluster-management-addon-observability"),
		newHubInfoSecret([]byte{}, "open-cluster-management-addon-observability"),
		newImagesCM("open-cluster-management-addon-observability"),
	}
	if err := createResources(k8sClient, resourcesDeps...); err != nil {
		t.Fatalf("Failed to create resources: %v", err)
	}
}

func tearDownCommonSpokeResources(t *testing.T, k8sClient client.Client) {
	// Delete resources required for the observability addon controller
	resourcesDeps := []client.Object{
		makeNamespace("open-cluster-management-addon-observability"),
	}
	for _, resource := range resourcesDeps {
		if err := k8sClient.Delete(context.Background(), resource); err != nil {
			t.Fatalf("Failed to delete resource: %v", err)
		}
	}
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

func makeNamespace(name string) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
}

// createResources creates the given resources in the cluster.
func createResources(client client.Client, resources ...client.Object) error {
	for _, resource := range resources {
		if err := client.Create(context.Background(), resource); err != nil {
			return err
		}
	}
	return nil
}

func newObservabilityAddonBis(name, ns string) *oav1beta1.ObservabilityAddon {
	return &oav1beta1.ObservabilityAddon{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: observabilityshared.ObservabilityAddonSpec{
			EnableMetrics: true,
		},
	}
}

func newHostedCluster(name, ns string) *hyperv1.HostedCluster {
	return &hyperv1.HostedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: hyperv1.HostedClusterSpec{
			ClusterID: "0ecda14c-0583-4ad3-b38e-d925694078cb",
			Platform: hyperv1.PlatformSpec{
				Type: "AWS",
			},
			Release: hyperv1.Release{
				Image: "quay.io/openshift-release-dev/ocp-release:4.14.13-multi",
			},
			Etcd: hyperv1.EtcdSpec{
				ManagementType: "Managed",
			},
			Services: []hyperv1.ServicePublishingStrategyMapping{},
		},
	}
}

func newServiceMonitor(name, namespace string) *promv1.ServiceMonitor {
	return &promv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: promv1.ServiceMonitorSpec{
			Endpoints: []promv1.Endpoint{
				{
					Port:      "metrics",
					TLSConfig: &promv1.TLSConfig{},
				},
			},
			Selector:          metav1.LabelSelector{},
			NamespaceSelector: promv1.NamespaceSelector{},
		},
	}
}

func newMicroshiftVersionCM(namespace string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "microshift-version",
			Namespace: namespace,
		},
		Data: map[string]string{
			"version": "v4.15.15",
		},
	}
}

func newMetricsAllowlistCM(namespace string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "observability-metrics-allowlist",
			Namespace: namespace,
		},
		Data: map[string]string{
			"metrics_list.yaml": `
names:
  - apiserver_watch_events_sizes_bucket
`,
		},
	}
}
