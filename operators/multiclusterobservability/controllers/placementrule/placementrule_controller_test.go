// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package placementrule

import (
	"context"
	"os"
	"path"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"

	ocinfrav1 "github.com/openshift/api/config/v1"
	operatorv1 "github.com/openshift/api/operator/v1"
	routev1 "github.com/openshift/api/route/v1"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	mcov1beta1 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/rendering/templates"

	operatorutil "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/util"
	operatorconfig "github.com/stolostron/multicluster-observability-operator/operators/pkg/config"
	"github.com/stolostron/multicluster-observability-operator/operators/pkg/util"
	mchv1 "github.com/stolostron/multiclusterhub-operator/api/v1"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"
)

const (
	namespace              = "test-ns"
	namespace2             = "test-ns-2"
	clusterName            = "cluster1"
	clusterName2           = "cluster2"
	mcoName                = "test-mco"
	defaultAddonConfigName = "test-default"
	addonConfigName        = "test"
)

var (
	mcoNamespace = config.GetDefaultNamespace()
)

func initSchema(t *testing.T) {
	s := scheme.Scheme
	if err := clusterv1.AddToScheme(s); err != nil {
		t.Fatalf("Unable to add placementrule scheme: (%v)", err)
	}
	if err := mcov1beta2.AddToScheme(s); err != nil {
		t.Fatalf("Unable to add mcov1beta2 scheme: (%v)", err)
	}
	if err := mcov1beta1.AddToScheme(s); err != nil {
		t.Fatalf("Unable to add mcov1beta1 scheme: (%v)", err)
	}
	if err := routev1.AddToScheme(s); err != nil {
		t.Fatalf("Unable to add routev1 scheme: (%v)", err)
	}
	if err := operatorv1.AddToScheme(s); err != nil {
		t.Fatalf("Unable to add routev1 scheme: (%v)", err)
	}
	if err := ocinfrav1.AddToScheme(s); err != nil {
		t.Fatalf("Unable to add ocinfrav1 scheme: (%v)", err)
	}
	if err := workv1.AddToScheme(s); err != nil {
		t.Fatalf("Unable to add workv1 scheme: (%v)", err)
	}
	if err := mchv1.SchemeBuilder.AddToScheme(s); err != nil {
		t.Fatalf("Unable to add mchv1 scheme: (%v)", err)
	}
}

var testImagemanifestsMap = map[string]string{
	"endpoint_monitoring_operator": "test.io/endpoint-monitoring:test",
	"metrics_collector":            "test.io/metrics-collector:test",
}

func newTestImageManifestsConfigMap(namespace, version string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.ImageManifestConfigMapNamePrefix + version,
			Namespace: namespace,
			Labels: map[string]string{
				config.OCMManifestConfigMapTypeLabelKey:    config.OCMManifestConfigMapTypeLabelValue,
				config.OCMManifestConfigMapVersionLabelKey: version,
			},
		},
		Data: testImagemanifestsMap,
	}
}

func newMCHInstanceWithVersion(namespace, version string) *mchv1.MultiClusterHub {
	return &mchv1.MultiClusterHub{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: namespace,
		},
		Spec: mchv1.MultiClusterHubSpec{},
		Status: mchv1.MultiClusterHubStatus{
			CurrentVersion: version,
			DesiredVersion: version,
		},
	}
}

func newConsoleRoute() *routev1.Route {
	return &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "multicloud-console",
			Namespace: config.GetMCONamespace(),
		},
		Spec: routev1.RouteSpec{
			Host: "console",
		},
	}
}

