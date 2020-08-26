// Copyright (c) 2020 Red Hat, Inc.

package placementrule

import (
	"context"
	"os"
	"path"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	workv1 "github.com/open-cluster-management/api/work/v1"
	mcov1beta1 "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/apis/observability/v1beta1"
)

const (
	pullSecretName = "test-pull-secret"
)

func newTestMCO() *mcov1beta1.MultiClusterObservability {
	return &mcov1beta1.MultiClusterObservability{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mcoName,
			Namespace: mcoNamespace,
		},
		Spec: mcov1beta1.MultiClusterObservabilitySpec{
			ImagePullSecret: pullSecretName,
		},
	}
}

func newTestPullSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pullSecretName,
			Namespace: mcoNamespace,
		},
		Data: map[string][]byte{
			".dockerconfigjson": []byte("test-docker-config"),
		},
	}
}

func TestManifestWork(t *testing.T) {
	initSchema(t)

	objs := []runtime.Object{newSATokenSecret(), newTestSA(), newTestInfra(), newTestRoute()}
	c := fake.NewFakeClient(objs...)

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get work dir: (%v)", err)
	}
	templatePath = path.Join(wd, "../../../manifests/endpoint-observability")
	err = createManifestWork(c, namespace, clusterName, newTestMCO(), newTestPullSecret())
	if err != nil {
		t.Fatalf("Failed to create manifestwork: (%v)", err)
	}
	found := &workv1.ManifestWork{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: workName, Namespace: namespace}, found)
	if err != nil {
		t.Fatalf("Failed to get manifestwork: (%v)", err)
	}
	if len(found.Spec.Workload.Manifests) != 8 {
		t.Fatal("Wrong size of manifests in the mainfestwork")
	}

	spokeNameSpace = "spoke-ns"
	err = createManifestWork(c, namespace, clusterName, newTestMCO(), newTestPullSecret())
	if err != nil {
		t.Fatalf("Failed to create manifestwork with updated namespace: (%v)", err)
	}

	err = deleteManifestWork(c, namespace)
	if err != nil {
		t.Fatalf("Failed to delete manifestwork: (%v)", err)
	}
	err = c.Get(context.TODO(), types.NamespacedName{Name: workName, Namespace: namespace}, found)
	if err == nil || !errors.IsNotFound(err) {
		t.Fatalf("Failed to delete observabilityaddon: (%v)", err)
	}
}
