// Copyright (c) 2020 Red Hat, Inc.

package multiclusterobservability

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mcov1beta1 "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/apis/observability/v1beta1"
	mcoconfig "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/config"
)

func GenerateObjectStorageSecret(
	c client.Client,
	mco *mcov1beta1.MultiClusterObservability) error {

	found := &corev1.Secret{}
	namespacedName := types.NamespacedName{
		Name:      mcoconfig.DefaultObjStorageSecretName,
		Namespace: mcoconfig.GetDefaultNamespace(),
	}

	err := c.Get(context.TODO(), namespacedName, found)
	if err != nil {
		if errors.IsNotFound(err) {
			if err := newObjectStorageSecret(c, mco); err != nil {
				return err
			}
			return nil
		}
		return err
	}
	return nil
}

func getObjStorageConf() string {
	objStorageConf := fmt.Sprintf(`type: %s
config:
  bucket: "%s"
  endpoint: "%s"
  insecure: %v
  access_key: "%s"
  secret_key: "%s"`,
		mcoconfig.DefaultObjStorageType,
		mcoconfig.DefaultObjStorageBucket,
		mcoconfig.DefaultObjStorageEndpoint,
		mcoconfig.DefaultObjStorageInsecure,
		mcoconfig.DefaultObjStorageAccesskey,
		mcoconfig.DefaultObjStorageSecretkey,
	)

	return objStorageConf
}

func newObjectStorageSecret(
	c client.Client,
	mco *mcov1beta1.MultiClusterObservability) error {

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mcoconfig.DefaultObjStorageSecretName,
			Namespace: mcoconfig.GetDefaultNamespace(),
		},

		Type: "Opaque",
		StringData: map[string]string{
			mcoconfig.DefaultObjStorageSecretName: getObjStorageConf(),
		},
	}

	return c.Create(context.TODO(), secret)
}
