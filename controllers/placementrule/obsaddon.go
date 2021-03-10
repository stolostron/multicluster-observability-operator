// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package placementrule

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	obv1beta1 "github.com/open-cluster-management/multicluster-observability-operator/api/v1beta1"
	"github.com/open-cluster-management/multicluster-observability-operator/pkg/util"
)

const (
	obsAddonName      = "observability-addon"
	obsAddonFinalizer = "observability.open-cluster-management.io/addon-cleanup"
)

func deleteObsAddon(client client.Client, namespace string) error {
	found := &obv1beta1.ObservabilityAddon{}
	err := client.Get(context.TODO(), types.NamespacedName{Name: obsAddonName, Namespace: namespace}, found)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		log.Error(err, "Failed to check observabilityaddon cr before delete", "namespace", namespace)
		return err
	}
	err = client.Delete(context.TODO(), found)
	if err != nil {
		log.Error(err, "Failed to delete observabilityaddon", "namespace", namespace)
	}

	err = removeObservabilityAddon(client, namespace)
	if err != nil {
		return err
	}

	log.Info("observabilityaddon is deleted", "namespace", namespace)
	return nil
}

func createObsAddon(client client.Client, namespace string) error {
	ec := &obv1beta1.ObservabilityAddon{
		ObjectMeta: metav1.ObjectMeta{
			Name:      obsAddonName,
			Namespace: namespace,
			Labels: map[string]string{
				ownerLabelKey: ownerLabelValue,
			},
		},
	}
	found := &obv1beta1.ObservabilityAddon{}
	err := client.Get(context.TODO(), types.NamespacedName{Name: obsAddonName, Namespace: namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating observabilityaddon cr", "namespace", namespace)
		err = client.Create(context.TODO(), ec)
		if err != nil {
			log.Error(err, "Failed to create observabilityaddon cr")
			return err
		}
		return nil
	} else if err != nil {
		log.Error(err, "Failed to check observabilityaddon cr before create")
		return err
	}

	log.Info("endponitmetrics already existed/unchanged", "namespace", namespace)
	return nil
}

func deleteStaleObsAddon(c client.Client, namespace string) error {
	found := &obv1beta1.ObservabilityAddon{}
	err := c.Get(context.TODO(), types.NamespacedName{Name: obsAddonName, Namespace: namespace}, found)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		log.Error(err, "Failed to check observabilityaddon cr before delete stale ones", "namespace", namespace)
		return err
	}
	if found.GetDeletionTimestamp() != nil && util.Contains(found.GetFinalizers(), obsAddonFinalizer) {
		found.SetFinalizers(util.Remove(found.GetFinalizers(), obsAddonFinalizer))
		err = c.Update(context.TODO(), found)
		if err != nil {
			log.Error(err, "Failed to delete finalizer in observabilityaddon", "namespace", namespace)
			return err
		}
		log.Info("observabilityaddon's finalizer is deleted", "namespace", namespace)
	}

	log.Info("observabilityaddon's finalizer is deleted", "namespace", namespace)
	return nil
}