func setupTest(t *testing.T) func() {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get work dir: (%v)", err)
	}
	t.Log("begin setupTest")
	os.MkdirAll(path.Join(wd, "../../tests"), 0755)
	testManifestsPath := path.Join(wd, "../../tests/manifests")
	manifestsPath := path.Join(wd, "../../manifests")
	os.Setenv("TEMPLATES_PATH", testManifestsPath)
	templates.ResetTemplates()
	//clean up the manifest path if left over from previous test
	if fi, err := os.Lstat(testManifestsPath); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		if err = os.Remove(testManifestsPath); err != nil {
			t.Logf("Failed to delete symlink(%s) for the test manifests: (%v)", testManifestsPath, err)
		}
	}
	err = os.Symlink(manifestsPath, testManifestsPath)
	if err != nil {
		t.Fatalf("Failed to create symbollink(%s) to(%s) for the test manifests: (%v)", testManifestsPath, manifestsPath, err)
	}
	t.Log("setupTest done")

	return func() {
		t.Log("begin teardownTest")
		if err = os.Remove(testManifestsPath); err != nil {
			t.Logf("Failed to delete symbollink(%s) for the test manifests: (%v)", testManifestsPath, err)
		}
		os.Remove(path.Join(wd, "../../tests"))
		os.Unsetenv("TEMPLATES_PATH")
		t.Log("teardownTest done")
	}
}

