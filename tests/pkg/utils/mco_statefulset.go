// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

import (
	"context"
	"fmt"

	. "github.com/onsi/gomega"
	appv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"
)

func GetStatefulSet(opt TestOptions, isHub bool, name string,
	namespace string) (*appv1.StatefulSet, error) {
	clientKube := GetKubeClient(opt, isHub)
	sts, err := clientKube.AppsV1().StatefulSets(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		klog.Errorf("Failed to get statefulset %s in namespace %s due to %v", name, namespace, err)
	}
	return sts, err
}

func GetStatefulSetWithCluster(cluster Cluster, name string,
	namespace string) (*appv1.StatefulSet, error) {
	clientKube := GetKubeClientWithCluster(cluster)
	sts, err := clientKube.AppsV1().StatefulSets(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		klog.Errorf("Failed to get statefulset %s in namespace %s due to %v", name, namespace, err)
	}
	return sts, err
}

func GetStatefulSetWithLabel(opt TestOptions, isHub bool, label string,
	namespace string) (*appv1.StatefulSetList, error) {
	clientKube := GetKubeClient(opt, isHub)
	sts, err := clientKube.AppsV1().StatefulSets(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: label,
	})

	if err != nil {
		klog.Errorf(
			"Failed to get statefulset with label selector %s in namespace %s due to %v",
			label,
			namespace,
			err)
	}
	return sts, err
}

func CheckStatefulSetAvailability(cluster Cluster, name, namespace string, shouldExist bool) {
	if shouldExist {
		Eventually(func() error {
			sts, err := GetStatefulSetWithCluster(cluster, name, namespace)
			if err != nil {
				return err
			}
			if sts.Status.ReadyReplicas != *sts.Spec.Replicas {
				return fmt.Errorf("statefulset %s/%s is not ready: %d/%d", namespace, name, sts.Status.ReadyReplicas, *sts.Spec.Replicas)
			}
			return nil
		}, 300, 1).Should(Not(HaveOccurred()))
	} else {
		Eventually(func() error {
			_, err := GetStatefulSetWithCluster(cluster, name, namespace)
			return err
		}, 300, 1).Should(HaveOccurred())
	}
}

func CheckStatefulSetAvailabilityOnClusters(clusters []Cluster, name, namespace string, shouldExist bool) {
	for _, cluster := range clusters {
		CheckStatefulSetAvailability(cluster, name, namespace, shouldExist)
	}
}
