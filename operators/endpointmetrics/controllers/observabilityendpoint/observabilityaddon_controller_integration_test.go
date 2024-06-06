// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

//go:build integration

package observabilityendpoint

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	hyperv1 "github.com/openshift/hypershift/api/v1alpha1"
	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/pkg/hypershift"
	"github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/pkg/util"
	oav1beta1 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

// TestIntegrationReconcileHypershift tests the reconcile function for hypershift CRDs.
func TestIntegrationReconcileHypershift(t *testing.T) {
	testNamespace := "open-cluster-management-addon-observability"
	namespace = testNamespace
	hubNamespace = "local-cluster"
	isHubMetricsCollector = true
	installPrometheus = false
	serviceAccountName = "endpoint-monitoring-operator"

	testEnv, k8sClient := setupTestEnv(t)
	defer testEnv.Stop()

	hostedClusterNs := "hosted-cluster-ns"
	hostedClusterName := "myhostedcluster"
	hostedCluster := newHostedCluster(hostedClusterName, hostedClusterNs)

	resourcesDeps := []client.Object{
		// Create resources required for the observability addon controller
		makeNamespace(testNamespace),
		newHubInfoSecret([]byte{}, testNamespace),
		newImagesCM(testNamespace),
		// Create resources required for the hypershift case
		makeNamespace(hostedClusterNs),
		makeNamespace(hypershift.HostedClusterNamespace(hostedCluster)),
		hostedCluster,
		newServiceMonitor(hypershift.EtcdSmName, hypershift.HostedClusterNamespace(hostedCluster)),
		newServiceMonitor(hypershift.ApiServerSmName, hypershift.HostedClusterNamespace(hostedCluster)),
	}
	if err := createResources(k8sClient, resourcesDeps...); err != nil {
		t.Fatalf("Failed to create resources: %v", err)
	}

	mgr, err := ctrl.NewManager(testEnv.Config, ctrl.Options{
		Scheme:  k8sClient.Scheme(),
		Metrics: metricsserver.Options{BindAddress: "0"},
	})
	assert.NoError(t, err)

	hubClientWithReload, err := util.NewReloadableHubClientWithReloadFunc(func() (client.Client, error) {
		return k8sClient, nil
	})
	assert.NoError(t, err)
	reconciler := ObservabilityAddonReconciler{
		Client:    k8sClient,
		HubClient: hubClientWithReload,
	}

	err = reconciler.SetupWithManager(mgr)
	assert.NoError(t, err)

	go func() {
		err = mgr.Start(ctrl.SetupSignalHandler())
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

// setupTestEnv starts the test environment (etcd and kube api-server).
func setupTestEnv(t *testing.T) (*envtest.Environment, client.Client) {
	rootPath := filepath.Join("..", "..", "..")
	crds := readCRDFiles(t,
		filepath.Join(rootPath, "multiclusterobservability", "config", "crd", "bases", "observability.open-cluster-management.io_multiclusterobservabilities.yaml"),
		filepath.Join(rootPath, "endpointmetrics", "manifests", "prometheus", "crd", "servicemonitor_crd_0_53_1.yaml"),
		filepath.Join(rootPath, "endpointmetrics", "manifests", "prometheus", "crd", "prometheusrule_crd_0_53_1.yaml"),
	)
	testEnv := &envtest.Environment{
		CRDDirectoryPaths: []string{filepath.Join("testdata", "crd"), filepath.Join("..", "..", "config", "crd", "bases")},
		CRDs:              crds,
	}

	cfg, err := testEnv.Start()
	if err != nil {
		t.Fatal(err)
	}

	scheme := runtime.NewScheme()
	kubescheme.AddToScheme(scheme)
	hyperv1.AddToScheme(scheme)
	promv1.AddToScheme(scheme)
	oav1beta1.AddToScheme(scheme)
	mcov1beta2.AddToScheme(scheme)

	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatal(err)
	}

	opts := zap.Options{
		Development: true,
	}
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	return testEnv, k8sClient
}

func readCRDFiles(t *testing.T, crdPaths ...string) []*apiextensionsv1.CustomResourceDefinition {
	ret := []*apiextensionsv1.CustomResourceDefinition{}

	for _, crdPath := range crdPaths {
		crdYamlData, err := os.ReadFile(crdPath)
		if err != nil {
			t.Fatalf("Failed to read CRD file: %v", err)
		}

		dec := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
		var crd apiextensionsv1.CustomResourceDefinition
		_, _, err = dec.Decode(crdYamlData, nil, &crd)
		if err != nil {
			t.Fatalf("Failed to decode CRD: %v", err)
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