func TestObservabilityAddonController(t *testing.T) {
	s := scheme.Scheme
	addonv1alpha1.AddToScheme(s)
	initSchema(t)
	config.SetMonitoringCRName(mcoName)
	mco := newTestMCO()
	pull := newTestPullSecret()
	deprecatedRole := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "endpoint-observability-role",
			Namespace: namespace,
			Labels: map[string]string{
				ownerLabelKey: ownerLabelValue,
			},
		},
	}
	objs := []runtime.Object{mco, pull, newConsoleRoute(), newTestObsApiRoute(), newTestAlertmanagerRoute(), newTestIngressController(), newTestRouteCASecret(), newCASecret(), newCertSecret(mcoNamespace), NewMetricsAllowListCM(),
		NewAmAccessorSA(), NewAmAccessorTokenSecret(), deprecatedRole, newClusterMgmtAddon(),
		newAddonDeploymentConfig(defaultAddonConfigName, namespace), newAddonDeploymentConfig(addonConfigName, namespace)}
	c := fake.NewClientBuilder().WithRuntimeObjects(objs...).Build()
	r := &PlacementRuleReconciler{Client: c, Scheme: s, CRDMap: map[string]bool{config.IngressControllerCRD: true}}

	defer setupTest(t)()

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      config.GetDefaultCRName(),
			Namespace: mcoNamespace,
		},
	}

	managedClusterList.Store(namespace, "4")
	managedClusterList.Store(namespace2, "4")

	_, err := r.Reconcile(context.TODO(), req)
	if err != nil {
		t.Fatalf("reconcile: (%v)", err)
	}
	found := &workv1.ManifestWork{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: namespace + workNameSuffix, Namespace: namespace}, found)
	if err != nil {
		t.Fatalf("Failed to get manifestwork %s: (%v)", namespace, err)
	}
	err = c.Get(context.TODO(), types.NamespacedName{Name: namespace2 + workNameSuffix, Namespace: namespace2}, found)
	if err != nil {
		t.Fatalf("Failed to get manifestwork for %s: (%v)", namespace2, err)
	}
	foundRole := &rbacv1.Role{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: "endpoint-observability-role", Namespace: namespace}, foundRole)
	if err == nil || !errors.IsNotFound(err) {
		t.Fatalf("Deprecated role not removed")
	}

	managedClusterList.Range(func(key, value interface{}) bool {
		managedClusterList.Delete(key)
		return true
	})
	managedClusterList.Store(namespace, "4")
	_, err = r.Reconcile(context.TODO(), req)
	if err != nil {
		t.Fatalf("reconcile: (%v)", err)
	}

	err = c.Get(context.TODO(), types.NamespacedName{Name: namespace2 + workNameSuffix, Namespace: namespace2}, found)
	if err == nil || !errors.IsNotFound(err) {
		t.Fatalf("Failed to delete manifestwork for cluster2: (%v)", err)
	}

	err = c.Delete(context.TODO(), pull)
	if err != nil {
		t.Fatalf("Failed to delete pull secret: (%v)", err)
	}
	_, err = r.Reconcile(context.TODO(), req)
	if err != nil {
		t.Fatalf("reconcile: (%v)", err)
	}
	foundAddonDeploymentConfig := &addonv1alpha1.AddOnDeploymentConfig{}
	err = c.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: defaultAddonConfigName}, foundAddonDeploymentConfig)
	if err != nil {
		t.Fatalf("Failed to get addondeploymentconfig %s: (%v)", name, err)
	}

	//Change proxyconfig in addondeploymentconfig
	foundAddonDeploymentConfig.Spec.ProxyConfig = addonv1alpha1.ProxyConfig{
		HTTPProxy:  "http://test1.com",
		HTTPSProxy: "https://test1.com",
		NoProxy:    "test.com",
	}

	err = c.Update(context.TODO(), foundAddonDeploymentConfig)
	if err != nil {
		t.Fatalf("Failed to update addondeploymentconfig %s: (%v)", name, err)
	}

	req = ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name: config.AddonDeploymentConfigUpdateName,
		},
	}

	_, err = r.Reconcile(context.TODO(), req)
	if err != nil {
		t.Fatalf("reconcile after updating addondeploymentconfig: (%v)", err)
	}

	foundManifestwork := &workv1.ManifestWork{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: namespace + workNameSuffix, Namespace: namespace}, foundManifestwork)
	if err != nil {
		t.Fatalf("Failed to get manifestwork %s: (%v)", namespace, err)
	}
	for _, manifest := range foundManifestwork.Spec.Workload.Manifests {
		obj, _ := util.GetObject(manifest.RawExtension)
		if obj.GetObjectKind().GroupVersionKind().Kind == "Deployment" {
			//Check the proxy env variables
			deployment := obj.(*appsv1.Deployment)
			spec := deployment.Spec.Template.Spec
			for _, c := range spec.Containers {
				if c.Name == "endpoint-observability-operator" {
					env := c.Env
					for _, e := range env {
						if e.Name == "HTTP_PROXY" {
							if e.Value != "http://test1.com" {
								t.Fatalf("HTTP_PROXY is not set correctly: expected %s, got %s", "http://test1.com", e.Value)
							}
						} else if e.Name == "HTTPS_PROXY" {
							if e.Value != "https://test1.com" {
								t.Fatalf("HTTPS_PROXY is not set correctly: expected %s, got %s", "https://test1.com", e.Value)
							}
						} else if e.Name == "NO_PROXY" {
							if e.Value != "test.com" {
								t.Fatalf("NO_PROXY is not set correctly: expected %s, got %s", "test.com", e.Value)
							}
						}
					}
				}
			}
		}
	}

	err = c.Delete(context.TODO(), mco)
	if err != nil {
		t.Fatalf("Failed to delete mco: (%v)", err)
	}
	_, err = r.Reconcile(context.TODO(), req)
	if err != nil {
		t.Fatalf("reconcile: (%v)", err)
	}
	foundList := &workv1.ManifestWorkList{}
	err = c.List(context.TODO(), foundList)
	if err != nil {
		t.Fatalf("Failed to list manifestwork: (%v)", err)
	}
	if len(foundList.Items) != 0 {
		t.Fatalf("Not all manifestwork removed after remove mco resource")
	}

	mco.ObjectMeta.ResourceVersion = ""
	err = c.Create(context.TODO(), mco)
	if err != nil {
		t.Fatalf("Failed to create mco: (%v)", err)
	}

	_, err = r.Reconcile(context.TODO(), req)
	if err != nil {
		t.Fatalf("reconcile: (%v)", err)
	}
	err = c.Get(context.TODO(), types.NamespacedName{Name: namespace + workNameSuffix, Namespace: namespace}, found)
	if err != nil {
		t.Fatalf("Failed to get manifestwork for cluster1: (%v)", err)
	}

	invalidName := "invalid-work"
	invalidWork := &workv1.ManifestWork{
		ObjectMeta: metav1.ObjectMeta{
			Name:      invalidName,
			Namespace: namespace,
			Labels: map[string]string{
				ownerLabelKey: ownerLabelValue,
			},
		},
	}
	err = c.Create(context.TODO(), invalidWork)
	if err != nil {
		t.Fatalf("Failed to create manifestwork: (%v)", err)
	}

	_, err = r.Reconcile(context.TODO(), req)
	if err != nil {
		t.Fatalf("reconcile: (%v)", err)
	}
	err = c.Get(context.TODO(), types.NamespacedName{Name: invalidName, Namespace: namespace}, found)
	if err == nil {
		t.Fatalf("Invalid manifestwork not removed")
	}

	// test mch update and image replacement
	version := "2.4.0"
	imageManifestsCM := newTestImageManifestsConfigMap(config.GetMCONamespace(), version)
	err = c.Create(context.TODO(), imageManifestsCM)
	if err != nil {
		t.Fatalf("Failed to create the testing image manifest configmap: (%v)", err)
	}

	// Cannot trigger predicate logic, explicitly enable alert forwarding
	config.SetAlertingDisabled(false)
	hubInfoSecret = nil

	_, err = r.Reconcile(context.TODO(), req)
	if err != nil {
		t.Fatalf("reconcile: (%v)", err)
	}

	// test mco-disable-alerting annotation
	// 1. Verify that alertmanager-endpoint in secret hub-info-secret in the ManifestWork is not null
	t.Logf("check alertmanager endpoint is not null")
	foundManifestwork = &workv1.ManifestWork{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: namespace + workNameSuffix, Namespace: namespace}, foundManifestwork)
	if err != nil {
		t.Fatalf("Failed to get manifestwork %s: (%v)", namespace, err)
	}

	valid := false
	for _, manifest := range foundManifestwork.Spec.Workload.Manifests {
		obj, _ := util.GetObject(manifest.RawExtension)
		if obj.GetObjectKind().GroupVersionKind().Kind == "Secret" {
			s := obj.(*corev1.Secret)
			if s.GetName() == operatorconfig.HubInfoSecretName {
				hubInfo := operatorconfig.HubInfo{}
				yaml.Unmarshal(s.Data[operatorconfig.HubInfoSecretKey], &hubInfo)
				if err != nil {
					t.Fatalf("Failed to parse %s: (%v)", operatorconfig.HubInfoSecretKey, err)
				}
				if hubInfo.AlertmanagerEndpoint == "" {
					t.Fatalf("Null alert manager endpoint found in %s: ", operatorconfig.HubInfoSecretKey)
				}
				t.Logf("AlertmanagerEndpoint %s not null", hubInfo.AlertmanagerEndpoint)
				valid = true
			}
		}
	}
	if !valid {
		t.Fatalf("Secret %s not found in ManifestWork", operatorconfig.HubInfoSecretName)
	}

	// 2. Set mco-disable-alerting annotation in mco
	// Verify that alertmanager-endpoint in secret hub-info-secret in the ManifestWork is null
	t.Logf("check alertmanager endpoint is null after disabling alerts through annotation")
	mco.Annotations = map[string]string{config.AnnotationDisableMCOAlerting: "true"}
	c.Update(context.TODO(), mco)
	if err != nil {
		t.Fatalf("Failed to update mco after adding annotation %s: (%v)", config.AnnotationDisableMCOAlerting, err)
	}
	// Cannot trigger predicate logic, explicitly disabling alert forwarding
	config.SetAlertingDisabled(true)
	hubInfoSecret = nil

	_, err = r.Reconcile(context.TODO(), req)
	if err != nil {
		t.Fatalf("reconcile error after disabling alert forwarding through annotation: (%v)", err)
	}

	foundManifestwork = &workv1.ManifestWork{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: namespace + workNameSuffix, Namespace: namespace}, foundManifestwork)
	if err != nil {
		t.Fatalf("Failed to get manifestwork %s: (%v)", namespace, err)
	}

	valid = false
	for _, manifest := range foundManifestwork.Spec.Workload.Manifests {
		obj, _ := util.GetObject(manifest.RawExtension)
		if obj.GetObjectKind().GroupVersionKind().Kind == "Secret" {
			s := obj.(*corev1.Secret)
			if s.GetName() == operatorconfig.HubInfoSecretName {
				hubInfo := operatorconfig.HubInfo{}
				yaml.Unmarshal(s.Data[operatorconfig.HubInfoSecretKey], &hubInfo)
				if err != nil {
					t.Fatalf("Failed to parse %s: (%v)", operatorconfig.HubInfoSecretKey, err)
				}
				t.Logf("alert manager endpoint: %s", hubInfo.AlertmanagerEndpoint)
				if hubInfo.AlertmanagerEndpoint != "" {
					t.Fatalf("alert manager endpoint is not null after disabling alerts  %s: ", operatorconfig.HubInfoSecretKey)
				}
				valid = true
			}
		}
	}
	if !valid {
		t.Fatalf("Secret %s not found in ManifestWork", operatorconfig.HubInfoSecretName)
	}

	// 3. Remove mco-disable-alerting annotation in mco
	// Verify that alertmanager-endpoint in secret hub-info-secret in the ManifestWork is not null
	t.Logf("check alert manager endpoint is restored after alert forwarding is reenabled by removing annotation")
	delete(mco.Annotations, config.AnnotationDisableMCOAlerting)
	c.Update(context.TODO(), mco)
	if err != nil {
		t.Fatalf("Failed to update mco after removing annotation %s: (%v)", config.AnnotationDisableMCOAlerting, err)
	}
	// Cannot trigger predicate logic, explicitly enabling alert forwaring
	config.SetAlertingDisabled(false)
	hubInfoSecret = nil

	_, err = r.Reconcile(context.TODO(), req)
	if err != nil {
		t.Fatalf("reconcile after removing annotation to disable alert forwarding: (%v)", err)
	}

	foundManifestwork = &workv1.ManifestWork{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: namespace + workNameSuffix, Namespace: namespace}, foundManifestwork)
	if err != nil {
		t.Fatalf("Failed to get manifestwork %s: (%v)", namespace, err)
	}

	valid = false
	for _, manifest := range foundManifestwork.Spec.Workload.Manifests {
		obj, _ := util.GetObject(manifest.RawExtension)
		if obj.GetObjectKind().GroupVersionKind().Kind == "Secret" {
			s := obj.(*corev1.Secret)
			if s.GetName() == operatorconfig.HubInfoSecretName {
				hubInfo := operatorconfig.HubInfo{}
				yaml.Unmarshal(s.Data[operatorconfig.HubInfoSecretKey], &hubInfo)
				if err != nil {
					t.Fatalf("Failed to parse %s: (%v)", operatorconfig.HubInfoSecretKey, err)
				}
				if hubInfo.AlertmanagerEndpoint == "" {
					t.Fatalf("Null alert manager endpoint found in %s: ", operatorconfig.HubInfoSecretKey)
				}
				t.Logf("AlertmanagerEndpoint: %s", hubInfo.AlertmanagerEndpoint)
				valid = true
			}
		}
	}
	if !valid {
		t.Fatalf("Secret %s not found in ManifestWork", operatorconfig.HubInfoSecretName)
	}

	testMCHInstance := newMCHInstanceWithVersion(config.GetMCONamespace(), version)
	err = c.Create(context.TODO(), testMCHInstance)
	if err != nil {
		t.Fatalf("Failed to create the testing mch instance: (%v)", err)
	}

	req = ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name: config.MCHUpdatedRequestName,
		},
	}

	_, ok, err := config.ReadImageManifestConfigMap(c, testMCHInstance.Status.CurrentVersion)
	if err != nil || !ok {
		t.Fatalf("Failed to read image manifest configmap: (%T,%v)", ok, err)
	}

	// set the MCHCrdName for the reconciler
	r.CRDMap[config.MCHCrdName] = true
	_, err = r.Reconcile(context.TODO(), req)
	if err != nil {
		t.Fatalf("reconcile: (%v)", err)
	}

	foundManifestwork = &workv1.ManifestWork{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: namespace + workNameSuffix, Namespace: namespace}, foundManifestwork)
	if err != nil {
		t.Fatalf("Failed to get manifestwork %s: (%v)", namespace, err)
	}
	for _, w := range foundManifestwork.Spec.Workload.Manifests {
		var rawBytes []byte
		rawBytes, err := w.RawExtension.Marshal()
		if err != nil {
			t.Fatalf("Failed to marshal RawExtension: (%v)", err)
		}
		rawStr := string(rawBytes)
		// make sure the image for endpoint-metrics-operator is updated
		if strings.Contains(rawStr, "Deployment") {
			t.Logf("raw string: \n%s\n", rawStr)
			if !strings.Contains(rawStr, "test.io/endpoint-monitoring:test") {
				t.Fatalf("the image for endpoint-metrics-operator should be replaced with: test.io/endpoint-monitoring:test")
			}
		}
		// make sure the images-list configmap is updated
		if strings.Contains(rawStr, "images-list") {
			t.Logf("raw string: \n%s\n", rawStr)
			if !strings.Contains(rawStr, "test.io/metrics-collector:test") {
				t.Fatalf("the image for endpoint-metrics-operator should be replaced with: test.io/endpoint-monitoring:test")
			}
		}
	}
}

