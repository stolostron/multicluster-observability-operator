// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

//go:build integration

package status

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/pkg/util"
	oashared "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/shared"
	oav1beta1 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
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

func TestIntegrationReconcileStatus(t *testing.T) {
	spokeNamespace := "test-namespace"
	hubNamespace := "hub-namespace"
	obsAddonName := "observability-addon"

	// Setup spoke cluster
	testEnv, k8sClient := setupTestEnv(t)
	defer testEnv.Stop()

	spokeObsAddon := newObservabilityAddon(obsAddonName, spokeNamespace)
	resourcesDeps := []client.Object{
		makeNamespace(spokeNamespace),
		spokeObsAddon,
	}
	if err := createResources(k8sClient, resourcesDeps...); err != nil {
		t.Fatalf("Failed to create resources: %v", err)
	}

	// Setup hub cluster
	hubTestEnv, hubK8sClient := setupTestEnv(t)
	defer hubTestEnv.Stop()

	resourcesDeps = []client.Object{
		makeNamespace(hubNamespace),
		newObservabilityAddon(obsAddonName, hubNamespace),
	}
	if err := createResources(hubK8sClient, resourcesDeps...); err != nil {
		t.Fatalf("Failed to create resources: %v", err)
	}

	// Setup controller manager
	mgr, err := ctrl.NewManager(testEnv.Config, ctrl.Options{
		Scheme:  k8sClient.Scheme(),
		Metrics: metricsserver.Options{BindAddress: "0"},
	})
	assert.NoError(t, err)

	hubClientWithReload, err := util.NewReloadableHubClientWithReloadFunc(func() (client.Client, error) {
		return hubK8sClient, nil
	})
	assert.NoError(t, err)
	reconciler := StatusReconciler{
		Client:       k8sClient,
		Namespace:    spokeNamespace,
		HubNamespace: hubNamespace,
		ObsAddonName: obsAddonName,
		Logger:       ctrl.Log.WithName("controllers").WithName("Status"),
		HubClient:    hubClientWithReload,
	}

	err = reconciler.SetupWithManager(mgr)
	assert.NoError(t, err)

	go func() {
		err = mgr.Start(ctrl.SetupSignalHandler())
		assert.NoError(t, err)
	}()

	// Test:
	// Update on the spoke addon status should trigger an update on the hub addon status.

	go func() {
		// Update spoke addon status concurrently to trigger the reconcile loop.
		addCondition(spokeObsAddon, "Deployed", metav1.ConditionTrue)
		err := wait.Poll(2*time.Second, 10*time.Second, func() (bool, error) {
			spokeObsAddon.Status.Conditions[0].LastTransitionTime = metav1.Time{
				Time: time.Now(),
			}
			err := k8sClient.Status().Update(context.Background(), spokeObsAddon)
			if err != nil {
				return false, err
			}

			return true, nil
		})
		assert.NoError(t, err)
	}()

	err = wait.Poll(1*time.Second, 10*time.Second, func() (bool, error) {
		hubObsAddon := &oav1beta1.ObservabilityAddon{}
		err := hubK8sClient.Get(context.Background(), types.NamespacedName{Name: obsAddonName, Namespace: hubNamespace}, hubObsAddon)
		if err != nil {
			return false, err
		}

		return len(hubObsAddon.Status.Conditions) > 0, nil
	})

	assert.NoError(t, err)
}

// setupTestEnv starts the test environment (etcd and kube api-server).
func setupTestEnv(t *testing.T) (*envtest.Environment, client.Client) {
	scheme := runtime.NewScheme()
	kubescheme.AddToScheme(scheme)
	oav1beta1.AddToScheme(scheme)

	addonCrdYamlData, err := os.ReadFile("../../config/crd/bases/observability.open-cluster-management.io_observabilityaddons.yaml")
	if err != nil {
		t.Fatalf("Failed to read CRD file: %v", err)
	}

	dec := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	var crd apiextensionsv1.CustomResourceDefinition
	_, _, err = dec.Decode(addonCrdYamlData, nil, &crd)
	if err != nil {
		t.Fatalf("Failed to decode CRD: %v", err)
	}

	testEnv := &envtest.Environment{
		CRDs: []*apiextensionsv1.CustomResourceDefinition{&crd},
	}

	cfg, err := testEnv.Start()
	if err != nil {
		t.Fatal(err)
	}

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

func newObservabilityAddon(name string, ns string) *oav1beta1.ObservabilityAddon {
	return &oav1beta1.ObservabilityAddon{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: oashared.ObservabilityAddonSpec{
			EnableMetrics: true,
			Interval:      60,
		},
	}
}

func addCondition(oba *oav1beta1.ObservabilityAddon, statusType string, status metav1.ConditionStatus) {
	condition := oav1beta1.StatusCondition{
		Type:    statusType,
		Status:  status,
		Reason:  "DummyReason",
		Message: "DummyMessage",
		LastTransitionTime: metav1.Time{
			Time: time.Now(),
		},
	}
	oba.Status.Conditions = append(oba.Status.Conditions, condition)
}
