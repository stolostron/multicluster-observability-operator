// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package util

import (
	"context"
	"os"
	"reflect"
	"time"

	addonv1alpha1 "github.com/open-cluster-management/api/addon/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ManagedClusterAddonName = "observability-controller"
)

var (
	spokeNameSpace = os.Getenv("SPOKE_NAMESPACE")
)

func CreateManagedClusterAddonCR(c client.Client, namespace string) error {
	newManagedClusterAddon := &addonv1alpha1.ManagedClusterAddOn{
		TypeMeta: metav1.TypeMeta{
			APIVersion: addonv1alpha1.SchemeGroupVersion.String(),
			Kind:       "ManagedClusterAddOn",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      ManagedClusterAddonName,
			Namespace: namespace,
		},
		Spec: addonv1alpha1.ManagedClusterAddOnSpec{
			InstallNamespace: spokeNameSpace,
		},
		Status: addonv1alpha1.ManagedClusterAddOnStatus{
			AddOnConfiguration: addonv1alpha1.ConfigCoordinates{
				CRDName: "observabilityaddons.observability.open-cluster-management.io",
				CRName:  "observability-addon",
			},
			AddOnMeta: addonv1alpha1.AddOnMeta{
				DisplayName: "Observability Controller",
				Description: "Manages Observability components.",
			},
			Conditions: []metav1.Condition{
				{
					Type:               "Progressing",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: metav1.NewTime(time.Now()),
					Reason:             "ManifestWorkCreated",
					Message:            "Addon Installing",
				},
			},
		},
	}
	managedClusterAddon := &addonv1alpha1.ManagedClusterAddOn{}
	// check if managedClusterAddon exists
	if err := c.Get(
		context.TODO(),
		types.NamespacedName{
			Name:      ManagedClusterAddonName,
			Namespace: namespace,
		},
		managedClusterAddon,
	); err != nil && errors.IsNotFound(err) {
		// create new managedClusterAddon
		if err := c.Create(context.TODO(), newManagedClusterAddon); err != nil {
			log.Error(err, "Cannot create observability-controller  ManagedClusterAddOn")
			return err
		}
		if err := c.Status().Update(context.TODO(), newManagedClusterAddon); err != nil {
			log.Error(err, "Cannot update status for observability-controller  ManagedClusterAddOn")
			return err
		}
	} else if err != nil {
		log.Error(err, "Failed to get ManagedClusterAddOn ", "namespace", namespace)
		return err
	}

	if !reflect.DeepEqual(managedClusterAddon.Spec, newManagedClusterAddon.Spec) {
		log.Info("Updating observability-controller managedClusterAddon")
		newManagedClusterAddon.ObjectMeta.ResourceVersion = managedClusterAddon.ObjectMeta.ResourceVersion
		err := c.Update(context.TODO(), newManagedClusterAddon)
		if err != nil {
			log.Error(err, "Failed to update observability-controller managedClusterAddon")
			return err
		}
		return nil
	}

	log.Info("ManagedClusterAddOn already present", "namespace", namespace)

	return nil
}
