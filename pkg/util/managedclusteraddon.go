// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package util

import (
	"context"
	"os"
	"time"

	addonv1alpha1 "github.com/open-cluster-management/api/addon/v1alpha1"
	"github.com/open-cluster-management/multicluster-observability-operator/pkg/config"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ManagedClusterAddonName = "observability-controller"
	Annotation              = `
	[{"signerName":"kubernetes.io/kube-apiserver-client"},{"signerName":"open-cluster-management.io/observability-signer","subject":{"organization":["open-cluster-management-observability"],"organizationalUnit":["acm"],"commonName":"managed-cluster-observability"}}]
`
)

var (
	spokeNameSpace = os.Getenv("SPOKE_NAMESPACE")
)

func CreateManagedClusterAddonCR(client client.Client, name string, namespace string) error {
	managedClusterAddon := &addonv1alpha1.ManagedClusterAddOn{}
	// check if managedClusterAddon exists
	if err := client.Get(
		context.TODO(),
		types.NamespacedName{
			Name:      ManagedClusterAddonName,
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
				Name:      ManagedClusterAddonName,
				Namespace: namespace,
				Annotations: map[string]string{
					"addon.open-cluster-management.io/installNamespace": spokeNameSpace,
					"addon.open-cluster-management.io/registrations":    Annotation,
				},
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
				Registrations: []addonv1alpha1.RegistrationConfig{
					{
						SignerName: "kubernetes.io/kube-apiserver-client",
					},
					{
						SignerName: "open-cluster-management.io/observability-signer",
						Subject: addonv1alpha1.Subject{
							User:              "managed-cluster-observability",
							Groups:            []string{"open-cluster-management-observability"},
							OrganizationUnits: []string{config.ManagedClusterOU},
						},
					},
				},
			},
		}
		if err := client.Create(context.TODO(), newManagedClusterAddon); err != nil {
			log.Error(err, "Cannot create observability-controller  ManagedClusterAddOn")
			return err
		}
		if err := client.Status().Update(context.TODO(), newManagedClusterAddon); err != nil {
			log.Error(err, "Cannot update status for observability-controller  ManagedClusterAddOn")
			return err
		}
	} else if err != nil {
		log.Error(err, "Failed to get ManagedClusterAddOn ", "namespace", namespace)
		return err
	}
	log.Info("ManagedClusterAddOn already present", "namespace", namespace)

	return nil
}
