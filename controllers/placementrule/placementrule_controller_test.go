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
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	utilpointer "k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	addonv1alpha1 "github.com/open-cluster-management/api/addon/v1alpha1"
	workv1 "github.com/open-cluster-management/api/work/v1"
	placementv1 "github.com/stolostron/multicloud-operators-placementrule/pkg/apis/apps/v1"
	mcov1beta1 "github.com/stolostron/multicluster-observability-operator/api/v1beta1"
	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/api/v1beta2"
	"github.com/stolostron/multicluster-observability-operator/pkg/config"
	mchv1 "github.com/stolostron/multiclusterhub-operator/pkg/apis/operator/v1"
)

const (
	namespace    = "test-ns"
	namespace2   = "test-ns-2"
	clusterName  = "cluster1"
	clusterName2 = "cluster2"
	clusterName3 = "cluster3"
	mcoName      = "test-mco"
)

var (
	mcoNamespace = config.GetDefaultNamespace()
)

func initSchema(t *testing.T) {
	s := scheme.Scheme
	if err := placementv1.AddToScheme(s); err != nil {
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

func createPlacementRuleCRD() *apiextensionsv1beta1.CustomResourceDefinition {
	return &apiextensionsv1beta1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "placementrules.apps.open-cluster-management.io"},
		Spec: apiextensionsv1beta1.CustomResourceDefinitionSpec{
			Scope:                 apiextensionsv1beta1.NamespaceScoped,
			Conversion:            &apiextensionsv1beta1.CustomResourceConversion{Strategy: apiextensionsv1beta1.NoneConverter},
			PreserveUnknownFields: utilpointer.BoolPtr(false),
			Group:                 "apps.open-cluster-management.io",
			Names: apiextensionsv1beta1.CustomResourceDefinitionNames{
				Kind:     "PlacementRule",
				ListKind: "PlacementRuleList",
				Plural:   "placementrules",
				Singular: "placementrule",
			},
			Version: "v1",
			Versions: []apiextensionsv1beta1.CustomResourceDefinitionVersion{
				{Name: "v1", Storage: true, Served: true},
			},
		},
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

func TestObservabilityAddonController(t *testing.T) {
	s := scheme.Scheme
	addonv1alpha1.AddToScheme(s)
	initSchema(t)
	config.SetMonitoringCRName(mcoName)

	placementRuleName := config.GetDefaultCRName()
	p := &placementv1.PlacementRule{
		ObjectMeta: v1.ObjectMeta{
			Name:      placementRuleName,
			Namespace: mcoNamespace,
		},
		Status: placementv1.PlacementRuleStatus{
			Decisions: []placementv1.PlacementDecision{
				{
					ClusterName:      clusterName,
					ClusterNamespace: namespace,
				},
				{
					ClusterName:      clusterName2,
					ClusterNamespace: namespace2,
				},
			},
		},
	}
	p2 := &placementv1.PlacementRule{
		ObjectMeta: v1.ObjectMeta{
			Name:      config.Placement311CRName,
			Namespace: mcoNamespace,
		},
		Status: placementv1.PlacementRuleStatus{
			Decisions: []placementv1.PlacementDecision{
				{
					ClusterName:      clusterName3,
					ClusterNamespace: namespace,
				},
			},
		},
	}
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
	objs := []runtime.Object{p, p2, mco, pull, newTestObsApiRoute(), newTestAlertmanagerRoute(), newTestIngressController(), newTestRouteCASecret(), newCASecret(), newCertSecret(mcoNamespace), NewMetricsAllowListCM(),
		NewAmAccessorSA(), NewAmAccessorTokenSecret(), newManagedClusterAddon(), deprecatedRole}
	c := fake.NewFakeClient(objs...)
	r := &PlacementRuleReconciler{Client: c, Scheme: s, CRDMap: map[string]bool{config.PlacementRuleCrdName: true}}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get work dir: (%v)", err)
	}
	templatePath = path.Join(wd, "../../tests/manifests/endpoint-observability")
	os.MkdirAll(path.Dir(templatePath), 0755)
	endpointObsManifestsPath := path.Join(wd, "../../manifests/endpoint-observability")
	err = os.Symlink(endpointObsManifestsPath, templatePath)
	if err != nil {
		t.Fatalf("Failed to create symbollink(%s) to(%s) for the test manifests: (%v)", templatePath, endpointObsManifestsPath, err)
	}

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      placementRuleName,
			Namespace: mcoNamespace,
		},
	}
	_, err = r.Reconcile(context.TODO(), req)
	if err != nil {
		t.Fatalf("reconcile: (%v)", err)
	}
	found := &workv1.ManifestWork{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: namespace + workNameSuffix, Namespace: namespace}, found)
	if err != nil {
		t.Fatalf("Failed to get manifestwork for cluster1: (%v)", err)
	}
	err = c.Get(context.TODO(), types.NamespacedName{Name: namespace2 + workNameSuffix, Namespace: namespace2}, found)
	if err != nil {
		t.Fatalf("Failed to get manifestwork for cluster2: (%v)", err)
	}
	foundRole := &rbacv1.Role{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: "endpoint-observability-role", Namespace: namespace}, foundRole)
	if err == nil || !errors.IsNotFound(err) {
		t.Fatalf("Deprecated role not removed")
	}

	newPlacement := &placementv1.PlacementRule{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: placementRuleName, Namespace: mcoNamespace}, newPlacement)
	if err != nil {
		t.Fatalf("Failed to get placementrule: (%v)", err)
	}
	newPlacement.Status = placementv1.PlacementRuleStatus{
		Decisions: []placementv1.PlacementDecision{
			{
				ClusterName:      clusterName,
				ClusterNamespace: namespace,
			},
		},
	}

	err = c.Update(context.TODO(), newPlacement)
	if err != nil {
		t.Fatalf("Failed to update placementrule: (%v)", err)
	}
	_, err = r.Reconcile(context.TODO(), req)
	if err != nil {
		t.Fatalf("reconcile: (%v)", err)
	}
	err = c.Get(context.TODO(), types.NamespacedName{Name: namespace + workNameSuffix, Namespace: namespace}, found)
	if err != nil {
		t.Fatalf("Failed to get manifestwork for cluster1: (%v)", err)
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
	// test mch update and image replacement
	version := "2.3.0"
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

	foundManifestwork := &workv1.ManifestWork{}
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
			if !strings.Contains(rawStr, "test.io/metrics-collector:test") {
				t.Fatalf("the image for metrics-collector should be replaced with: test.io/metrics-collector:test")
			}
		}
	}

	// remove the testing manifests directory
	if err = os.Remove(templatePath); err != nil {
		t.Fatalf("Failed to delete symbollink(%s) for the test manifests: (%v)", templatePath, err)
	}
	os.Remove(path.Dir(templatePath))
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
