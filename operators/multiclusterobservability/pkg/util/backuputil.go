// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package util

import (
	"context"

	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func AddBackupLabelToConfigMap(ctx context.Context, c client.Client, name, namespace string) error {
	m := &corev1.ConfigMap{}
	err := c.Get(ctx, types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}, m)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			log.Error(err, "ConfigMap not found", "ConfigMap", name)
			return nil
		}
		return err
	}

	if _, ok := m.Labels[config.BackupLabelName]; !ok {
		if m.Labels == nil {
			m.Labels = make(map[string]string)
		}
		m.Labels[config.BackupLabelName] = config.BackupLabelValue
		err := c.Update(ctx, m)
		if err != nil {
			return err
		}
		log.Info("Add backup label for configMap", "name", name)
	}
	return nil
}

func AddBackupLabelToSecret(ctx context.Context, c client.Client, name, namespace string) error {
	s := &corev1.Secret{}
	err := c.Get(ctx, types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}, s)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			log.Error(err, "Secret not found", "Secret", name)
			return nil
		}
		return err
	}

	return AddBackupLabelToSecretObj(ctx, c, s)
}

func AddBackupLabelToSecretObj(ctx context.Context, c client.Client, s *corev1.Secret) error {
	if _, ok := s.Labels[config.BackupLabelName]; !ok {
		if s.Labels == nil {
			s.Labels = make(map[string]string)
		}
		s.Labels[config.BackupLabelName] = config.BackupLabelValue
		err := c.Update(ctx, s)
		if err != nil {
			return err
		}
		log.Info("Add backup label for secret", "name", s.Name)
	}
	return nil
}
