// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package microshift

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// IsMicroshiftCluster checks if the cluster is a microshift cluster.
// It verifies the existence of the configmap microshift-version in namespace kube-public.
// If the configmap exists, it returns the version of the microshift cluster.
// If the configmap does not exist, it returns an empty string.
func IsMicroshiftCluster(ctx context.Context, client client.Client) (string, error) {
	res := &corev1.ConfigMap{}
	err := client.Get(ctx, types.NamespacedName{
		Name:      "microshift-version",
		Namespace: "kube-public",
	}, res)
	if err != nil {
		if errors.IsNotFound(err) {
			return "", nil
		}
		return "", err
	}

	return res.Data["version"], nil
}
