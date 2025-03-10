// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

//go:build integration

package multiclusterobservability_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	ocinfrav1 "github.com/openshift/api/config/v1"
	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	observabilityshared "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/shared"
	oav1beta1 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/controllers/multiclusterobservability"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	kubescheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

var (
	testEnvSpoke *envtest.Environment
	restCfgSpoke *rest.Config
	testEnvHub   *envtest.Environment
	restCfgHub   *rest.Config
)

// TestIntegrationReconcileAddon ensures that addon directives are applied to the resources in the cluster.
// This includes enabled/disabled, interval, and resources requirements.
func TestIntegrationReconcileMCO(t *testing.T) {
	scheme := createBaseScheme()
	assert.NoError(t, ocinfrav1.AddToScheme(scheme))
	assert.NoError(t, oav1beta1.AddToScheme(scheme))
	assert.NoError(t, mcov1beta2.AddToScheme(scheme))
	assert.NoError(t, clusterv1.AddToScheme(scheme))

	// Setup spoke client and resources
	k8sSpokeClient, err := client.New(restCfgSpoke, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatal(err)
	}

	spokeNamespace := "spoke-ns"
	resources := []client.Object{
		newNamespace(spokeNamespace),
	}
	if err := createResources(k8sSpokeClient, resources...); err != nil {
		t.Fatalf("Failed to create resources: %v", err)
	}

	// Setup hub client and resources
	k8sHubClient, err := client.New(restCfgHub, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatal(err)
	}
	hubNamespace := "hub-ns"
	resources = []client.Object{
		newNamespace(hubNamespace),
		// add MCO
		&mcov1beta2.MultiClusterObservability{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "mco",
				Namespace: hubNamespace,
			},
			Spec: mcov1beta2.MultiClusterObservabilitySpec{
				ObservabilityAddonSpec: &observabilityshared.ObservabilityAddonSpec{
					Interval: 44,
					Resources: &corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("100Mi"),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("100Mi"),
						},
					},
				},
				StorageConfig: &mcov1beta2.StorageConfig{
					MetricObjectStorage: &observabilityshared.PreConfiguredStorage{
						Key:  "key",
						Name: "name",
					},
				},
			},
		},
		// add managedCluster
		&clusterv1.ManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "managedcluster",
				Namespace: "spoke-a",
			},
			Spec: clusterv1.ManagedClusterSpec{},
		},
	}
	if err := createResources(k8sHubClient, resources...); err != nil {
		t.Fatalf("Failed to create resources: %v", err)
	}

	// Setup the controller
	mgr, err := ctrl.NewManager(testEnvSpoke.Config, ctrl.Options{
		Scheme:  k8sSpokeClient.Scheme(),
		Metrics: ctrlmetrics.Options{BindAddress: "0"}, // Avoids port conflict with the default port 8080
	})
	assert.NoError(t, err)
	// type MultiClusterObservabilityReconciler struct {
	// 	Manager    manager.Manager
	// 	Client     client.Client
	// 	Log        logr.Logger
	// 	Scheme     *runtime.Scheme
	// 	CRDMap     map[string]bool
	// 	APIReader  client.Reader
	// 	RESTMapper meta.RESTMapper
	// }
	// Manager:    mgr,
	// 	Client:     mgr.GetClient(),
	// 	Log:        ctrl.Log.WithName("controllers").WithName("MultiClusterObservability"),
	// 	Scheme:     mgr.GetScheme(),
	// 	CRDMap:     crdMaps,
	// 	APIReader:  mgr.GetAPIReader(),
	// 	RESTMapper: mgr.GetRESTMapper(),
	reconciler := multiclusterobservability.MultiClusterObservabilityReconciler{
		Client:     k8sHubClient,
		Manager:    mgr,
		Log:        ctrl.Log.WithName("controllers").WithName("MultiClusterObservability"),
		Scheme:     scheme,
		CRDMap:     nil,
		APIReader:  nil,
		RESTMapper: mgr.GetRESTMapper(),
	}
	err = reconciler.SetupWithManager(mgr)
	assert.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		err = mgr.Start(ctx)
		assert.NoError(t, err)
	}()

	// ensure the MCO fields are replicated to the observability addon

}

func TestMain(m *testing.M) {
	opts := zap.Options{
		Development: true,
	}
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	rootPath := filepath.Join("..", "..", "..")
	// spokeCrds := readCRDFiles(
	// filepath.Join(rootPath, "multiclusterobservability", "config", "crd", "bases", "observability.open-cluster-management.io_observabilityaddons.yaml"),
	// filepath.Join(rootPath, "endpointmetrics", "manifests", "prometheus", "crd", "servicemonitor_crd_0_53_1.yaml"),
	// filepath.Join(rootPath, "endpointmetrics", "manifests", "prometheus", "crd", "prometheusrule_crd_0_53_1.yaml"),
	// )
	testEnvSpoke = &envtest.Environment{
		// CRDDirectoryPaths:       []string{filepath.Join("testdata", "crd")},
		// CRDs:                    spokeCrds,
		ControlPlaneStopTimeout: 5 * time.Minute,
	}

	var err error
	restCfgSpoke, err = testEnvSpoke.Start()
	if err != nil {
		panic(fmt.Sprintf("Failed to start spoke test environment: %v", err))
	}

	hubCRDs := readCRDFiles(
		// filepath.Join(rootPath, "multiclusterobservability", "config", "crd", "bases", "observability.open-cluster-management.io_observabilityaddons.yaml"),
		filepath.Join(rootPath, "multiclusterobservability", "config", "crd", "bases", "observability.open-cluster-management.io_multiclusterobservabilities.yaml"),
	// filepath.Join(rootPath, "endpointmetrics", "manifests", "prometheus", "crd", "servicemonitor_crd_0_53_1.yaml"),
	)

	testEnvHub = &envtest.Environment{
		// CRDDirectoryPaths:       []string{filepath.Join("testdata", "crd")},
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

// createResources creates the given resources in the cluster.
func createResources(client client.Client, resources ...client.Object) error {
	for _, resource := range resources {
		if err := client.Create(context.Background(), resource); err != nil {
			return err
		}
	}
	return nil
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

func newNamespace(name string) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
}
