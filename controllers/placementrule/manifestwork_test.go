// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

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
	mcov1beta2 "github.com/open-cluster-management/multicluster-observability-operator/api/v1beta2"
	mcocontroller "github.com/open-cluster-management/multicluster-observability-operator/controllers/multiclusterobservability"
	"github.com/open-cluster-management/multicluster-observability-operator/pkg/config"
)

const (
	pullSecretName   = "test-pull-secret"
	operatorWorkSize = 6
	resourceWorkSize = 6
)

func newTestMCO() *mcov1beta2.MultiClusterObservability {
	return &mcov1beta2.MultiClusterObservability{
		ObjectMeta: metav1.ObjectMeta{
			Name: mcoName,
		},
		Spec: mcov1beta2.MultiClusterObservabilitySpec{
			ImagePullSecret: pullSecretName,
			ObservabilityAddonSpec: &mcov1beta2.ObservabilityAddonSpec{
				EnableMetrics: true,
				Interval:      1,
			},
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

func newCASecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mcocontroller.GetServerCerts(),
			Namespace: mcoNamespace,
		},
		Data: map[string][]byte{
			"ca.crt": []byte("test-ca-crt"),
		},
	}
}

func newCertSecret(namespaces ...string) *corev1.Secret {
	ns := namespace
	if len(namespaces) != 0 {
		ns = namespaces[0]
	}
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      certsName,
			Namespace: ns,
		},
		Data: map[string][]byte{
			"tls.crt": []byte("test-tls-crt"),
			"tls.key": []byte("test-tls-key"),
		},
	}
}

func NewMetricsAllowListCM() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.AllowlistConfigMapName,
			Namespace: mcoNamespace,
		},
		Data: map[string]string{"metrics_list.yaml": `
  names:
    - a
    - b
`},
	}
}

func NewMetricsCustomAllowListCM() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.AllowlistCustomConfigMapName,
			Namespace: mcoNamespace,
		},
		Data: map[string]string{"metrics_list.yaml": `
  names:
    - c
    - d
`},
	}
}
func TestManifestWork(t *testing.T) {

	initSchema(t)

	objs := []runtime.Object{newSATokenSecret(), newTestSA(), newTestInfra(),
		newTestRoute(), newCASecret(), newCertSecret(), NewMetricsAllowListCM(), NewMetricsCustomAllowListCM()}
	c := fake.NewFakeClient(objs...)

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get work dir: (%v)", err)
	}
	templatePath = path.Join(wd, "../../manifests/endpoint-observability")

	err = createManifestWorks(c, nil, namespace, clusterName, newTestMCO(), newTestPullSecret())
	if err != nil {
		t.Fatalf("Failed to create manifestworks: (%v)", err)
	}
	found := &workv1.ManifestWork{}
	workName := namespace + operatorWorkNameSuffix
	err = c.Get(context.TODO(), types.NamespacedName{Name: workName, Namespace: namespace}, found)
	if err != nil {
		t.Fatalf("Failed to get manifestwork %s: (%v)", workName, err)
	}
	if len(found.Spec.Workload.Manifests) != operatorWorkSize {
		t.Fatalf("Wrong size of manifests in the mainfestwork %s", workName)
	}
	workName = namespace + resWorkNameSuffix
	err = c.Get(context.TODO(), types.NamespacedName{Name: workName, Namespace: namespace}, found)
	if err != nil {
		t.Fatalf("Failed to get manifestwork %s: (%v)", workName, err)
	}
	if len(found.Spec.Workload.Manifests) != resourceWorkSize {
		t.Fatalf("Wrong size of manifests in the mainfestwork %s", workName)
	}

	err = createManifestWorks(c, nil, namespace, clusterName, newTestMCO(), nil)
	if err != nil {
		t.Fatalf("Failed to create manifestworks: (%v)", err)
	}
	err = c.Get(context.TODO(), types.NamespacedName{Name: workName, Namespace: namespace}, found)
	if err != nil {
		t.Fatalf("Failed to get manifestwork %s: (%v)", workName, err)
	}
	if len(found.Spec.Workload.Manifests) != resourceWorkSize-1 {
		t.Fatalf("Wrong size of manifests in the mainfestwork %s", workName)
	}

	spokeNameSpace = "spoke-ns"
	err = createManifestWorks(c, nil, namespace, clusterName, newTestMCO(), newTestPullSecret())
	if err != nil {
		t.Fatalf("Failed to create manifestworks with updated namespace: (%v)", err)
	}

	err = deleteManifestWorks(c, namespace)
	if err != nil {
		t.Fatalf("Failed to delete manifestworks: (%v)", err)
	}
	err = c.Get(context.TODO(), types.NamespacedName{Name: namespace + operatorWorkNameSuffix, Namespace: namespace}, found)
	if err == nil || !errors.IsNotFound(err) {
		t.Fatalf("Manifestwork not deleted: (%v)", err)
	}
	err = c.Get(context.TODO(), types.NamespacedName{Name: namespace + resWorkNameSuffix, Namespace: namespace}, found)
	if err == nil || !errors.IsNotFound(err) {
		t.Fatalf("Manifestwork not deleted: (%v)", err)
	}
}
