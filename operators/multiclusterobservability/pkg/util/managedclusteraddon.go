// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package util

import (
	"context"
	"os"
	"reflect"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ManagedClusterAddonName = "observability-controller"
)

var (
	spokeNameSpace = os.Getenv("SPOKE_NAMESPACE")
)

func CreateManagedClusterAddonCR(c client.Client, namespace, labelKey, labelValue string) error {
	newManagedClusterAddon := &addonv1alpha1.ManagedClusterAddOn{
		TypeMeta: metav1.TypeMeta{
			APIVersion: addonv1alpha1.SchemeGroupVersion.String(),
			Kind:       "ManagedClusterAddOn",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      ManagedClusterAddonName,
			Namespace: namespace,
			Labels: map[string]string{
				labelKey: labelValue,
			},
		},
		Spec: addonv1alpha1.ManagedClusterAddOnSpec{
			InstallNamespace: spokeNameSpace,
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
			log.Error(err, "failed to create managedclusteraddon", "name", ManagedClusterAddonName, "namespace", namespace)
			return err
		}

		// wait 10s for the created managedclusteraddon ready
		if errPoll := wait.Poll(2*time.Second, 10*time.Second, func() (bool, error) {
			if err := c.Get(
				context.TODO(),
				types.NamespacedName{
					Name:      ManagedClusterAddonName,
					Namespace: namespace,
				},
				managedClusterAddon,
			); err == nil {
				return true, nil
			}
			return false, err
		}); errPoll != nil {
			log.Error(errPoll, "failed to get the created managedclusteraddon", "name", ManagedClusterAddonName, "namespace", namespace)
			return errPoll
		}

		// got the created managedclusteraddon just now, uopdating its status
		managedClusterAddon.Status.AddOnConfiguration = addonv1alpha1.ConfigCoordinates{
			CRDName: "observabilityaddons.observability.open-cluster-management.io",
			CRName:  "observability-addon",
		}
		managedClusterAddon.Status.AddOnMeta = addonv1alpha1.AddOnMeta{
			DisplayName: "Observability Controller",
			Description: "Manages Observability components.",
		}
		if len(managedClusterAddon.Status.Conditions) > 0 {
			managedClusterAddon.Status.Conditions = append(managedClusterAddon.Status.Conditions, metav1.Condition{
				Type:               "Progressing",
				Status:             metav1.ConditionTrue,
				LastTransitionTime: metav1.NewTime(time.Now()),
				Reason:             "ManifestWorkCreated",
				Message:            "Addon Installing",
			})
		} else {
			managedClusterAddon.Status.Conditions = []metav1.Condition{
				{
					Type:               "Progressing",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: metav1.NewTime(time.Now()),
					Reason:             "ManifestWorkCreated",
					Message:            "Addon Installing",
				},
			}
		}
		// update status for the created managedclusteraddon
		if err := c.Status().Update(context.TODO(), managedClusterAddon); err != nil {
			log.Error(err, "failed to update status for managedclusteraddon", "name", ManagedClusterAddonName, "namespace", namespace)
			return err
		}
		return nil
	} else if err != nil {
		log.Error(err, "failed to get managedclusteraddon", "name", ManagedClusterAddonName, "namespace", namespace)
		return err
	}

	// managedclusteraddon already exists, updating...
	if !reflect.DeepEqual(managedClusterAddon.Spec, newManagedClusterAddon.Spec) {
		log.Info("found difference, updating managedClusterAddon", "name", ManagedClusterAddonName, "namespace", namespace)
		newManagedClusterAddon.ObjectMeta.ResourceVersion = managedClusterAddon.ObjectMeta.ResourceVersion
		err := c.Update(context.TODO(), newManagedClusterAddon)
		if err != nil {
			log.Error(err, "failed to update managedclusteraddon", "name", ManagedClusterAddonName, "namespace", namespace)
			return err
		}
		return nil
	}

	log.Info("ManagedClusterAddOn is created or updated successfully", "name", ManagedClusterAddonName, "namespace", namespace)
	return nil
}
