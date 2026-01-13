// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package util

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
)

func AddBackupLabelToConfigMap(c client.Client, name, namespace string) error {
	m := &corev1.ConfigMap{}
	err := c.Get(context.TODO(), types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}, m)

	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			log.Error(err, "ConfigMap not found", "ConfigMap", name)
			return nil
		} else {
			return err
		}
	}

	if _, ok := m.ObjectMeta.Labels[config.BackupLabelName]; !ok {
		if m.ObjectMeta.Labels == nil {
			m.ObjectMeta.Labels = make(map[string]string)
		}
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

func AddBackupLabelToSecret(c client.Client, name, namespace string) error {
	s := &corev1.Secret{}
	err := c.Get(context.TODO(), types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}, s)

	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			log.Error(err, "Secret not found", "Secret", name)
			return nil
		} else {
			return err
		}
	}

	return AddBackupLabelToSecretObj(c, s)
}

func AddBackupLabelToSecretObj(c client.Client, s *corev1.Secret) error {
	if _, ok := s.ObjectMeta.Labels[config.BackupLabelName]; !ok {
		if s.ObjectMeta.Labels == nil {
			s.ObjectMeta.Labels = make(map[string]string)
		}
		s.ObjectMeta.Labels[config.BackupLabelName] = config.BackupLabelValue
		err := c.Update(context.TODO(), s)
		if err != nil {
			return err
		} else {
			log.Info("Add backup label for secret", "name", s.ObjectMeta.Name)
		}
	}
	return nil
}
