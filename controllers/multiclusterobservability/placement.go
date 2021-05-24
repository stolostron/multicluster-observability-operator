// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package multiclusterobservability

import (
	"context"
	"reflect"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	placementv1alpha1 "github.com/open-cluster-management/api/cluster/v1alpha1"
	mcov1beta2 "github.com/open-cluster-management/multicluster-observability-operator/api/v1beta2"
	"github.com/open-cluster-management/multicluster-observability-operator/pkg/config"
)

func createPlacement(client client.Client, scheme *runtime.Scheme,
	mco *mcov1beta2.MultiClusterObservability) error {
	name := config.GetPlacementName()
	namespace := config.GetDefaultNamespace()
	p := &placementv1alpha1.Placement{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: placementv1alpha1.PlacementSpec{
			Predicates: []placementv1alpha1.ClusterPredicate{
				{
					RequiredClusterSelector: placementv1alpha1.ClusterSelector{
						LabelSelector: metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "observability",
									Operator: metav1.LabelSelectorOpNotIn,
									Values:   []string{"disabled"},
								},
								{
									Key:      "vendor",
									Operator: metav1.LabelSelectorOpIn,
									Values:   []string{"OpenShift"},
								},
							},
						},
					},
				},
			},
		},
	}

	// Set MultiClusterObservability instance as the owner and controller
	if namespace == config.GetDefaultNamespace() {
		if err := controllerutil.SetControllerReference(mco, p, scheme); err != nil {
			return err
		}
	}

	found := &placementv1alpha1.Placement{}
	err := client.Get(context.TODO(), types.NamespacedName{Name: name, Namespace: namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating Placement", "name", name)
		err = client.Create(context.TODO(), p)
		if err != nil {
			log.Error(err, "Failed to create Placement", "name", name)
			return err
		}
		return nil
	} else if err != nil {
		log.Error(err, "Failed to check Placement", "name", name)
		return err
	}

	if !reflect.DeepEqual(found.Spec, p.Spec) {
		log.Info("Reverting Placement", "name", name)
		p.ObjectMeta.ResourceVersion = found.ObjectMeta.ResourceVersion
		err = client.Update(context.TODO(), p)
		if err != nil {
			log.Error(err, "Failed to revert Placement", "name", name)
			return err
		}
		return nil
	}

	log.Info("Placement already existed", "name", name)
	return nil
}
