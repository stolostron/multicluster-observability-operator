// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package multiclusterobservability

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	placementv1alpha1 "github.com/open-cluster-management/api/cluster/v1alpha1"
	mcov1beta2 "github.com/open-cluster-management/multicluster-observability-operator/api/v1beta2"
	mcoconfig "github.com/open-cluster-management/multicluster-observability-operator/pkg/config"
)

func TestcreatePlacement(t *testing.T) {
	var (
		name       = "monitoring"
		namespace  = mcoconfig.GetDefaultNamespace()
		pName      = mcoconfig.GetPlacementRuleName()
		testSuffix = "-test"
	)
	mco := &mcov1beta2.MultiClusterObservability{
		TypeMeta:   metav1.TypeMeta{Kind: "MultiClusterObservability"},
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec:       mcov1beta2.MultiClusterObservabilitySpec{},
	}

	s := scheme.Scheme
	mcov1beta2.SchemeBuilder.AddToScheme(s)
	placementv1alpha1.AddToScheme(s)

	c := fake.NewFakeClient()

	err := createPlacement(c, s, mco)
	if err != nil {
		t.Fatalf("createPlacement: (%v)", err)
	}

	// Test scenario in which placement updated by others
	p := &placementv1alpha1.Placement{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pName,
			Namespace: namespace,
		},
		Spec: placementv1alpha1.PlacementSpec{
			Predicates: []placementv1alpha1.ClusterPredicate{
				{
					RequiredClusterSelector: placementv1alpha1.ClusterSelector{
						LabelSelector: metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "observability" + testSuffix,
									Operator: metav1.LabelSelectorOpNotIn,
									Values:   []string{"disabled"},
								},
							},
						},
					},
				},
			},
		},
	}
	c = fake.NewFakeClient(p)
	err = createPlacement(c, s, mco)
	if err != nil {
		t.Fatalf("createPlacement: (%v)", err)
	}

	found := &placementv1alpha1.Placement{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: pName, Namespace: namespace}, found)
	if err != nil {
		t.Fatalf("Failed to get placement (%s): (%v)", pName, err)
	}
	if found.Spec.Predicates[0].RequiredClusterSelector.LabelSelector.MatchExpressions[0].Key != "observability" {
		t.Fatalf("Failed to revert placement (%s)", pName)
	}

}
