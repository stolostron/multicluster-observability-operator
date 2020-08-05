// Copyright (c) 2020 Red Hat, Inc.

package multiclustermonitoring

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	monitoringv1alpha1 "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/apis/monitoring/v1alpha1"
	mcoconfig "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/config"
)

const (
	// TODO: get it from cr spec when PR merged
	defaultObjStorageSecretName = "thanos-objectstorage"
	// #nosec
	defaultObjStorageSecretKey = "thanos.yaml"
)

func GenerateObjectStorageSecret(
	c client.Client,
	mco *monitoringv1alpha1.MultiClusterObservability) (*reconcile.Result, error) {

	found := &corev1.Secret{}
	namespacedName := types.NamespacedName{
		Name:      defaultObjStorageSecretName,
		Namespace: mcoconfig.GetDefaultNamespace(),
	}

	err := c.Get(context.TODO(), namespacedName, found)
	if err != nil {
		if errors.IsNotFound(err) {
			if err := newObjectStorageSecret(c); err != nil {
				return &reconcile.Result{}, err
			}
			return nil, nil
		}
		return &reconcile.Result{}, err
	} else {
		_, ok := found.Data[defaultObjStorageSecretKey]
		if !ok {
			if err := newObjectStorageSecret(c); err != nil {
				return &reconcile.Result{}, err
			}
		}
	}
	return nil, nil
}

func newObjectStorageSecret(c client.Client) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      defaultObjStorageSecretName,
			Namespace: mcoconfig.GetDefaultNamespace(),
		},

		Type: "Opaque",
		StringData: map[string]string{
			defaultObjStorageSecretKey: `type: s3
config:
  bucket: "thanos"
  endpoint: "minio:9000"
  insecure: true
  access_key: "minio"
  secret_key: "minio123"`}}

	return c.Create(context.TODO(), secret)
}
