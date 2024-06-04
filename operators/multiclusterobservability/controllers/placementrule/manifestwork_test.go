// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package placementrule

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"

	mcoshared "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/shared"
	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	operatorconfig "github.com/stolostron/multicluster-observability-operator/operators/pkg/config"
	"github.com/stolostron/multicluster-observability-operator/operators/pkg/util"
)

const (
	pullSecretName = "test-pull-secret"
	workSize       = 13
)

func init() {
	os.Setenv("UNIT_TEST", "true")
}

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

func newTestMCOWithAlertDisableAnnotation() *mcov1beta2.MultiClusterObservability {
	return &mcov1beta2.MultiClusterObservability{
		ObjectMeta: metav1.ObjectMeta{
			Name:        mcoName,
			Annotations: map[string]string{config.AnnotationDisableMCOAlerting: "true"},
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
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
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
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
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
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
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
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
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
  recording_rules:
    - record: f
      expr: g
  collect_rules:
    - group: keepGroup
      annotations:
        description:
      selector:
        matchExpressions:
          - key: clusterType
            operator: NotIn
            values: ["SNO"]
      rules:
      - collect: c
        annotations:
          description:
        expr: e
        for: 2m
        dynamic_metrics:
          matches:
            - __name__="foo"
    - group: discardGroup
      annotations:
        description:
      selector:
        matchExpressions:
          - key: clusterType
            operator: In
            values: ["SNO"]
        rules:
        - collect: d
          annotations:
            description:
          expr: d
          for: 2m
          dynamic_metrics:
            names:
              - foobar_metric
`,
			"ocp311_metrics_list.yaml": `
  names:
    - a
    - b
  renames:
    a: c
  recording_rules:
    - record: f
      expr: g
`,
			"uwl_metrics_list.yaml": `
  names:
    - a
    - b
  renames:
    b: d
`},
	}
}

func NewMetricsCustomAllowListCM() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
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
  collect_rules:
    - name: -discard
`,
			"uwl_metrics_list.yaml": `
  names:
    - c
    - d
  renames:
    a: c
`},
	}
}

func NewCorruptMetricsCustomAllowListCM() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.AllowlistCustomConfigMapName,
			Namespace: mcoNamespace,
		},
		Data: map[string]string{"uwl_metrics_list.yaml": `
  names:
    d: e
`},
	}
}

func NewAmAccessorSA() *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ServiceAccount",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.AlertmanagerAccessorSAName,
			Namespace: mcoNamespace,
		},
		Secrets: []corev1.ObjectReference{
			// Test ocp 4.11 behavior where the service accounts won't list service account secrets any longger
			// {Name: config.AlertmanagerAccessorSecretName + "-token-xxx"},
		},
	}
}

func NewAmAccessorTokenSecret() *corev1.Secret {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.AlertmanagerAccessorSecretName + "-token-xxx",
			Namespace: mcoNamespace,
		},
		Data: map[string][]byte{
			"token": []byte("xxxxx"),
		},
		Type: corev1.SecretTypeServiceAccountToken,
	}
}

