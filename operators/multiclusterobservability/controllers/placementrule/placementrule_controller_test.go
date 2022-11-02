// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package placementrule

import (
	"context"
	"os"
	"path"
	"strings"
	"testing"

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
	operatorconfig "github.com/stolostron/multicluster-observability-operator/operators/pkg/config"
	"github.com/stolostron/multicluster-observability-operator/operators/pkg/util"
	mchv1 "github.com/stolostron/multiclusterhub-operator/api/v1"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"
)

const (
	namespace    = "test-ns"
	namespace2   = "test-ns-2"
	clusterName  = "cluster1"
	clusterName2 = "cluster2"
	mcoName      = "test-mco"
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

func TestObservabilityPlacementController(t *testing.T) {
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

	// Uncomment these lines to log controller
	// opts := zap.Options{
	// 	Development: true,
	// }
	// ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	objs := []runtime.Object{mco, pull, newConsoleRoute(), newTestObsApiRoute(), newTestAlertmanagerRoute(), newTestIngressController(), newTestRouteCASecret(), newCASecret(), newCertSecret(mcoNamespace), NewMetricsAllowListCM(),
		NewAmAccessorSA(), NewAmAccessorTokenSecret(), newManagedClusterAddon(), deprecatedRole}
	c := fake.NewClientBuilder().WithRuntimeObjects(objs...).Build()
	r := &PlacementRuleReconciler{Client: c, Scheme: s, CRDMap: map[string]bool{config.IngressControllerCRD: true}}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get work dir: (%v)", err)
	}
	os.MkdirAll(path.Join(wd, "../../placementrule-tests"), 0755)
	testManifestsPath := path.Join(wd, "../../placementrule-tests/manifests")
	manifestsPath := path.Join(wd, "../../manifests")
	os.Setenv("TEMPLATES_PATH", testManifestsPath)
	templates.ResetTemplates()
	if _, err := os.Stat(testManifestsPath); err == os.ErrNotExist {
		err = os.Symlink(manifestsPath, testManifestsPath)
		if err != nil {
			t.Fatalf("Failed to create symbollink(%s) to(%s) for the test manifests: (%v)", testManifestsPath, manifestsPath, err)
		}
	}

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      config.GetDefaultCRName(),
			Namespace: mcoNamespace,
		},
	}

	managedClusterList = map[string]string{
		namespace:  "4",
		namespace2: "4",
	}
	_, err = r.Reconcile(context.TODO(), req)
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

	managedClusterList = map[string]string{namespace: "4"}
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

	// test mco-disable-alerting annotation
	// 1. Verify that alertmanager-endpoint in secret hub-info-secret in the ManifestWork is not null
	t.Logf("check alertmanager endpoint is not null")
	foundManifestwork := &workv1.ManifestWork{}
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
	// force hubInfoSecret to be regenerated
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
				t.Logf("AlertmanagerEndpoint is null")
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
	// force hubInfoSecret to be regenerated
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

	// test mch update and image replacement
	version := "2.4.0"
	imageManifestsCM := newTestImageManifestsConfigMap(config.GetMCONamespace(), version)
	err = c.Create(context.TODO(), imageManifestsCM)
	if err != nil {
		t.Fatalf("Failed to create the testing image manifest configmap: (%v)", err)
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

	ok, err := config.ReadImageManifestConfigMap(c, testMCHInstance.Status.CurrentVersion)
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

	// remove the testing manifests directory
	_, err = os.Stat(testManifestsPath)
	if err == nil {
		if err = os.Remove(testManifestsPath); err != nil {
			t.Fatalf("Failed to delete symbollink(%s) for the test manifests: (%v)", testManifestsPath, err)
		}
	}
	os.Remove(path.Join(wd, "../../placementrule-tests"))
}

func newManagedClusterAddon() *addonv1alpha1.ManagedClusterAddOn {
	return &addonv1alpha1.ManagedClusterAddOn{
		TypeMeta: metav1.TypeMeta{
			APIVersion: addonv1alpha1.SchemeGroupVersion.String(),
			Kind:       "ManagedClusterAddOn",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "managedClusterAddonName",
			Namespace: namespace,
		},
	}
}
