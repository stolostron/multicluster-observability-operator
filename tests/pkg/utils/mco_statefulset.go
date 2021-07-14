// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package utils

import (
	"context"

	appv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"
)

func GetStatefulSet(opt TestOptions, isHub bool, name string,
	namespace string) (*appv1.StatefulSet, error) {
	clientKube := getKubeClient(opt, isHub)
	sts, err := clientKube.AppsV1().StatefulSets(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		klog.Errorf("Failed to get statefulset %s in namespace %s due to %v", name, namespace, err)
	}
	return sts, err
}

func GetStatefulSetWithLabel(opt TestOptions, isHub bool, label string,
	namespace string) (*appv1.StatefulSetList, error) {
	clientKube := getKubeClient(opt, isHub)
	sts, err := clientKube.AppsV1().StatefulSets(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: label,
	})

	if err != nil {
		klog.Errorf("Failed to get statefulset with label selector %s in namespace %s due to %v", label, namespace, err)
	}
	return sts, err
}
