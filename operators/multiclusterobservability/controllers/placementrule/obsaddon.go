// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package placementrule

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	obsv1beta1 "github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
	"github.com/open-cluster-management/multicluster-observability-operator/operators/pkg/util"
)

const (
	obsAddonName      = "observability-addon"
	obsAddonFinalizer = "observability.open-cluster-management.io/addon-cleanup"
)

func deleteObsAddon(c client.Client, namespace string) error {
	found := &obsv1beta1.ObservabilityAddon{}
	err := c.Get(context.TODO(), types.NamespacedName{Name: obsAddonName, Namespace: namespace}, found)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		log.Error(err, "Failed to check observabilityaddon cr before delete", "namespace", namespace)
		return err
	}

	err = c.Delete(context.TODO(), found)
	if err != nil {
		log.Error(err, "Failed to delete observabilityaddon", "namespace", namespace)
	}

	err = removeObservabilityAddon(c, namespace)
	if err != nil {
		return err
	}

	// forcely remove observabilityaddon if it's already stuck in Terminating more than 5 minutes
	time.AfterFunc(time.Duration(5)*time.Minute, func() {
		err := deleteStaleObsAddon(c, namespace, false)
		if err != nil {
			log.Error(err, "Failed to forcely remove observabilityaddon", "namespace", namespace)
		}
	})

	log.Info("observabilityaddon is deleted", "namespace", namespace)
	return nil
}

func createObsAddon(c client.Client, namespace string) error {
	ec := &obsv1beta1.ObservabilityAddon{
		ObjectMeta: metav1.ObjectMeta{
			Name:      obsAddonName,
			Namespace: namespace,
			Labels: map[string]string{
				ownerLabelKey: ownerLabelValue,
			},
		},
	}
	found := &obsv1beta1.ObservabilityAddon{}
	err := c.Get(context.TODO(), types.NamespacedName{Name: obsAddonName, Namespace: namespace}, found)
	if err != nil && errors.IsNotFound(err) || err == nil && found.GetDeletionTimestamp() != nil {
		if err == nil {
			err = deleteFinalizer(c, found)
			if err != nil {
				return err
			}
		}
		log.Info("Creating observabilityaddon cr", "namespace", namespace)
		err = c.Create(context.TODO(), ec)
		if err != nil {
			log.Error(err, "Failed to create observabilityaddon cr")
			return err
		}
		return nil
	} else if err != nil {
		log.Error(err, "Failed to check observabilityaddon cr before create")
		return err
	}

	log.Info("observabilityaddon already existed/unchanged", "namespace", namespace)
	return nil
}

func deleteStaleObsAddon(c client.Client, namespace string, isForce bool) error {
	found := &obsv1beta1.ObservabilityAddon{}
	err := c.Get(context.TODO(), types.NamespacedName{Name: obsAddonName, Namespace: namespace}, found)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		log.Error(err, "Failed to check observabilityaddon cr before delete stale ones", "namespace", namespace)
		return err
	}
	if found.GetDeletionTimestamp() == nil && !isForce {
		log.Info("observabilityaddon is not in Terminating status, skip", "namespace", namespace)
		return nil
	}
	err = deleteFinalizer(c, found)
	if err != nil {
		return err
	}
	obsaddon := &obsv1beta1.ObservabilityAddon{
		ObjectMeta: metav1.ObjectMeta{
			Name:      obsAddonName,
			Namespace: namespace,
		},
	}
	err = c.Delete(context.TODO(), obsaddon)
	if err != nil && !errors.IsNotFound(err) {
		log.Error(err, "Failed to delete observabilityaddon", "namespace", namespace)
		return err
	}
	log.Info("observabilityaddon is deleted thoroughly", "namespace", namespace)
	return nil
}

func deleteFinalizer(c client.Client, obsaddon *obsv1beta1.ObservabilityAddon) error {
	if util.Contains(obsaddon.GetFinalizers(), obsAddonFinalizer) {
		obsaddon.SetFinalizers(util.Remove(obsaddon.GetFinalizers(), obsAddonFinalizer))
		err := c.Update(context.TODO(), obsaddon)
		if err != nil {
			log.Error(err, "Failed to delete finalizer in observabilityaddon", "namespace", obsaddon.Namespace)
			return err
		}
		log.Info("observabilityaddon's finalizer is deleted", "namespace", obsaddon.Namespace)
	}
	return nil
}
