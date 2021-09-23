// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package placementrule

import (
	"context"
	"encoding/base64"
	"os"
	"path"
	"reflect"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/open-cluster-management/multicloud-operators-foundation/pkg/apis/imageregistry/v1alpha1"
	mcoshared "github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/api/shared"
	mcov1beta2 "github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	"github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	operatorconfig "github.com/open-cluster-management/multicluster-observability-operator/operators/pkg/config"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"
)

const (
	pullSecretName = "test-pull-secret"
	workSize       = 13
)

func newTestMCO() *mcov1beta2.MultiClusterObservability {
	return &mcov1beta2.MultiClusterObservability{
		ObjectMeta: metav1.ObjectMeta{
			Name: mcoName,
		},
		Spec: mcov1beta2.MultiClusterObservabilitySpec{
			ImagePullSecret: pullSecretName,
			ObservabilityAddonSpec: &mcoshared.ObservabilityAddonSpec{
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
			Name:      config.ServerCACerts,
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
			Name:      managedClusterObsCertName,
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
			Name:      operatorconfig.AllowlistConfigMapName,
			Namespace: mcoNamespace,
		},
		Data: map[string]string{"metrics_list.yaml": `
  names:
    - a
    - b
  renames:
    a: c
  rules:
    - record: f
      expr: g
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
  renames:
    d: e
  rules:
    - record: h
      expr: i
`},
	}
}

func NewAmAccessorSA() *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.AlertmanagerAccessorSAName,
			Namespace: mcoNamespace,
		},
		Secrets: []corev1.ObjectReference{
			{Name: config.AlertmanagerAccessorSecretName + "-token-xxx"},
		},
	}
}

func NewAmAccessorTokenSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.AlertmanagerAccessorSecretName + "-token-xxx",
			Namespace: mcoNamespace,
		},
		Data: map[string][]byte{
			"token": []byte("xxxxx"),
		},
	}
}

func newCluster(name string, labels map[string]string) *clusterv1.ManagedCluster {
	return &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
	}
}

func newPullSecret(name, namespace string, data []byte) *corev1.Secret {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			corev1.DockerConfigJsonKey: data,
		},
		StringData: nil,
		Type:       corev1.SecretTypeDockerConfigJson,
	}
}

func newImageRegistry(name, namespace, registryAddress, pullSecret string) *v1alpha1.ManagedClusterImageRegistry {
	return &v1alpha1.ManagedClusterImageRegistry{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: v1alpha1.ImageRegistrySpec{
			Registry:     registryAddress,
			PullSecret:   corev1.LocalObjectReference{Name: pullSecret},
			PlacementRef: v1alpha1.PlacementRef{},
		},
	}
}

