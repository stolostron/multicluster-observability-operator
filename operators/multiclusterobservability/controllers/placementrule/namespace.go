// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package placementrule

import (
	"os"

	operatorconfig "github.com/stolostron/multicluster-observability-operator/operators/pkg/config"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
