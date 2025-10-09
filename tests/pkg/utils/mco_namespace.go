// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

import (
	"context"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"
)

func GetNamespace(opt TestOptions, isHub bool, namespace string) (error, *v1.Namespace) {
	clientKube := GetKubeClient(opt, isHub)

	ns, err := clientKube.CoreV1().Namespaces().Get(context.TODO(), namespace, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		klog.Errorf("Failed to get namespace %s due to %v", namespace, err)
		return err, nil
	}
	return nil, ns
}
