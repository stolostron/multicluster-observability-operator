// Copyright (c) 2020 Red Hat, Inc.

package multiclustermonitoring

import (
	"context"
	"os"
	"path"
	"testing"

	observatoriumv1alpha1 "github.com/observatorium/configuration/api/v1alpha1"
	configv1 "github.com/openshift/api/config/v1"
	routev1 "github.com/openshift/api/route/v1"
	fakeconfigclient "github.com/openshift/client-go/config/clientset/versioned/fake"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"

	monitoringv1alpha1 "github.com/open-cluster-management/multicluster-observability-operator/pkg/apis/monitoring/v1alpha1"
)

func init() {
	os.Setenv("TEMPLATES_PATH", "../../../manifests/")
}

func TestLabelsForMultiClusterMonitoring(t *testing.T) {
	lab := labelsForMultiClusterMonitoring("test")

	value, _ := lab["monitoring.open-cluster-management.io/name"]
	if value != "test" {
		t.Errorf("value (%v) is not the expected (test)", value)
	}
}

func createObservatoriumApiService(name, namespace string) *corev1.Service {
	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name + "-observatorium-observatorium-api",
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/component": "api",
				"app.kubernetes.io/instance":  name + "-observatorium",
			},
		},
		Spec: corev1.ServiceSpec{},
	}
}

func createClusterVersion() *configv1.ClusterVersion {
	return &configv1.ClusterVersion{
		ObjectMeta: metav1.ObjectMeta{Name: "version"},
		Spec: configv1.ClusterVersionSpec{
			ClusterID: configv1.ClusterID("xxx-xxxxxx-xxxx"),
		},
	}
}

func TestMultiClusterMonitoringCRUpdate(t *testing.T) {
	var (
		name               = "monitoring"
		namespace          = "open-cluster-management-observability"
		defaultStorageSize = "3Gi"
	)
	logf.SetLogger(logf.ZapLogger(true))

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get work dir: (%v)", err)
	}
	//provide a non-existence path to bypass the rendering
	//cannot convert unstructured.Unstructured into v1.Service in fake client
	os.Setenv("TEMPLATES_PATH", path.Join(wd, "../../../tests/manifests"))

	// A MultiClusterObservability object with metadata and spec.
	mcm := &monitoringv1alpha1.MultiClusterObservability{
		TypeMeta:   metav1.TypeMeta{Kind: "MultiClusterObservability"},
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
		Spec:       monitoringv1alpha1.MultiClusterMonitoringSpec{},
	}

	// Register operator types with the runtime scheme.
	s := scheme.Scheme
	monitoringv1alpha1.SchemeBuilder.AddToScheme(s)
	observatoriumv1alpha1.AddToScheme(s)
	routev1.AddToScheme(s)

	svc := createObservatoriumApiService(name, namespace)
	objs := []runtime.Object{mcm, svc}
	// Create a fake client to mock API calls.
	cl := fake.NewFakeClient(objs...)

	ocpClient := fakeconfigclient.NewSimpleClientset([]runtime.Object{createClusterVersion()}...)
	// Create a ReconcileMemcached object with the scheme and fake client.
	r := &ReconcileMultiClusterMonitoring{client: cl, scheme: s, ocpClient: ocpClient}

	// Mock request to simulate Reconcile() being called on an event for a
	// watched resource .
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		},
	}

	_, err = r.Reconcile(req)
	if err != nil {
		t.Fatalf("reconcile: (%v)", err)
	}

	updatedMCM := &monitoringv1alpha1.MultiClusterObservability{}
	err = cl.Get(context.TODO(), req.NamespacedName, updatedMCM)
	if err != nil {
		t.Fatalf("Failed to get MultiClusterObservability: (%v)", err)
	}
	log.Info("updated MultiClusterObservability successfully", "MultiClusterObservability", updatedMCM)

	// A MultiClusterObservability object with metadata and spec.
	mcm = &monitoringv1alpha1.MultiClusterObservability{
		TypeMeta:   metav1.TypeMeta{Kind: "MultiClusterObservability"},
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
		Spec: monitoringv1alpha1.MultiClusterMonitoringSpec{
			Observatorium: &observatoriumv1alpha1.ObservatoriumSpec{
				Compact: observatoriumv1alpha1.CompactSpec{
					VolumeClaimTemplate: observatoriumv1alpha1.VolumeClaimTemplate{
						Spec: corev1.PersistentVolumeClaimSpec{
							AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceName(corev1.ResourceStorage): resource.MustParse(defaultStorageSize),
								},
							},
						},
					},
				},
				Rule: observatoriumv1alpha1.RuleSpec{
					VolumeClaimTemplate: observatoriumv1alpha1.VolumeClaimTemplate{
						Spec: corev1.PersistentVolumeClaimSpec{
							AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceName(corev1.ResourceStorage): resource.MustParse(defaultStorageSize),
								},
							},
						},
					},
				},
			},
		},
	}
	err = cl.Update(context.TODO(), mcm)
	if err != nil {
		t.Fatalf("Failed to update MultiClusterObservability: (%v)", err)
	}
	_, err = r.Reconcile(req)
	if err != nil {
		t.Fatalf("reconcile: (%v)", err)
	}
	updatedMCM = &monitoringv1alpha1.MultiClusterObservability{}
	err = r.client.Get(context.TODO(), req.NamespacedName, updatedMCM)
	if err != nil {
		t.Fatalf("Failed to get MultiClusterObservability: (%v)", err)
	}
	log.Info("updated MultiClusterObservability successfully", "MultiClusterObservability", updatedMCM)

}