func TestManifestWork(t *testing.T) {
	initSchema(t)
	objs := []runtime.Object{
		newTestObsApiRoute(),
		newTestAlertmanagerRoute(),
		newTestIngressController(),
		newTestRouteCASecret(),
		newCASecret(),
		newCertSecret(mcoNamespace),
		NewMetricsAllowListCM(),
		NewMetricsCustomAllowListCM(),
		NewAmAccessorSA(),
		NewAmAccessorTokenSecret(),
		newCluster(clusterName, map[string]string{v1alpha1.ClusterImageRegistryLabel: namespace + ".image_registry"}),
		newImageRegistry("image_registry", namespace, "registry_server", "custorm_pull_secret"),
		newPullSecret("custorm_pull_secret", namespace, []byte("custorm")),
	}
	c := fake.NewFakeClient(objs...)
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get work dir: (%v)", err)
	}
	os.MkdirAll(path.Join(wd, "../../tests"), 0755)
	testManifestsPath := path.Join(wd, "../../tests/manifests")
	manifestsPath := path.Join(wd, "../../manifests")
	os.Setenv("TEMPLATES_PATH", testManifestsPath)
	err = os.Symlink(manifestsPath, testManifestsPath)
	if err != nil {
		t.Fatalf("Failed to create symbollink(%s) to(%s) for the test manifests: (%v)", testManifestsPath, manifestsPath, err)
	}
	works, crdWork, _, err := generateGlobalManifestResources(c, newTestMCO())
	if err != nil {
		t.Fatalf("Failed to get global manifestwork resourc: (%v)", err)
	}
	t.Logf("work size is %d", len(works))
	if hubInfoSecret, err = generateHubInfoSecret(c, config.GetDefaultNamespace(), spokeNameSpace, true); err != nil {
		t.Fatalf("Failed to generate hubInfo secret: (%v)", err)
	}
	err = createManifestWorks(c, nil, namespace, clusterName, newTestMCO(), works, crdWork, endpointMetricsOperatorDeploy, hubInfoSecret, false)
	if err != nil {
		t.Fatalf("Failed to create manifestworks: (%v)", err)
	}
	found := &workv1.ManifestWork{}
	workName := namespace + workNameSuffix
	err = c.Get(context.TODO(), types.NamespacedName{Name: workName, Namespace: namespace}, found)
	if err != nil {
		t.Fatalf("Failed to get manifestwork %s: (%v)", workName, err)
	}
	if len(found.Spec.Workload.Manifests) != workSize-1 {
		t.Fatalf("Wrong size of manifests in the mainfestwork %s: %d", workName, len(found.Spec.Workload.Manifests))
	}

	err = c.Create(context.TODO(), newTestPullSecret())
	if err != nil {
		t.Fatalf("Failed to create pull secret: (%v)", err)
	}
	// reset image pull secret
	pullSecret = nil
	works, crdWork, _, err = generateGlobalManifestResources(c, newTestMCO())
	if err != nil {
		t.Fatalf("Failed to get global manifestwork resourc: (%v)", err)
	}
	err = createManifestWorks(c, nil, namespace, clusterName, newTestMCO(), works, crdWork, endpointMetricsOperatorDeploy, hubInfoSecret, false)
	if err != nil {
		t.Fatalf("Failed to create manifestworks: (%v)", err)
	}
	err = c.Get(context.TODO(), types.NamespacedName{Name: workName, Namespace: namespace}, found)
	if err != nil {
		t.Fatalf("Failed to get manifestwork %s: (%v)", workName, err)
	}
	if len(found.Spec.Workload.Manifests) != workSize {
		t.Fatalf("Wrong size of manifests in the mainfestwork %s: %d", workName, len(found.Spec.Workload.Manifests))
	}

	spokeNameSpace = "spoke-ns"
	err = createManifestWorks(c, nil, namespace, clusterName, newTestMCO(), works, crdWork, endpointMetricsOperatorDeploy, hubInfoSecret, false)
	if err != nil {
		t.Fatalf("Failed to create manifestworks with updated namespace: (%v)", err)
	}

	err = deleteManifestWorks(c, namespace)
	if err != nil {
		t.Fatalf("Failed to delete manifestworks: (%v)", err)
	}
	err = c.Get(context.TODO(), types.NamespacedName{Name: namespace + workNameSuffix, Namespace: namespace}, found)
	if err == nil || !errors.IsNotFound(err) {
		t.Fatalf("Manifestwork not deleted: (%v)", err)
	}

	// set the default pull secret
	pullSecret = newPullSecret("multiclusterhub-operator-pull-secret", namespace, []byte("default"))
	// config the managedcluster to use the custom registry
	managedClusterImageRegistry[clusterName] = "open-cluster-management.io/image-registry=" + namespace + ".image_registry"

	works, crdWork, _, err = generateGlobalManifestResources(c, newTestMCO())
	if err != nil {
		t.Fatalf("Failed to get global manifestwork resourc: (%v)", err)
	}

	if hubInfoSecret, err = generateHubInfoSecret(c, config.GetDefaultNamespace(), spokeNameSpace, true); err != nil {
		t.Fatalf("Failed to generate hubInfo secret: (%v)", err)
	}

	err = createManifestWorks(c, nil, namespace, clusterName, newTestMCO(), works, crdWork, endpointMetricsOperatorDeploy, hubInfoSecret, false)
	if err != nil {
		t.Fatalf("Failed to create manifestworks: (%v)", err)
	}
	found = &workv1.ManifestWork{}
	workName = namespace + workNameSuffix
	err = c.Get(context.TODO(), types.NamespacedName{Name: workName, Namespace: namespace}, found)
	if err != nil {
		t.Fatalf("Failed to get manifestwork %s: (%v)", workName, err)
	}

	// To check pullsecret, endpoint-observability-operator and image list configmap
	for _, manifest := range found.Spec.Workload.Manifests {
		obj := &unstructured.Unstructured{}
		obj.UnmarshalJSON(manifest.Raw)
		if obj.GetKind() == "Secret" && obj.GetName() == "multiclusterhub-operator-pull-secret" {
			if !strings.Contains(string(manifest.Raw), base64.StdEncoding.EncodeToString([]byte("custorm"))) {
				t.Errorf("multiclusterhub-operator-pull-secret should use the custom pull secret")
			}
		}

		if obj.GetKind() == "ConfigMap" && obj.GetName() == "images-list" {
			if !strings.Contains(string(manifest.Raw), "registry_server") {
				t.Errorf("images-list should use the custom registry image")
			}
		}

		if obj.GetKind() == "Deployment" && obj.GetName() == "endpoint-observability-operator" {
			if !strings.Contains(string(manifest.Raw), "registry_server") {
				t.Errorf("endpoint-observability-operator should use the custom registry image")
			}
		}
	}

	if err = os.Remove(testManifestsPath); err != nil {
		t.Fatalf("Failed to delete symbollink(%s) for the test manifests: (%v)", testManifestsPath, err)
	}
	os.Remove(path.Join(wd, "../../tests"))
}

