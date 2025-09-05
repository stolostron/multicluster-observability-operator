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
	"slices"
	"testing"
	"time"

	routev1 "github.com/openshift/api/route/v1"
	imagev1client "github.com/openshift/client-go/image/clientset/versioned/typed/image/v1"
	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	promv1alpha1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1alpha1"
	observabilityshared "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/shared"
	mcov1beta1 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/controllers/multiclusterobservability"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	observatoriumAPIs "github.com/stolostron/observatorium-operator/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	kubescheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	addonapiv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

var (
	testEnvHub *envtest.Environment
	restCfgHub *rest.Config
)

// TestIntegrationMCO_HubRules verifies that hub rules and alerts for Thanos are deployed.
func TestIntegrationMCO_HubRules(t *testing.T) {
	scheme := createBaseScheme(t)

	wd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Setenv("TEMPLATES_PATH", filepath.Join(wd, "../../manifests")))

	// Set hub client and resources
	k8sHubClient, err := client.New(restCfgHub, client.Options{Scheme: scheme})
	require.NoError(t, err)

	storageSecretName := "storage-secret"
	storageSecretKey := "storage-key"
	hubNamespace := config.GetDefaultNamespace()
	resources := []client.Object{
		newNamespace(hubNamespace),
		newStorageSecret(storageSecretName, hubNamespace, storageSecretKey),
		newObservatoriumApiRoute(hubNamespace),
		newMCO(hubNamespace, storageSecretName, storageSecretKey),
	}
	err = createResources(k8sHubClient, resources...)
	require.NoError(t, err)

	// Set the controller
	mgr, err := ctrl.NewManager(testEnvHub.Config, ctrl.Options{
		Scheme:  k8sHubClient.Scheme(),
		Metrics: ctrlmetrics.Options{BindAddress: "0"}, // Avoids port conflict with the default port 8080
	})
	require.NoError(t, err)

	imageClient, err := imagev1client.NewForConfig(restCfgHub)
	require.NoError(t, err)
	reconciler := multiclusterobservability.MultiClusterObservabilityReconciler{
		Client:      k8sHubClient,
		Manager:     mgr,
		Log:         ctrl.Log.WithName("controllers").WithName("MultiClusterObservability"),
		Scheme:      scheme,
		CRDMap:      nil,
		APIReader:   nil,
		RESTMapper:  mgr.GetRESTMapper(),
		ImageClient: imageClient,
	}
	err = reconciler.SetupWithManager(mgr)
	assert.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		err = mgr.Start(ctx)
		assert.NoError(t, err)
	}()

	// Ensure that the prometheusRule is set.
	err = wait.PollUntilContextTimeout(ctx, time.Second, 15*time.Second, false, func(ctx context.Context) (done bool, err error) {
		hubRules := &promv1.PrometheusRule{}
		err = k8sHubClient.Get(ctx, types.NamespacedName{Name: "acm-observability-alert-rules", Namespace: hubNamespace}, hubRules)
		if err != nil {
			if errors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
		containsRules := slices.ContainsFunc(hubRules.Spec.Groups, func(e promv1.RuleGroup) bool { return e.Name == "acm-thanos-compact" }) // ensures content is set
		if !containsRules {
			return false, fmt.Errorf("PrometheusRule object doesn't contain expected rules: %v", hubRules.Spec.Groups)
		}

		return true, nil
	})
	assert.NoError(t, err)
}

func TestMain(m *testing.M) {
	opts := zap.Options{
		Development: true,
	}
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	var err error
	testEnvHub = &envtest.Environment{
		CRDDirectoryPaths:       []string{filepath.Join("testdata", "crd"), filepath.Join("..", "..", "bundle", "manifests")},
		ControlPlaneStopTimeout: 5 * time.Minute,
	}

	restCfgHub, err = testEnvHub.Start()
	if err != nil {
		panic(fmt.Sprintf("Failed to start hub test environment: %v", err))
	}

	exitCode := m.Run()

	err = testEnvHub.Stop()
	if err != nil {
		panic(fmt.Sprintf("Failed to stop hub test environment: %v", err))
	}

	os.Exit(exitCode)
}

func createBaseScheme(t *testing.T) *runtime.Scheme {
	scheme := runtime.NewScheme()
	require.NoError(t, kubescheme.AddToScheme(scheme))
	require.NoError(t, apiextensionsv1.AddToScheme(scheme))
	require.NoError(t, mcov1beta1.AddToScheme(scheme))
	require.NoError(t, mcov1beta2.AddToScheme(scheme))
	require.NoError(t, observatoriumAPIs.AddToScheme(scheme))
	require.NoError(t, promv1.AddToScheme(scheme))
	require.NoError(t, promv1alpha1.AddToScheme(scheme))
	require.NoError(t, routev1.AddToScheme(scheme))
	require.NoError(t, addonapiv1alpha1.AddToScheme(scheme))
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

func newNamespace(name string) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
}

func newStorageSecret(name, ns, key string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Data: map[string][]byte{
			key: []byte(""),
		},
	}
}

func newObservatoriumApiRoute(ns string) *routev1.Route {
	return &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "observatorium-api",
			Namespace: ns,
		},
		Spec: routev1.RouteSpec{
			Host: "toto.com",
			To: routev1.RouteTargetReference{
				Name: "toto",
			},
		},
	}
}

func newMCO(ns, storageSecretName, storageSecretKey string) *mcov1beta2.MultiClusterObservability {
	return &mcov1beta2.MultiClusterObservability{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mco",
			Namespace: ns,
		},
		Spec: mcov1beta2.MultiClusterObservabilitySpec{
			StorageConfig: &mcov1beta2.StorageConfig{
				MetricObjectStorage: &observabilityshared.PreConfiguredStorage{
					Name: storageSecretName,
					Key:  storageSecretKey,
				},
			},
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
		},
	}
}
