// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package util

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
)

func AddBackupLabelToConfigMap(c client.Client, name string, namespace string) error {
	m := &corev1.ConfigMap{}
	err := c.Get(context.TODO(), types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}, m)
	if err != nil {
		return err
	}
	if _, ok := m.ObjectMeta.Labels[config.BackupLabelName]; !ok {
		m.ObjectMeta.Labels[config.BackupLabelName] = config.BackupLabelValue
		err := c.Update(context.TODO(), m)
		if err != nil {
			return err
		} else {
			log.Info("Add backup label for configMap", "name", name)
		}

	}
	return nil
}

func AddBackupLabelToSecret(c client.Client, name string, namespace string) error {
	s := &corev1.Secret{}
	err := c.Get(context.TODO(), types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}, s)
	if err != nil {
		return err
	}
	if _, ok := s.ObjectMeta.Labels[config.BackupLabelName]; !ok {
		s.ObjectMeta.Labels[config.BackupLabelName] = config.BackupLabelValue
		err := c.Update(context.TODO(), s)
		if err != nil {
			return err
		} else {
			log.Info("Add backup label for secret", "name", name)
		}
	}
	return nil
}
