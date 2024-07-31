// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package placementrule

import (
	"context"
	"fmt"
	ocinfrav1 "github.com/openshift/api/config/v1"
	"k8s.io/apimachinery/pkg/types"
	"os"
	client "sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	operatorconfig "github.com/stolostron/multicluster-observability-operator/operators/pkg/config"
)

var (
	spokeNameSpace = os.Getenv("SPOKE_NAMESPACE")
)

func GetClusterID(ctx context.Context, c client.Client) (string, error) {
	clusterVersion := &ocinfrav1.ClusterVersion{}
	if err := c.Get(ctx, types.NamespacedName{Name: "version"}, clusterVersion); err != nil {
		return "", fmt.Errorf("failed to get clusterVersion: %w", err)
	}

	return string(clusterVersion.Spec.ClusterID), nil
}

func getSpokeNameSpace(c client.Client) string {
	clusterID, _ := GetClusterID(context.TODO(), c)
	spokeNameSpace = spokeNameSpace + "-" + clusterID
	return spokeNameSpace
}

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
