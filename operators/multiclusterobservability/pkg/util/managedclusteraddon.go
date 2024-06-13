// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package util

import (
	"context"
	"os"
	"time"

	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	ManagedClusterAddonName = "observability-controller" // #nosec G101 -- Not a hardcoded credential.
)

var (
	log            = logf.Log.WithName("util")
	spokeNameSpace = os.Getenv("SPOKE_NAMESPACE")
)

func CreateManagedClusterAddonCR(c client.Client, namespace, labelKey, labelValue string) (
	*addonv1alpha1.ManagedClusterAddOn, error) {
	// local-cluster does not have a managedClusterAddon
	if namespace == config.GetDefaultNamespace() {
		return nil, nil
	}
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
			log.Error(
				err,
				"failed to create managedclusteraddon",
				"name",
				ManagedClusterAddonName,
				"namespace",
				namespace,
			)
			return nil, err
		}

		// wait 10s for the created managedclusteraddon ready
		if errPoll := wait.PollUntilContextTimeout(context.Background(), 2*time.Second, 10*time.Second, true, func(ctx context.Context) (bool, error) {
			if err := c.Get(
				ctx,
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
			log.Error(
				errPoll,
				"failed to get the created managedclusteraddon",
				"name",
				ManagedClusterAddonName,
				"namespace",
				namespace,
			)
			return nil, errPoll
		}

		// got the created managedclusteraddon just now, uopdating its status
		// TODO(saswatamcode): Remove deprecated field
		//nolint:staticcheck
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
			log.Error(
				err,
				"failed to update status for managedclusteraddon",
				"name",
				ManagedClusterAddonName,
				"namespace",
				namespace,
			)
			return nil, err
		}
		return managedClusterAddon, nil
	} else if err != nil {
		log.Error(err, "failed to get managedclusteraddon", "name", ManagedClusterAddonName, "namespace", namespace)
		return nil, err
	}

	log.Info(
		"ManagedClusterAddOn is created or updated successfully",
		"name",
		ManagedClusterAddonName,
		"namespace",
		namespace,
	)
	return managedClusterAddon, nil
}
