// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

//go:build integration

package observabilityendpoint_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	ocinfrav1 "github.com/openshift/api/config/v1"
	hyperv1 "github.com/openshift/hypershift/api/hypershift/v1alpha1"
	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/controllers/observabilityendpoint"
	"github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/pkg/hypershift"
	"github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/pkg/util"
	observabilityshared "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/shared"
	oav1beta1 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	operatorconfig "github.com/stolostron/multicluster-observability-operator/operators/pkg/config"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
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

// TestIntegrationReconcileAddon ensures that addon directives are applied the the resources in the cluster.
func TestIntegrationReconcileAddon(t *testing.T) {
	scheme := createBaseScheme()
	assert.NoError(t, ocinfrav1.AddToScheme(scheme))
	assert.NoError(t, oav1beta1.AddToScheme(scheme))

	// Setup spoke client and resources
	k8sSpokeClient, err := client.New(restCfgSpoke, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatal(err)
	}

	testNamespace := "spoke-ns"
	addon := newObservabilityAddon("observability-addon", testNamespace)
	addon.Spec.EnableMetrics = true
	addon.Spec.Interval = 60
	addon.Spec.Resources = &corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("100m"),
			corev1.ResourceMemory: resource.MustParse("200Mi"),
		},
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("10m"),
			corev1.ResourceMemory: resource.MustParse("20Mi"),
		},
	}
	endpointOperatorDeploy := &appsv1.Deployment{ // needed by the metrics-collector to copy some settings
		ObjectMeta: metav1.ObjectMeta{
			Name:      "endpoint-observability-operator",
			Namespace: testNamespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "endpoint-observability-operator",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "endpoint-observability-operator",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "endpoint-observability-operator",
							Image: "test",
						},
					},
				},
			},
		},
	}
	resources := []client.Object{}
	resources = append(resources, newAcmResources(testNamespace)...)
	resources = append(resources, newOcpSpokeResources(testNamespace)...)
	resources = append(resources, addon, endpointOperatorDeploy)
	resources = append(resources, newPrometheusUwlResources()...) // to add uwl collector and check config
	resources = append(resources, newUwlMetrics("userwl", []string{"dummy_metric"})...)
	if err := createResources(k8sSpokeClient, resources...); err != nil {
		t.Fatalf("Failed to create resources: %v", err)
	}

	// Setup hub client and resources
	k8sHubClient, err := client.New(restCfgHub, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatal(err)
	}
	hubClientWithReload, err := util.NewReloadableHubClientWithReloadFunc(func() (client.Client, error) {
		return k8sHubClient, nil
	})
	assert.NoError(t, err)
	hubNamespace := "hub-ns"
	hubAddon := addon.DeepCopy()
	hubAddon.ResourceVersion = ""
	hubAddon.Namespace = hubNamespace
	resources = []client.Object{
		makeNamespace(hubNamespace),
		hubAddon,
	}
	if err := createResources(k8sHubClient, resources...); err != nil {
		t.Fatalf("Failed to create resources: %v", err)
	}

	// Setup the controller
	reconciler := observabilityendpoint.ObservabilityAddonReconciler{
		Client:                k8sSpokeClient,
		HubClient:             hubClientWithReload,
		IsHubMetricsCollector: false,
		Scheme:                scheme,
		Namespace:             testNamespace,
		HubNamespace:          hubNamespace,
		ServiceAccountName:    "endpoint-monitoring-operator",
		InstallPrometheus:     false,
	}
	mgr, err := ctrl.NewManager(testEnvSpoke.Config, ctrl.Options{
		Scheme:  k8sSpokeClient.Scheme(),
		Metrics: metricsserver.Options{BindAddress: "0"}, // Avoids port conflict with the default port 8080
	})
	assert.NoError(t, err)
	err = reconciler.SetupWithManager(mgr)
	assert.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		err = mgr.Start(ctx)
		assert.NoError(t, err)
	}()

	// Check that addon resources request are applied to the metrics-collector pod
	deployment := &appsv1.Deployment{}
	for _, resourceName := range []string{"metrics-collector-deployment", "uwl-metrics-collector-deployment"} {
		err = wait.PollUntilContextTimeout(ctx, 1*time.Second, 5*time.Second, false, func(ctx context.Context) (bool, error) {
			err := k8sSpokeClient.Get(ctx, types.NamespacedName{Name: resourceName, Namespace: testNamespace}, deployment)
			if err != nil {
				return false, err
			}
			return true, nil
		})
		if err != nil {
			t.Fatalf("Failed to get metrics-collector pod: %v", err)
		}
		assert.EqualValues(t, 1, *deployment.Spec.Replicas)
		pod := deployment.Spec.Template
		assert.Equal(t, addon.Spec.Resources.Limits.Cpu().String(), pod.Spec.Containers[0].Resources.Limits.Cpu().String())
		assert.Equal(t, addon.Spec.Resources.Limits.Memory().String(), pod.Spec.Containers[0].Resources.Limits.Memory().String())
		assert.Equal(t, addon.Spec.Resources.Requests.Cpu().String(), pod.Spec.Containers[0].Resources.Requests.Cpu().String())
		assert.Equal(t, addon.Spec.Resources.Requests.Memory().String(), pod.Spec.Containers[0].Resources.Requests.Memory().String())
		var intervalArg string
		for _, arg := range pod.Spec.Containers[0].Command {
			if strings.HasPrefix(arg, "--interval=") {
				intervalArg = arg
				break
			}
		}
		assert.Equal(t, fmt.Sprintf("--interval=%ds", addon.Spec.Interval), intervalArg)
	}

	currentAddon := &oav1beta1.ObservabilityAddon{}
	err = wait.PollUntilContextTimeout(ctx, 1*time.Second, 5*time.Second, false, func(ctx context.Context) (bool, error) {
		if err := k8sSpokeClient.Get(ctx, types.NamespacedName{Name: "observability-addon", Namespace: testNamespace}, currentAddon); err != nil {
			return false, err
		}
		return true, nil
	})
	if err != nil {
		t.Fatalf("Failed to get observability addon: %v", err)
	}
	var progressingCondition *oav1beta1.StatusCondition
	for _, condition := range currentAddon.Status.Conditions {
		if condition.Type == "Progressing" {
			progressingCondition = &condition
			break
		}
	}
	assert.NotNil(t, progressingCondition)
	assert.Equal(t, metav1.ConditionTrue, progressingCondition.Status)

	// Check that disabled addon removes metrics collector pods and sets the status to disabled
	if err := k8sSpokeClient.Get(ctx, types.NamespacedName{Name: "observability-addon", Namespace: testNamespace}, currentAddon); err != nil {
		t.Fatalf("Failed to get observability addon: %v", err)
	}
	disabledAddon := currentAddon.DeepCopy()
	disabledAddon.Spec.EnableMetrics = false
	if err := k8sSpokeClient.Update(ctx, disabledAddon); err != nil {
		t.Fatalf("Failed to update observability addon: %v", err)
	}

	for _, resourceName := range []string{"metrics-collector-deployment", "uwl-metrics-collector-deployment"} {
		err = wait.PollUntilContextTimeout(ctx, 1*time.Second, 5*time.Second, false, func(ctx context.Context) (bool, error) {
			err := k8sSpokeClient.Get(ctx, types.NamespacedName{Name: resourceName, Namespace: testNamespace}, deployment)
			if err != nil {
				return false, nil
			}
			return true, nil
		})
		if err != nil {
			t.Fatalf("Failed to wait for metrics-collector replicas to be zero: %v", err)
		}
		assert.Equal(t, int32(0), *deployment.Spec.Replicas)
	}

	err = wait.PollUntilContextTimeout(ctx, 1*time.Second, 5*time.Second, false, func(ctx context.Context) (bool, error) {
		if err := k8sSpokeClient.Get(ctx, types.NamespacedName{Name: "observability-addon", Namespace: testNamespace}, currentAddon); err != nil {
			return false, err
		}
		return true, nil
	})
	if err != nil {
		t.Fatalf("Failed to get observability addon: %v", err)
	}
	var disabledCondition *oav1beta1.StatusCondition
	for _, condition := range currentAddon.Status.Conditions {
		if condition.Type == "Disabled" {
			disabledCondition = &condition
			break
		}
	}
	assert.NotNil(t, disabledCondition)
	assert.Equal(t, metav1.ConditionTrue, disabledCondition.Status)

	// check invalid interval values are refused
	if err := k8sSpokeClient.Get(ctx, types.NamespacedName{Name: "observability-addon", Namespace: testNamespace}, currentAddon); err != nil {
		t.Fatalf("Failed to get observability addon: %v", err)
	}
	invalidAddon := currentAddon.DeepCopy()
	invalidAddon.Spec.Interval = 1
	if err := k8sSpokeClient.Update(ctx, invalidAddon); err == nil {
		t.Fatalf("Expected error when updating observability addon with invalid interval value")
	}

	invalidAddon.Spec.Interval = 10e6
	if err := k8sSpokeClient.Update(ctx, invalidAddon); err == nil {
		t.Fatalf("Expected error when updating observability addon with invalid interval value")
	}

	// delete the addon and check that resources are removed

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
	reconciler := observabilityendpoint.ObservabilityAddonReconciler{
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

func TestMain(m *testing.M) {
	opts := zap.Options{
		Development: true,
	}
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	rootPath := filepath.Join("..", "..", "..")
	spokeCrds := readCRDFiles(
		filepath.Join(rootPath, "multiclusterobservability", "config", "crd", "bases", "observability.open-cluster-management.io_observabilityaddons.yaml"),
		filepath.Join(rootPath, "endpointmetrics", "manifests", "prometheus", "crd", "servicemonitor_crd_0_53_1.yaml"),
		filepath.Join(rootPath, "endpointmetrics", "manifests", "prometheus", "crd", "prometheusrule_crd_0_53_1.yaml"),
	)
	testEnvSpoke = &envtest.Environment{
		CRDDirectoryPaths:       []string{filepath.Join("testdata", "crd")},
		CRDs:                    spokeCrds,
		ControlPlaneStopTimeout: 5 * time.Minute,
	}

	var err error
	restCfgSpoke, err = testEnvSpoke.Start()
	if err != nil {
		panic(fmt.Sprintf("Failed to start spoke test environment: %v", err))
	}

	hubCRDs := readCRDFiles(
		filepath.Join(rootPath, "multiclusterobservability", "config", "crd", "bases", "observability.open-cluster-management.io_observabilityaddons.yaml"),
		filepath.Join(rootPath, "multiclusterobservability", "config", "crd", "bases", "observability.open-cluster-management.io_multiclusterobservabilities.yaml"),
		filepath.Join(rootPath, "endpointmetrics", "manifests", "prometheus", "crd", "servicemonitor_crd_0_53_1.yaml"),
	)

	testEnvHub = &envtest.Environment{
		CRDDirectoryPaths:       []string{filepath.Join("testdata", "crd")},
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

func newAcmResources(ns string) []client.Object {
	return []client.Object{
		makeNamespace(ns),
		newHubInfoSecret([]byte{}, ns),
		newImagesCM(ns),
	}
}

func newOcpSpokeResources(addonNs string) []client.Object {
	return []client.Object{
		makeNamespace("openshift-monitoring"),
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "prometheus-k8s",
				Namespace: "openshift-monitoring",
			},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{
					{
						Name: "web",
						Port: 9090,
					},
				},
			},
		},
		&ocinfrav1.ClusterVersion{
			ObjectMeta: metav1.ObjectMeta{
				Name: "version",
			},
			Spec: ocinfrav1.ClusterVersionSpec{
				ClusterID: "b551a7ec-e6c1-4132-a95b-935d726a9766",
			},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "observability-alertmanager-accessor",
				Namespace: addonNs,
			},
			Data: map[string][]byte{
				"token": []byte("am token"),
			},
		},
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

func newObservabilityAddon(name, ns string) *oav1beta1.ObservabilityAddon {
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
			Networking: hyperv1.ClusterNetworking{
				NetworkType: "OpenShiftSDN",
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

func newPrometheusUwlResources() []client.Object {
	return []client.Object{
		makeNamespace("openshift-user-workload-monitoring"),
		&appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "prometheus-user-workload",
				Namespace: "openshift-user-workload-monitoring",
			},
			Spec: appsv1.StatefulSetSpec{
				// Replicas: util.Int32Ptr(1),
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"app": "prometheus-user-workload",
					},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"app": "prometheus-user-workload",
						},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "prometheus-user-workload",
								Image: "test",
							},
						},
					},
				},
			},
		},
	}
}

