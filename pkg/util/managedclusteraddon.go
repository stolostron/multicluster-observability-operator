// Copyright (c) 2020 Red Hat, Inc.

package util

import (
	"context"

	addonv1alpha1 "github.com/open-cluster-management/addon-framework/api/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	managedClusterAddonName = "observability-controller"
)

func CreateManagedClusterAddonCR(client client.Client, namespace string) error {
	managedClusterAddon := &addonv1alpha1.ManagedClusterAddOn{}
	// check if managedClusterAddon exists
	if err := client.Get(
		context.TODO(),
		types.NamespacedName{
			Name:      managedClusterAddonName,
			Namespace: namespace,
		},
		managedClusterAddon,
	); err != nil && errors.IsNotFound(err) {
		// create new managedClusterAddon
		newManagedClusterAddon := &addonv1alpha1.ManagedClusterAddOn{
			TypeMeta: metav1.TypeMeta{
				APIVersion: addonv1alpha1.SchemeGroupVersion.String(),
				Kind:       "ManagedClusterAddOn",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      managedClusterAddonName,
				Namespace: namespace,
			},
		}

		if err := client.Create(context.TODO(), newManagedClusterAddon); err != nil {
			log.Error(err, "Cannot create observability-controller  ManagedClusterAddOn")
			return err
		}
	} else if err != nil {
		log.Error(err, "Failed to get ManagedClusterAddOn ", "namespace", namespace)
		return err
	}
	log.Info("ManagedClusterAddOn already present", "namespace", namespace)
	return nil
}
