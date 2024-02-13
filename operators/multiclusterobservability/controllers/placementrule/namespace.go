// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package placementrule

import (
	"context"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"os"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	operatorconfig "github.com/stolostron/multicluster-observability-operator/operators/pkg/config"
)

var (
	spokeNameSpace = os.Getenv("SPOKE_NAMESPACE")
)

func generateNamespace() *corev1.Namespace {
	return &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "Namespace",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: spokeNameSpace,
			Annotations: map[string]string{
				operatorconfig.WorkloadPartitioningNSAnnotationsKey: operatorconfig.WorkloadPartitioningNSExpectedValue,
			},
		},
	}
}

func generateLocalClusterNamespace(r client.Client) error {
	namespace := &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "Namespace",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: localClusterName,
			Annotations: map[string]string{
				operatorconfig.WorkloadPartitioningNSAnnotationsKey: operatorconfig.WorkloadPartitioningNSExpectedValue,
			},
		},
	}
	found := &corev1.Namespace{}
	err := r.Get(context.TODO(), types.NamespacedName{Name: localClusterName}, found)
	if err != nil {
		if errors.IsNotFound(err) {
			err = r.Create(context.TODO(), namespace)
			if err != nil {
				log.Error(err, "Failed to create namespace", "namespace", namespace.Name)
				return err
			}
		}
	}
	return nil
}