func newUwlMetrics(ns string, metrics []string) []client.Object {
	var data strings.Builder
	data.WriteString("names:\n")
	for _, metric := range metrics {
		data.WriteString(fmt.Sprintf("  - %s\n", metric))
	}
	return []client.Object{
		makeNamespace(ns),
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "observability-metrics-custom-allowlist",
				Namespace: ns,
			},
			Data: map[string]string{
				"uwl_metrics_list.yaml": data.String(),
			},
		},
	}
}

func newImagesCM(ns string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      operatorconfig.ImageConfigMap,
			Namespace: ns,
		},
		Data: map[string]string{
			operatorconfig.MetricsCollectorKey:   "metrics-collector-image",
			operatorconfig.NodeExporterKey:       "node-exporter-image",
			operatorconfig.KubeStateMetricsKey:   "kube-state-metrics-image",
			operatorconfig.KubeRbacProxyKey:      "kube-rbac-proxy-image",
			operatorconfig.PrometheusOperatorKey: "prometheus-operator-image",
		},
	}
}

func newHubInfoSecret(data []byte, ns string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      operatorconfig.HubInfoSecretName,
			Namespace: ns,
		},
		Data: map[string][]byte{
			operatorconfig.HubInfoSecretKey: data,
			operatorconfig.ClusterNameKey:   []byte("test-cluster"),
		},
	}
}