func newCluster(name string, annotation map[string]string) *clusterv1.ManagedCluster {
	return &clusterv1.ManagedCluster{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ManagedCluster",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Annotations: annotation,
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

func TestGetAllowList(t *testing.T) {
	initSchema(t)
	objs := []runtime.Object{
		NewMetricsAllowListCM(),
		NewCorruptMetricsCustomAllowListCM(),
	}
	c := fake.NewClientBuilder().WithRuntimeObjects(objs...).Build()
	_, _, cc, err := util.GetAllowList(c, config.AllowlistCustomConfigMapName, config.GetDefaultNamespace())
	if err == nil {
		t.Fatalf("the cm is %v, The yaml marshall error is ignored", cc)
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
		newCluster(clusterName, map[string]string{
			ClusterImageRegistriesAnnotation: newAnnotationRegistries([]Registry{
				{Source: "quay.io/stolostron", Mirror: "registry_server/stolostron"}},
				fmt.Sprintf("%s.%s", namespace, "custorm_pull_secret"))}),
		newPullSecret("custorm_pull_secret", namespace, []byte("custorm")),
	}
	c := fake.NewClientBuilder().
		WithRuntimeObjects(objs...).
		Build()

	defer setupTest(t)()

	works, crdWork, _, err := generateGlobalManifestResources(c, newTestMCO())
	if err != nil {
		t.Fatalf("Failed to get global manifestwork resource: (%v)", err)
	}
	t.Logf("work size is %d", len(works))
	if hubInfoSecret, err = generateHubInfoSecret(c, config.GetDefaultNamespace(), spokeNameSpace, true); err != nil {
		t.Fatalf("Failed to generate hubInfo secret: (%v)", err)
	}

	addonConfig := &addonv1alpha1.AddOnDeploymentConfig{
		TypeMeta: metav1.TypeMeta{
			Kind:       "AddOnDeploymentConfig",
			APIVersion: "v1alpha1",
		},
		Spec: addonv1alpha1.AddOnDeploymentConfigSpec{
			NodePlacement: &addonv1alpha1.NodePlacement{
				NodeSelector: map[string]string{
					"kubernetes.io/os": "linux",
				},
				Tolerations: []corev1.Toleration{
					{
						Key:      "foo",
						Operator: corev1.TolerationOpExists,
						Effect:   corev1.TaintEffectNoExecute,
					},
				},
			},
			ProxyConfig: addonv1alpha1.ProxyConfig{
				HTTPProxy:  "http://foo.com",
				HTTPSProxy: "https://foo.com",
				NoProxy:    "bar.com",
				CABundle:   []byte{0x01, 0x02, 0x03, 0xAB, 0xCD, 0xEF},
			},
		},
	}

	err = createManifestWorks(
		c,
		namespace,
		clusterName,
		newTestMCO(),
		works,
		metricsAllowlistConfigMap,
		crdWork,
		endpointMetricsOperatorDeploy,
		hubInfoSecret,
		addonConfig,
		false,
	)
	if err != nil {
		t.Fatalf("Failed to create manifestworks: (%v)", err)
	}

	annotations := endpointMetricsOperatorDeploy.Spec.Template.Annotations
	v, f := annotations[operatorconfig.WorkloadPartitioningPodAnnotationKey]
	if !f || v != operatorconfig.WorkloadPodExpectedValueJSON {
		t.Fatalf("Failed to find annotation %v: %v on the pod spec of deployment: %v",
			operatorconfig.WorkloadPartitioningPodAnnotationKey,
			operatorconfig.WorkloadPodExpectedValueJSON,
			endpointMetricsOperatorDeploy.Name,
		)
	}

	// Check if HTTP_PROXY, HTTPS_PROXY, and NO_PROXY are present and set correctly
	containers := endpointMetricsOperatorDeploy.Spec.Template.Spec.Containers
	for _, container := range containers {
		if container.Name == "endpoint-observability-operator" {
			env := container.Env
			foundHTTPProxy := false
			foundHTTPSProxy := false
			foundNOProxy := false
			foundCABundle := false
			for _, e := range env {
				if e.Name == "HTTP_PROXY" {
					foundHTTPProxy = true
					if e.Value != "http://foo.com" {
						t.Fatalf("HTTP_PROXY is not set correctly: expected %s, got %s", "http://foo.com", e.Value)
					}
				} else if e.Name == "HTTPS_PROXY" {
					foundHTTPSProxy = true
					if e.Value != "https://foo.com" {
						t.Fatalf("HTTPS_PROXY is not set correctly: expected %s, got %s", "https://foo.com", e.Value)
					}
				} else if e.Name == "NO_PROXY" {
					foundNOProxy = true
					if e.Value != "bar.com" {
						t.Fatalf("NO_PROXY is not set correctly: expected %s, got %s", "bar.com", e.Value)
					}
				} else if e.Name == "HTTPS_PROXY_CA_BUNDLE" {
					foundCABundle = true
					if e.Value != base64.StdEncoding.EncodeToString([]byte{0x01, 0x02, 0x03, 0xAB, 0xCD, 0xEF}) {
						t.Fatalf("HTTPS_PROXY_CA_BUNDLE is not set correctly")
					}
				}
			}
			if !foundHTTPProxy {
				t.Fatalf("HTTP_PROXY is not present in env")
			}
			if !foundHTTPSProxy {
				t.Fatalf("HTTPS_PROXY is not present in env")
			}
			if !foundNOProxy {
				t.Fatalf("NO_PROXY is not present in env")
			}
			if !foundCABundle {
				t.Fatalf("HTTPS_PROXY_CA_BUNDLE is not present in env")
			}
		}
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
		t.Fatalf("Failed to get global manifestwork resource: (%v)", err)
	}
	err = createManifestWorks(c, namespace, clusterName, newTestMCO(), works, metricsAllowlistConfigMap, crdWork, endpointMetricsOperatorDeploy, hubInfoSecret, addonConfig, false)
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
	err = createManifestWorks(c, namespace, clusterName, newTestMCO(), works, metricsAllowlistConfigMap, crdWork, endpointMetricsOperatorDeploy, hubInfoSecret, addonConfig, false)
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
	managedClusterImageRegistryMutex.Lock()
	managedClusterImageRegistry[clusterName] = "open-cluster-management.io/image-registry=" + namespace + ".image_registry"
	managedClusterImageRegistryMutex.Unlock()

	works, crdWork, _, err = generateGlobalManifestResources(c, newTestMCO())
	if err != nil {
		t.Fatalf("Failed to get global manifestwork resource: (%v)", err)
	}
	if hubInfoSecret, err = generateHubInfoSecret(c, config.GetDefaultNamespace(), spokeNameSpace, true); err != nil {
		t.Fatalf("Failed to generate hubInfo secret: (%v)", err)
	}

	err = createManifestWorks(c, namespace, clusterName, newTestMCO(), works, metricsAllowlistConfigMap, crdWork, endpointMetricsOperatorDeploy, hubInfoSecret, addonConfig, false)
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
}

func TestLogSizeErrorDetails(t *testing.T) {
	logSizeErrorDetails("the size of manifests is 600000", &workv1.ManifestWork{
		Spec: workv1.ManifestWorkSpec{
			Workload: workv1.ManifestsTemplate{
				Manifests: []workv1.Manifest{
					{
						RawExtension: runtime.RawExtension{
							Object: NewMetricsAllowListCM(),
						},
					},
				},
			},
		},
	})
}