func TestMergeMetrics(t *testing.T) {
	testCaseList := []struct {
		name             string
		defaultAllowlist []string
		customAllowlist  []string
		want             []string
	}{
		{
			name:             "no deleted metrics",
			defaultAllowlist: []string{"a", "b"},
			customAllowlist:  []string{"c"},
			want:             []string{"a", "b", "c"},
		},

		{
			name:             "no default metrics",
			defaultAllowlist: []string{},
			customAllowlist:  []string{"a"},
			want:             []string{"a"},
		},

		{
			name:             "no metrics",
			defaultAllowlist: []string{},
			customAllowlist:  []string{},
			want:             []string{},
		},

		{
			name:             "have deleted metrics",
			defaultAllowlist: []string{"a", "b"},
			customAllowlist:  []string{"c", "-b"},
			want:             []string{"a", "c"},
		},

		{
			name:             "have deleted matches",
			defaultAllowlist: []string{"__name__=\"a\",job=\"a\"", "__name__=\"b\",job=\"b\""},
			customAllowlist:  []string{"-__name__=\"b\",job=\"b\"", "__name__=\"c\",job=\"c\""},
			want:             []string{"__name__=\"a\",job=\"a\"", "__name__=\"c\",job=\"c\""},
		},

		{
			name:             "deleted metrics is no exist",
			defaultAllowlist: []string{"a", "b"},
			customAllowlist:  []string{"c", "-d"},
			want:             []string{"a", "b", "c"},
		},

		{
			name:             "deleted all metrics",
			defaultAllowlist: []string{"a", "b"},
			customAllowlist:  []string{"-a", "-b"},
			want:             []string{},
		},

		{
			name:             "delete custorm metrics",
			defaultAllowlist: []string{"a", "b"},
			customAllowlist:  []string{"a", "-a"},
			want:             []string{"b"},
		},

		{
			name:             "have repeated default metrics",
			defaultAllowlist: []string{"a", "a"},
			customAllowlist:  []string{"a", "-b"},
			want:             []string{"a"},
		},

		{
			name:             "have repeated custom metrics",
			defaultAllowlist: []string{"a"},
			customAllowlist:  []string{"b", "b", "-a"},
			want:             []string{"b"},
		},
	}

	for _, c := range testCaseList {
		got := mergeMetrics(c.defaultAllowlist, c.customAllowlist)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%v: mergeMetrics() = %v, want %v", c.name, got, c.want)
		}
	}
}
