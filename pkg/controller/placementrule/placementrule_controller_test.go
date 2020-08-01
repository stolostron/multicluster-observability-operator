// Copyright (c) 2020 Red Hat, Inc.

package placementrule

import (
	"context"
	"testing"

	ocinfrav1 "github.com/openshift/api/config/v1"
	routev1 "github.com/openshift/api/route/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	workv1 "github.com/open-cluster-management/api/work/v1"
	placementv1 "github.com/open-cluster-management/multicloud-operators-placementrule/pkg/apis/apps/v1"
	"github.com/open-cluster-management/multicluster-observability-operator/pkg/apis"
	"github.com/open-cluster-management/multicluster-observability-operator/pkg/config"
)

const (
	namespace    = "test-ns"
	namespace2   = "test-ns-2"
	mcmName      = "test-mcm"
	mcmNameSpace = "test-mcm-namespace"
)

func initSchema(t *testing.T) {
	s := scheme.Scheme
	if err := placementv1.AddToScheme(s); err != nil {
		t.Fatalf("Unable to add placementrule scheme: (%v)", err)
	}
	if err := apis.AddToScheme(s); err != nil {
		t.Fatalf("Unable to add monitoringv1alpha1 scheme: (%v)", err)
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
}

func TestEndpointMonitoringController(t *testing.T) {
	s := scheme.Scheme
	initSchema(t)
	config.SetMonitoringCRName(mcmName)

	p := &placementv1.PlacementRule{
		ObjectMeta: v1.ObjectMeta{
			Name:      placementRuleName,
			Namespace: mcmNameSpace,
		},
		Status: placementv1.PlacementRuleStatus{
			Decisions: []placementv1.PlacementDecision{
				{
					ClusterName:      "cluster1",
					ClusterNamespace: namespace,
				},
				{
					ClusterName:      "cluster2",
					ClusterNamespace: namespace2,
				},
			},
		},
	}
	objs := []runtime.Object{p, newTestMCM(), newTestPullSecret(), newTestRoute(), newTestInfra(), newSATokenSecret(), newTestSA(), newSATokenSecret(namespace2), newTestSA(namespace2)}
	c := fake.NewFakeClient(objs...)

	r := &ReconcilePlacementRule{client: c, scheme: s}
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      placementRuleName,
			Namespace: mcmNameSpace,
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
			Namespace: mcmNameSpace,
		},
		Status: placementv1.PlacementRuleStatus{
			Decisions: []placementv1.PlacementDecision{
				{
					ClusterName:      "cluster1",
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
}