func newManagedClusterAddon() *addonv1alpha1.ManagedClusterAddOn {
	return &addonv1alpha1.ManagedClusterAddOn{
		TypeMeta: metav1.TypeMeta{
			APIVersion: addonv1alpha1.SchemeGroupVersion.String(),
			Kind:       "ManagedClusterAddOn",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "observability-controller",
			Namespace: namespace,
		},
		Spec: addonv1alpha1.ManagedClusterAddOnSpec{
			Configs: []addonv1alpha1.AddOnConfig{
				{
					ConfigGroupResource: addonv1alpha1.ConfigGroupResource{
						Group:    operatorutil.AddonGroup,
						Resource: operatorutil.AddonDeploymentConfigResource,
					},
					ConfigReferent: addonv1alpha1.ConfigReferent{
						Namespace: namespace,
						Name:      addonConfigName,
					},
				},
			},
		},
	}
}

func newClusterMgmtAddon() *addonv1alpha1.ClusterManagementAddOn {
	return &addonv1alpha1.ClusterManagementAddOn{
		TypeMeta: metav1.TypeMeta{
			APIVersion: addonv1alpha1.SchemeGroupVersion.String(),
			Kind:       "ClusterManagementAddOn",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "observability-controller",
		},
		Spec: addonv1alpha1.ClusterManagementAddOnSpec{
			SupportedConfigs: []addonv1alpha1.ConfigMeta{
				{
					ConfigGroupResource: addonv1alpha1.ConfigGroupResource{
						Group:    operatorutil.AddonGroup,
						Resource: operatorutil.AddonDeploymentConfigResource,
					},
					DefaultConfig: &addonv1alpha1.ConfigReferent{
						Namespace: namespace,
						Name:      defaultAddonConfigName,
					},
				},
			},
		},
	}
}

func newAddonDeploymentConfig(name, namespace string) *addonv1alpha1.AddOnDeploymentConfig {
	return &addonv1alpha1.AddOnDeploymentConfig{
		TypeMeta: metav1.TypeMeta{
			APIVersion: addonv1alpha1.SchemeGroupVersion.String(),
			Kind:       "AddonDeploymentConfig",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: addonv1alpha1.AddOnDeploymentConfigSpec{
			NodePlacement: &addonv1alpha1.NodePlacement{
				NodeSelector: map[string]string{
					"kubernetes.io/os": "linux",
				},
			},
			ProxyConfig: addonv1alpha1.ProxyConfig{
				HTTPProxy:  "http://foo.com",
				HTTPSProxy: "https://foo.com",
				NoProxy:    "bar.com",
			},
		},
	}
}
