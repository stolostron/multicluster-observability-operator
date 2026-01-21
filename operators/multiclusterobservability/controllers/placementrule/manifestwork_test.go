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
	"time"

	mcoshared "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/shared"
	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	operatorconfig "github.com/stolostron/multicluster-observability-operator/operators/pkg/config"
	"github.com/stolostron/multicluster-observability-operator/operators/pkg/util"
	authv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	kubefake "k8s.io/client-go/kubernetes/fake"
	v1 "k8s.io/client-go/kubernetes/typed/core/v1"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/yaml"
)

const (
	pullSecretName = "test-pull-secret"
	workSize       = 14
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

func newMockKubeClient() kubernetes.Interface {
	// Only include standard Kubernetes objects, not OpenShift-specific objects like Route
	k8sObjects := []runtime.Object{
		NewAmAccessorSA(),             // ServiceAccount (Kubernetes)
		newCASecret(),                 // Secret (Kubernetes)
		newCertSecret(mcoNamespace),   // Secret (Kubernetes)
		NewMetricsAllowListCM(),       // ConfigMap (Kubernetes)
		NewMetricsCustomAllowListCM(), // ConfigMap (Kubernetes)
	}

	return &mockKubeClient{
		Clientset: kubefake.NewSimpleClientset(k8sObjects...),
	}
}

type mockKubeClient struct {
	*kubefake.Clientset
}

func (m *mockKubeClient) CoreV1() v1.CoreV1Interface {
	return &mockCoreV1Client{m.Clientset.CoreV1()}
}

type mockCoreV1Client struct {
	v1.CoreV1Interface
}

func (m *mockCoreV1Client) ServiceAccounts(namespace string) v1.ServiceAccountInterface {
	return &mockServiceAccountInterface{
		ServiceAccountInterface: m.CoreV1Interface.ServiceAccounts(namespace),
	}
}

type mockServiceAccountInterface struct {
	v1.ServiceAccountInterface
}

func (m *mockServiceAccountInterface) CreateToken(ctx context.Context, name string, tokenRequest *authv1.TokenRequest, opts metav1.CreateOptions) (*authv1.TokenRequest, error) {
	// Return a mock token request
	return &authv1.TokenRequest{
		Status: authv1.TokenRequestStatus{
			Token:               "mock-token-12345",
			ExpirationTimestamp: metav1.NewTime(time.Now().Add(24 * time.Hour)),
		},
	}, nil
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
		Data: map[string]string{
			"metrics_list.yaml": `
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
			"uwl_metrics_list.yaml": `
  names:
    - a
    - b
  renames:
    b: d
`,
		},
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
		Data: map[string]string{
			"metrics_list.yaml": `
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
`,
		},
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

// TODO check whether the namespace is correct
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
	_, cc, err := util.GetAllowList(c, config.AllowlistCustomConfigMapName, config.GetDefaultNamespace())
	if err == nil {
		t.Fatalf("the cm is %v, The yaml marshall error is ignored", cc)
	}
}

func getRuntimeObjects() []runtime.Object {
	return []runtime.Object{
		newTestObsApiRoute(),
		newTestAlertmanagerRoute(),
		newTestIngressController(),
		newTestRouteCASecret(),
		newCASecret(),
		newCertSecret(mcoNamespace),
		NewMetricsAllowListCM(),
		NewMetricsCustomAllowListCM(),
		NewAmAccessorSA(),
		newCluster(clusterName, map[string]string{
			ClusterImageRegistriesAnnotation: newAnnotationRegistries([]Registry{
				{Source: "quay.io/stolostron", Mirror: "registry_server/stolostron"},
			},
				fmt.Sprintf("%s.%s", namespace, "custorm_pull_secret")),
		}),
		newPullSecret("custorm_pull_secret", namespace, []byte("custorm")),
	}
}

func TestManifestWork(t *testing.T) {
	initSchema(t)
	objs := getRuntimeObjects()
	c := fake.NewClientBuilder().
		WithRuntimeObjects(objs...).
		Build()

	setupTest(t)
	t.Logf("config.GetDefaultNamespace() returns: '%s'", config.GetDefaultNamespace())
	t.Logf("config.AlertmanagerAccessorSAName returns: '%s'", config.AlertmanagerAccessorSAName)
	// Test with UWM alerting disabled
	mco := newTestMCO()
	mco.Annotations = map[string]string{config.AnnotationDisableUWMAlerting: "true"}
	t.Logf("Mocking kube client")

	mockKubeClient := newMockKubeClient()

	t.Logf("Successfully created mock kube client")
	if mockKubeClient == nil {
		t.Fatalf("Failed to create mock kube client")
	}
	works, crdWork, err := generateGlobalManifestResources(context.Background(), c, mco, mockKubeClient)
	if err != nil {
		t.Fatalf("Failed to get global manifestwork resource: (%v)", err)
	}
	t.Logf("work size is %d", len(works))
	crdMap := map[string]bool{config.IngressControllerCRD: true}
	if hubInfoSecret, err = generateHubInfoSecret(c, config.GetDefaultNamespace(), spokeNameSpace, crdMap, config.IsUWMAlertingDisabledInSpec(mco)); err != nil {
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

	manWork, err := createManifestWorks(
		context.Background(),
		c,
		namespace,
		managedClusterInfo{Name: clusterName, IsLocalCluster: false},
		mco,
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
	if err := createManifestwork(context.Background(), c, manWork); err != nil {
		t.Fatalf("Failed to apply manifestworks: (%v)", err)
	}

	// Verify the hub info secret in the manifestwork
	found := &workv1.ManifestWork{}
	workName := namespace + workNameSuffix
	err = c.Get(context.TODO(), types.NamespacedName{Name: workName, Namespace: namespace}, found)
	if err != nil {
		t.Fatalf("Failed to get manifestwork %s: (%v)", workName, err)
	}

	// Find the hub info secret in the manifestwork
	for _, manifest := range found.Spec.Workload.Manifests {
		obj := &unstructured.Unstructured{}
		obj.UnmarshalJSON(manifest.Raw)
		if obj.GetKind() == "Secret" && obj.GetName() == operatorconfig.HubInfoSecretName {
			hubInfo := &operatorconfig.HubInfo{}
			secretData := obj.Object["data"].(map[string]any)[operatorconfig.HubInfoSecretKey].(string)
			decodedData, err := base64.StdEncoding.DecodeString(secretData)
			if err != nil {
				t.Fatalf("Failed to decode base64 secret data: (%v)", err)
			}
			err = yaml.Unmarshal(decodedData, hubInfo)
			if err != nil {
				t.Fatalf("Failed to unmarshal hub info secret: (%v)", err)
			}
			if !hubInfo.UWMAlertingDisabled {
				t.Fatalf("UWM alerting should be disabled in the hub info secret")
			}
		}
	}

	// Test with UWM alerting enabled
	mco.Annotations = map[string]string{config.AnnotationDisableUWMAlerting: "false"}
	works, crdWork, err = generateGlobalManifestResources(context.Background(), c, mco, mockKubeClient)
	if err != nil {
		t.Fatalf("Failed to get global manifestwork resource: (%v)", err)
	}
	if hubInfoSecret, err = generateHubInfoSecret(c, config.GetDefaultNamespace(), spokeNameSpace, crdMap, config.IsUWMAlertingDisabledInSpec(mco)); err != nil {
		t.Fatalf("Failed to generate hubInfo secret: (%v)", err)
	}

	manWork, err = createManifestWorks(
		context.Background(),
		c,
		namespace,
		managedClusterInfo{Name: clusterName, IsLocalCluster: false},
		mco,
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
	if err := createManifestwork(context.Background(), c, manWork); err != nil {
		t.Fatalf("Failed to apply manifestworks: (%v)", err)
	}

	// Verify the hub info secret in the manifestwork
	err = c.Get(context.TODO(), types.NamespacedName{Name: workName, Namespace: namespace}, found)
	if err != nil {
		t.Fatalf("Failed to get manifestwork %s: (%v)", workName, err)
	}

	// Find the hub info secret in the manifestwork
	for _, manifest := range found.Spec.Workload.Manifests {
		obj := &unstructured.Unstructured{}
		obj.UnmarshalJSON(manifest.Raw)
		if obj.GetKind() == "Secret" && obj.GetName() == operatorconfig.HubInfoSecretName {
			hubInfo := &operatorconfig.HubInfo{}
			secretData := obj.Object["data"].(map[string]any)[operatorconfig.HubInfoSecretKey].(string)
			decodedData, err := base64.StdEncoding.DecodeString(secretData)
			if err != nil {
				t.Fatalf("Failed to decode base64 secret data: (%v)", err)
			}
			err = yaml.Unmarshal(decodedData, hubInfo)
			if err != nil {
				t.Fatalf("Failed to unmarshal hub info secret: (%v)", err)
			}
			if hubInfo.UWMAlertingDisabled {
				t.Fatalf("UWM alerting should be enabled in the hub info secret")
			}
		}
	}

	amAccessorFound := false
	// Check that AlertmanagerAccessorSecret contains the token and expiration
	for _, manifest := range found.Spec.Workload.Manifests {
		obj := &unstructured.Unstructured{}
		obj.UnmarshalJSON(manifest.Raw)
		if obj.GetKind() == "Secret" && obj.GetName() == config.AlertmanagerAccessorSecretName {
			amAccessorFound = true
			if data, exists := obj.Object["data"]; exists {
				dataMap := data.(map[string]any)
				if _, exists := dataMap["token"]; !exists {
					t.Fatalf("Failed to find token in amAccessorSecret")
				}
			} else {
				t.Fatalf("Failed to find data in amAccessorSecret")
			}
			// check for token-expiration
			if metadata, exists := obj.Object["metadata"]; exists {
				metadataMap := metadata.(map[string]any)
				if annotations, exists := metadataMap["annotations"]; exists {
					annotationsMap := annotations.(map[string]any)
					if tokenExpiration, exists := annotationsMap[amTokenExpiration]; exists {
						_, err := time.Parse(time.RFC3339, tokenExpiration.(string))
						if err != nil {
							t.Fatalf("Failed to parse token-expiration from secret: %v", err)
						}
					} else {
						t.Fatalf("Failed to find token-expiration in amAccessorSecret")
					}
				} else {
					t.Fatalf("Failed to find annotations in amAccessorSecret")
				}
			} else {
				t.Fatalf("Failed to find metadata in amAccessorSecret")
			}
			break
		}
	}
	if !amAccessorFound {
		t.Fatalf("Failed to find amAccessorSecret in the manifestwork")
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

	found = &workv1.ManifestWork{}
	workName = namespace + workNameSuffix
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
	works, crdWork, err = generateGlobalManifestResources(context.Background(), c, newTestMCO(), mockKubeClient)
	if err != nil {
		t.Fatalf("Failed to get global manifestwork resource: (%v)", err)
	}
	manWork, err = createManifestWorks(
		context.Background(),
		c,
		namespace,
		managedClusterInfo{Name: clusterName, IsLocalCluster: false},
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
	if err := createManifestwork(context.Background(), c, manWork); err != nil {
		t.Fatalf("Failed to apply manifestworks: (%v)", err)
	}
	err = c.Get(context.TODO(), types.NamespacedName{Name: workName, Namespace: namespace}, found)
	if err != nil {
		t.Fatalf("Failed to get manifestwork %s: (%v)", workName, err)
	}
	if len(found.Spec.Workload.Manifests) != workSize {
		t.Fatalf("Wrong size of manifests in the mainfestwork %s: %d", workName, len(found.Spec.Workload.Manifests))
	}

	spokeNameSpace = "spoke-ns"
	manWork, err = createManifestWorks(
		context.Background(),
		c,
		namespace,
		managedClusterInfo{Name: clusterName, IsLocalCluster: false},
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
		t.Fatalf("Failed to create manifestworks with updated namespace: (%v)", err)
	}
	if err := createManifestwork(context.Background(), c, manWork); err != nil {
		t.Fatalf("Failed to apply manifestworks: (%v)", err)
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

	works, crdWork, err = generateGlobalManifestResources(context.Background(), c, newTestMCO(), mockKubeClient)
	if err != nil {
		t.Fatalf("Failed to get global manifestwork resource: (%v)", err)
	}
	if hubInfoSecret, err = generateHubInfoSecret(c, config.GetDefaultNamespace(), spokeNameSpace, crdMap, config.IsUWMAlertingDisabledInSpec(mco)); err != nil {
		t.Fatalf("Failed to generate hubInfo secret: (%v)", err)
	}

	manWork, err = createManifestWorks(
		context.Background(),
		c,
		namespace,
		managedClusterInfo{Name: clusterName, IsLocalCluster: false},
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
	if err := createManifestwork(context.Background(), c, manWork); err != nil {
		t.Fatalf("Failed to apply manifestworks: (%v)", err)
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

			// Check if HTTP_PROXY, HTTPS_PROXY, and NO_PROXY are present and set correctly
			containers := obj.Object["spec"].(map[string]any)["template"].(map[string]any)["spec"].(map[string]any)["containers"].([]any)
			for _, container := range containers {
				c := container.(map[string]any)
				if c["name"] == "endpoint-observability-operator" {
					foundHTTPProxy := false
					foundHTTPSProxy := false
					foundNOProxy := false
					foundCABundle := false
					// rewrite the below to check for env variables
					env := c["env"].([]any)
					for _, e := range env {
						e := e.(map[string]any)
						if e["name"] == "HTTP_PROXY" {
							foundHTTPProxy = true
							if e["value"] != "http://foo.com" {
								t.Errorf("HTTP_PROXY is not set correctly: expected %s, got %s", "http://foo.com", e["value"])
							}
						} else if e["name"] == "HTTPS_PROXY" {
							foundHTTPSProxy = true
							if e["value"] != "https://foo.com" {
								t.Errorf("HTTPS_PROXY is not set correctly: expected %s, got %s", "https://foo.com", e["value"])
							}
						} else if e["name"] == "NO_PROXY" {
							foundNOProxy = true
							if e["value"] != "bar.com" {
								t.Errorf("NO_PROXY is not set correctly: expected %s, got %s", "bar.com", e["value"])
							}
						} else if e["name"] == "HTTPS_PROXY_CA_BUNDLE" {
							foundCABundle = true
							if e["value"] != base64.StdEncoding.EncodeToString([]byte{0x01, 0x02, 0x03, 0xAB, 0xCD, 0xEF}) {
								t.Errorf("HTTPS_PROXY_CA_BUNDLE is not set correctly: expected %s, got %s", base64.StdEncoding.EncodeToString([]byte{0x01, 0x02, 0x03, 0xAB, 0xCD, 0xEF}), e["value"])
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

func TestUpdateManagedAnnotations(t *testing.T) {
	tests := []struct {
		name         string
		targetAnno   map[string]string
		sourceAnno   map[string]string
		expectedAnno map[string]string
		description  string
	}{
		{
			name:       "add our annotations to empty target",
			targetAnno: nil,
			sourceAnno: map[string]string{
				workPostponeDeleteAnnoKey:                  "",
				workv1.ManifestConfigSpecHashAnnotationKey: `{"config1":"hash1"}`,
			},
			expectedAnno: map[string]string{
				workPostponeDeleteAnnoKey:                  "",
				workv1.ManifestConfigSpecHashAnnotationKey: `{"config1":"hash1"}`,
			},
			description: "Should add both our annotations when target is empty",
		},
		{
			name: "update our annotations, preserve others",
			targetAnno: map[string]string{
				workPostponeDeleteAnnoKey:                  "",
				workv1.ManifestConfigSpecHashAnnotationKey: `{"config1":"old-hash"}`,
				"addon-framework.io/some-annotation":       "framework-value",
				"other-controller/annotation":              "other-value",
			},
			sourceAnno: map[string]string{
				workPostponeDeleteAnnoKey:                  "",
				workv1.ManifestConfigSpecHashAnnotationKey: `{"config1":"new-hash"}`,
			},
			expectedAnno: map[string]string{
				workPostponeDeleteAnnoKey:                  "",
				workv1.ManifestConfigSpecHashAnnotationKey: `{"config1":"new-hash"}`,
				"addon-framework.io/some-annotation":       "framework-value",
				"other-controller/annotation":              "other-value",
			},
			description: "Should update our annotations but preserve framework and other controller annotations",
		},
		{
			name: "remove our annotation when not in source",
			targetAnno: map[string]string{
				workPostponeDeleteAnnoKey:                  "",
				workv1.ManifestConfigSpecHashAnnotationKey: `{"config1":"hash1"}`,
				"addon-framework.io/some-annotation":       "framework-value",
			},
			sourceAnno: map[string]string{
				workPostponeDeleteAnnoKey: "",
				// config-spec-hash not in source - should be removed
			},
			expectedAnno: map[string]string{
				workPostponeDeleteAnnoKey:            "",
				"addon-framework.io/some-annotation": "framework-value",
			},
			description: "Should remove config-spec-hash when not in source, but preserve framework annotations",
		},
		{
			name: "preserve framework annotations with same prefix",
			targetAnno: map[string]string{
				"open-cluster-management.io/framework-annotation": "framework-value",
			},
			sourceAnno: map[string]string{
				workPostponeDeleteAnnoKey:                  "",
				workv1.ManifestConfigSpecHashAnnotationKey: `{"config1":"hash1"}`,
			},
			expectedAnno: map[string]string{
				workPostponeDeleteAnnoKey:                         "",
				workv1.ManifestConfigSpecHashAnnotationKey:        `{"config1":"hash1"}`,
				"open-cluster-management.io/framework-annotation": "framework-value",
			},
			description: "Should not remove framework annotations even if they share open-cluster-management.io prefix",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := &workv1.ManifestWork{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: tt.targetAnno,
				},
			}
			source := &workv1.ManifestWork{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: tt.sourceAnno,
				},
			}

			updateManagedAnnotations(target, source)

			if len(target.Annotations) != len(tt.expectedAnno) {
				t.Errorf("%s: annotation count mismatch. got %d, want %d\nGot: %v\nWant: %v",
					tt.description, len(target.Annotations), len(tt.expectedAnno), target.Annotations, tt.expectedAnno)
			}

			for key, expectedVal := range tt.expectedAnno {
				if actualVal, exists := target.Annotations[key]; !exists {
					t.Errorf("%s: missing annotation %q", tt.description, key)
				} else if actualVal != expectedVal {
					t.Errorf("%s: annotation %q value mismatch. got %q, want %q",
						tt.description, key, actualVal, expectedVal)
				}
			}

			// Verify no unexpected annotations
			for key := range target.Annotations {
				if _, expected := tt.expectedAnno[key]; !expected {
					t.Errorf("%s: unexpected annotation %q with value %q",
						tt.description, key, target.Annotations[key])
				}
			}
		})
	}
}

func TestShouldUpdateManifestWork(t *testing.T) {
	tests := []struct {
		name         string
		foundAnno    map[string]string
		desiredAnno  map[string]string
		shouldUpdate bool
		description  string
	}{
		{
			name: "no update needed when our annotations match",
			foundAnno: map[string]string{
				workPostponeDeleteAnnoKey:                  "",
				workv1.ManifestConfigSpecHashAnnotationKey: `{"config1":"hash1"}`,
				"addon-framework.io/some-annotation":       "framework-value",
			},
			desiredAnno: map[string]string{
				workPostponeDeleteAnnoKey:                  "",
				workv1.ManifestConfigSpecHashAnnotationKey: `{"config1":"hash1"}`,
			},
			shouldUpdate: false,
			description:  "Should not trigger update when our annotations match (ignore framework annotations)",
		},
		{
			name: "update needed when our annotation value changes",
			foundAnno: map[string]string{
				workPostponeDeleteAnnoKey:                  "",
				workv1.ManifestConfigSpecHashAnnotationKey: `{"config1":"old-hash"}`,
			},
			desiredAnno: map[string]string{
				workPostponeDeleteAnnoKey:                  "",
				workv1.ManifestConfigSpecHashAnnotationKey: `{"config1":"new-hash"}`,
			},
			shouldUpdate: true,
			description:  "Should trigger update when config-spec-hash value changes",
		},
		{
			name: "update needed when our annotation added",
			foundAnno: map[string]string{
				workPostponeDeleteAnnoKey: "",
			},
			desiredAnno: map[string]string{
				workPostponeDeleteAnnoKey:                  "",
				workv1.ManifestConfigSpecHashAnnotationKey: `{"config1":"hash1"}`,
			},
			shouldUpdate: true,
			description:  "Should trigger update when config-spec-hash is added",
		},
		{
			name: "update needed when our annotation removed",
			foundAnno: map[string]string{
				workPostponeDeleteAnnoKey:                  "",
				workv1.ManifestConfigSpecHashAnnotationKey: `{"config1":"hash1"}`,
			},
			desiredAnno: map[string]string{
				workPostponeDeleteAnnoKey: "",
			},
			shouldUpdate: true,
			description:  "Should trigger update when config-spec-hash is removed",
		},
		{
			name: "no update when framework annotation added",
			foundAnno: map[string]string{
				workPostponeDeleteAnnoKey:                  "",
				workv1.ManifestConfigSpecHashAnnotationKey: `{"config1":"hash1"}`,
			},
			desiredAnno: map[string]string{
				workPostponeDeleteAnnoKey:                  "",
				workv1.ManifestConfigSpecHashAnnotationKey: `{"config1":"hash1"}`,
				// Framework adds annotation - we don't include it in our desired state
			},
			shouldUpdate: false,
			description:  "Should not trigger update when framework adds their own annotation",
		},
		{
			name: "no update when framework annotation changes",
			foundAnno: map[string]string{
				workPostponeDeleteAnnoKey:                  "",
				workv1.ManifestConfigSpecHashAnnotationKey: `{"config1":"hash1"}`,
				"addon-framework.io/some-annotation":       "old-value",
			},
			desiredAnno: map[string]string{
				workPostponeDeleteAnnoKey:                  "",
				workv1.ManifestConfigSpecHashAnnotationKey: `{"config1":"hash1"}`,
			},
			shouldUpdate: false,
			description:  "Should not trigger update when framework changes their annotation (we ignore it)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			found := &workv1.ManifestWork{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: tt.foundAnno,
					Labels:      map[string]string{"test": "label"},
				},
				Spec: workv1.ManifestWorkSpec{
					Workload: workv1.ManifestsTemplate{
						Manifests: []workv1.Manifest{
							{RawExtension: runtime.RawExtension{Raw: []byte(`{"kind":"Secret"}`)}},
						},
					},
				},
			}
			desired := &workv1.ManifestWork{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: tt.desiredAnno,
					Labels:      map[string]string{"test": "label"},
				},
				Spec: workv1.ManifestWorkSpec{
					Workload: workv1.ManifestsTemplate{
						Manifests: []workv1.Manifest{
							{RawExtension: runtime.RawExtension{Raw: []byte(`{"kind":"Secret"}`)}},
						},
					},
				},
			}

			result := shouldUpdateManifestWork(desired, found)
			if result != tt.shouldUpdate {
				t.Errorf("%s: shouldUpdate mismatch. got %v, want %v\nFound: %v\nDesired: %v",
					tt.description, result, tt.shouldUpdate, tt.foundAnno, tt.desiredAnno)
			}
		})
	}
}
