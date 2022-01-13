// Copyright (c) 2020 Red Hat, Inc.

package placementrule

import (
	"context"
	"testing"

	cert "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1alpha1"
	ocinfrav1 "github.com/openshift/api/config/v1"
	routev1 "github.com/openshift/api/route/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"

	addonv1alpha1 "github.com/open-cluster-management/api/addon/v1alpha1"
	workv1 "github.com/open-cluster-management/api/work/v1"
	placementv1 "github.com/stolostron/multicloud-operators-placementrule/pkg/apis/apps/v1"
	"github.com/stolostron/multicluster-monitoring-operator/pkg/apis"
	"github.com/stolostron/multicluster-monitoring-operator/pkg/config"
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
	if err := placementv1.AddToScheme(s); err != nil {
		t.Fatalf("Unable to add placementrule scheme: (%v)", err)
	}
	if err := apis.AddToScheme(s); err != nil {
		t.Fatalf("Unable to add mcov1beta1 scheme: (%v)", err)
	}
	if err := routev1.AddToScheme(s); err != nil {
		t.Fatalf("Unable to add routev1 scheme: (%v)", err)
	}
	if err := ocinfrav1.AddToScheme(s); err != nil {
		t.Fatalf("Unable to add ocinfrav1 scheme: (%v)", err)
	}
	if err := workv1.AddToScheme(s); err != nil {
		t.Fatalf("Unable to add workv1 scheme: (%v)", err)
	}
	if err := cert.AddToScheme(s); err != nil {
		t.Fatalf("Unable to add cert scheme: (%v)", err)
	}
}

func TestObservabilityAddonController(t *testing.T) {

	logf.SetLogger(logf.ZapLogger(true))

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
	mco := newTestMCO()
	pull := newTestPullSecret()
	objs := []runtime.Object{p, mco, pull, newTestRoute(), newTestInfra(), newCASecret(), newCertSecret(), NewMetricsWhiteListCM(),
		newSATokenSecret(), newTestSA(), newSATokenSecret(namespace2), newTestSA(namespace2), newCertSecret(namespace2), newManagedClusterAddon()}
	c := fake.NewFakeClient(objs...)

	r := &ReconcilePlacementRule{client: c, scheme: s}
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      placementRuleName,
			Namespace: mcoNamespace,
		},
	}
	_, err := r.Reconcile(req)
	if err != nil {
		t.Fatalf("reconcile: (%v)", err)
	}
	found := &workv1.ManifestWork{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: workName, Namespace: namespace}, found)
	if err != nil {
		t.Fatalf("Failed to get manifestwork for cluster1: (%v)", err)
	}
	err = c.Get(context.TODO(), types.NamespacedName{Name: workName, Namespace: namespace2}, found)
	if err != nil {
		t.Fatalf("Failed to get manifestwork for cluster2: (%v)", err)
	}

	p = &placementv1.PlacementRule{
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
			},
		},
	}
	err = c.Update(context.TODO(), p)
	if err != nil {
		t.Fatalf("Failed to update placementrule: (%v)", err)
	}
	_, err = r.Reconcile(req)
	if err != nil {
		t.Fatalf("reconcile: (%v)", err)
	}
	err = c.Get(context.TODO(), types.NamespacedName{Name: workName, Namespace: namespace}, found)
	if err != nil {
		t.Fatalf("Failed to get manifestwork for cluster1: (%v)", err)
	}
	err = c.Get(context.TODO(), types.NamespacedName{Name: workName, Namespace: namespace2}, found)
	if err == nil || !errors.IsNotFound(err) {
		t.Fatalf("Failed to delete manifestwork for cluster2: (%v)", err)
	}

	err = c.Delete(context.TODO(), pull)
	if err != nil {
		t.Fatalf("Failed to delete pull secret: (%v)", err)
	}
	_, err = r.Reconcile(req)
	if err != nil {
		t.Fatalf("reconcile: (%v)", err)
	}

	err = c.Delete(context.TODO(), mco)
	if err != nil {
		t.Fatalf("Failed to delete mco: (%v)", err)
	}
	_, err = r.Reconcile(req)
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

	err = c.Create(context.TODO(), mco)
	if err != nil {
		t.Fatalf("Failed to create mco: (%v)", err)
	}
	err = c.Create(context.TODO(), newTestSA())
	if err != nil {
		t.Fatalf("Failed to create sa: (%v)", err)
	}

	_, err = r.Reconcile(req)
	if err != nil {
		t.Fatalf("reconcile: (%v)", err)
	}
	err = c.Get(context.TODO(), types.NamespacedName{Name: workName, Namespace: namespace}, found)
	if err != nil {
		t.Fatalf("Failed to get manifestwork for cluster1: (%v)", err)
	}
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
