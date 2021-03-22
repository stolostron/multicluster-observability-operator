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

	appsv1 "github.com/open-cluster-management/multicloud-operators-placementrule/pkg/apis/apps/v1"
	mcov1beta2 "github.com/open-cluster-management/multicluster-observability-operator/api/v1beta2"
	mcoconfig "github.com/open-cluster-management/multicluster-observability-operator/pkg/config"
)

func TestCreatePlacementRule(t *testing.T) {
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
	appsv1.SchemeBuilder.AddToScheme(s)

	c := fake.NewFakeClient()

	err := createPlacementRule(c, s, mco)
	if err != nil {
		t.Fatalf("createPlacementRule: (%v)", err)
	}

	// Test scenario in which placementrule updated by others
	p := &appsv1.PlacementRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pName,
			Namespace: namespace,
		},
		Spec: appsv1.PlacementRuleSpec{
			GenericPlacementFields: appsv1.GenericPlacementFields{
				ClusterSelector: &metav1.LabelSelector{
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
	}
	c = fake.NewFakeClient(p)
	err = createPlacementRule(c, s, mco)
	if err != nil {
		t.Fatalf("createPlacementRule: (%v)", err)
	}

	found := &appsv1.PlacementRule{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: pName, Namespace: namespace}, found)
	if err != nil {
		t.Fatalf("Failed to get placementrule (%s): (%v)", pName, err)
	}
	if found.Spec.GenericPlacementFields.ClusterSelector.MatchExpressions[0].Key != "observability" {
		t.Fatalf("Failed to revert placementrule (%s)", pName)
	}

}
