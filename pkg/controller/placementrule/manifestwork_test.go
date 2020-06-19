// Copyright (c) 2020 Red Hat, Inc.

package placementrule

import (
	"context"
	"os"
	"path"
	"testing"

	ocinfrav1 "github.com/openshift/api/config/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	workv1 "github.com/open-cluster-management/api/work/v1"
	monitoringv1alpha1 "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/apis/monitoring/v1alpha1"
)

func TestManifestWork(t *testing.T) {
	secretName := "test-secret"
	token := "test-token"
	ca := "test-ca"

	s := scheme.Scheme
	if err := ocinfrav1.AddToScheme(s); err != nil {
		t.Fatalf("Unable to add ocinfrav1 scheme: (%v)", err)
	}
	if err := workv1.AddToScheme(s); err != nil {
		t.Fatalf("Unable to add workv1 scheme: (%v)", err)
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
		},
		Type: corev1.SecretTypeServiceAccountToken,
		Data: map[string][]byte{
			"token":  []byte(token),
			"ca.crt": []byte(ca),
		},
	}
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceAccountName,
			Namespace: namespace,
			Annotations: map[string]string{
				ownerLabelKey: ownerLabelValue,
			},
		},
		Secrets: []corev1.ObjectReference{
			{
				Kind:      "Secret",
				Namespace: namespace,
				Name:      secretName,
			},
		},
	}
	infra := &ocinfrav1.Infrastructure{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
		Status: ocinfrav1.InfrastructureStatus{
			APIServerURL: "test-api-url",
		},
	}
	objs := []runtime.Object{secret, sa, infra}
	c := fake.NewFakeClient(objs...)

	mcm := &monitoringv1alpha1.MultiClusterMonitoring{
		Spec: monitoringv1alpha1.MultiClusterMonitoringSpec{
			ImagePullSecret: "pull-secret",
		},
	}
	ps := &corev1.Secret{
		Data: map[string][]byte{
			".dockerconfigjson": []byte("test-docker-config"),
		},
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get work dir: (%v)", err)
	}
	templatePath = path.Join(wd, "../../../manifests/endpoint-monitoring")
	err = createManifestWork(c, namespace, mcm, ps)
	if err != nil {
		t.Fatalf("Failed to create manifestwork: (%v)", err)
	}
	found := &workv1.ManifestWork{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: workName, Namespace: namespace}, found)
	if err != nil {
		t.Fatalf("Failed to get manifestwork: (%v)", err)
	}
	if len(found.Spec.Workload.Manifests) != 7 {
		t.Fatal("Wrong size of manifests in the mainfestwork")
	}

	spokeNameSpace = "spoke-ns"
	err = createManifestWork(c, namespace, mcm, ps)
	if err != nil {
		t.Fatalf("Failed to create manifestwork with updated namespace: (%v)", err)
	}

	err = deleteManifestWork(c, namespace)
	if err != nil {
		t.Fatalf("Failed to delete manifestwork: (%v)", err)
	}
	err = c.Get(context.TODO(), types.NamespacedName{Name: workName, Namespace: namespace}, found)
	if err == nil || !errors.IsNotFound(err) {
		t.Fatalf("Failed to delete EndpointMonitoring: (%v)", err)
	}
}
