// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package util

import (
	"context"
	"fmt"
	"reflect"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
)

const (
	ObservabilityController = "observability-controller"
	grafanaLink             = "/grafana/d/2b679d600f3b9e7676a7c5ac3643d448/acm-clusters-overview"
)

type clusterManagementAddOnSpec struct {
	DisplayName string `json:"displayName"`
	Description string `json:"description"`
	CRDName     string `json:"crdName"`
}

func CreateClusterManagementAddon(c client.Client) error {
	clusterManagementAddon := newClusterManagementAddon()
	found := &addonv1alpha1.ClusterManagementAddOn{}
	err := c.Get(context.TODO(), types.NamespacedName{Name: ObservabilityController}, found)
	if err != nil && errors.IsNotFound(err) {
		if err := c.Create(context.TODO(), clusterManagementAddon); err != nil {
			log.Error(err, "Failed to create observability-controller clustermanagementaddon ")
			return err
		}
		log.Info("Created observability-controller clustermanagementaddon")
		return nil
	} else if err != nil {
		log.Error(err, "Cannot create observability-controller clustermanagementaddon")
		return err
	}

	if !reflect.DeepEqual(found.Spec, clusterManagementAddon.Spec) ||
		!reflect.DeepEqual(found.ObjectMeta.Annotations, clusterManagementAddon.ObjectMeta.Annotations) {
		log.Info("Updating observability-controller clustermanagementaddon")
		clusterManagementAddon.ObjectMeta.ResourceVersion = found.ObjectMeta.ResourceVersion
		err = c.Update(context.TODO(), clusterManagementAddon)
		if err != nil {
			log.Error(err, "Failed to update observability-controller clustermanagementaddon")
			return err
		}
		return nil
	}

	log.Info(fmt.Sprintf("%s clustermanagementaddon is present ", ObservabilityController))
	return nil
}

func DeleteClusterManagementAddon(client client.Client) error {
	clustermanagementaddon := &addonv1alpha1.ClusterManagementAddOn{
		ObjectMeta: metav1.ObjectMeta{
			Name: ObservabilityController,
		},
	}
	err := client.Delete(context.TODO(), clustermanagementaddon)
	if err != nil && !errors.IsNotFound(err) {
		log.Error(err, "Failed to delete clustermanagementaddon", "name", ObservabilityController)
		return err
	}
	log.Info("ClusterManagementAddon deleted", "name", ObservabilityController)
	return nil
}

func newClusterManagementAddon() *addonv1alpha1.ClusterManagementAddOn {
	clusterManagementAddOnSpec := clusterManagementAddOnSpec{
		DisplayName: "Observability Controller",
		Description: "Manages Observability components.",
		CRDName:     "observabilityaddons.observability.open-cluster-management.io",
	}
	return &addonv1alpha1.ClusterManagementAddOn{
		TypeMeta: metav1.TypeMeta{
			APIVersion: addonv1alpha1.SchemeGroupVersion.String(),
			Kind:       "ClusterManagementAddOn",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: ObservabilityController,
			Annotations: map[string]string{
				"console.open-cluster-management.io/launch-link":      grafanaLink,
				"console.open-cluster-management.io/launch-link-text": "Grafana",
			},
		},
		Spec: addonv1alpha1.ClusterManagementAddOnSpec{
			AddOnMeta: addonv1alpha1.AddOnMeta{
				DisplayName: clusterManagementAddOnSpec.DisplayName,
				Description: clusterManagementAddOnSpec.Description,
			},
			AddOnConfiguration: addonv1alpha1.ConfigCoordinates{
				CRDName: clusterManagementAddOnSpec.CRDName,
			},
		},
	}
}
