// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package utils

import (
	appv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"
)

func GetStatefulSet(opt TestOptions, isHub bool, name string,
	namespace string) (error, *appv1.StatefulSet) {
	clientKube := getKubeClient(opt, isHub)
	sts, err := clientKube.AppsV1().StatefulSets(namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		klog.Errorf("Failed to get statefulset %s in namespace %s due to %v", name, namespace, err)
	}
	return err, sts
}
