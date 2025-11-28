// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"
)

func GetConfigMap(opt TestOptions, isHub bool, name string,
	namespace string) (error, *corev1.ConfigMap) {
	clientKube := GetKubeClient(opt, isHub)
	cm, err := clientKube.CoreV1().ConfigMaps(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		klog.Errorf("Failed to get configmap %s in namespace %s due to %v", name, namespace, err)
	}
	return err, cm
}

func DeleteConfigMap(opt TestOptions, isHub bool, name string, namespace string) error {
	clientKube := GetKubeClient(opt, isHub)
	err := clientKube.CoreV1().ConfigMaps(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
	if err != nil {
		klog.Errorf("Failed to delete configmap %s in namespace %s due to %v", name, namespace, err)
	}
	return err
}
