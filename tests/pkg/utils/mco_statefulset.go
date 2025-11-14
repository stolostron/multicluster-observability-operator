// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

import (
	"context"
	"fmt"

	. "github.com/onsi/gomega"
	appv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
	klog.V(1).Infof("Get statefulset <%v> in namespace <%v> on cluster <%v>", name, namespace, cluster.Name)
	return clientKube.AppsV1().StatefulSets(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func GetStatefulSetWithLabel(opt TestOptions, isHub bool, label string,
	namespace string) (*appv1.StatefulSetList, error) {
	clientKube := GetKubeClient(opt, isHub)
	sts, err := clientKube.AppsV1().StatefulSets(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: label,
	})
	if err != nil {
		klog.Errorf("Failed to get statefulset with label %s in namespace %s due to %v", label, namespace, err)
		return nil, err
	}
	return sts, nil
}

func CheckStatefulSetAvailability(cluster Cluster, name, namespace string, shouldExist bool) {
	if shouldExist {
		Eventually(func() error {
			sts, err := GetStatefulSetWithCluster(cluster, name, namespace)
			if err != nil {
				klog.Errorf("Failed to get statefulset %s/%s: %v", name, namespace, err)
				return fmt.Errorf("failed to get statefulset %s/%s: %w", name, namespace, err)
			}
			if sts.Status.ReadyReplicas != *sts.Spec.Replicas {
				klog.Errorf("Statefulset %s/%s is not ready: %d/%d", namespace, name, sts.Status.ReadyReplicas, *sts.Spec.Replicas)
				return fmt.Errorf("statefulset %s/%s is not ready: %d/%d", namespace, name, sts.Status.ReadyReplicas, *sts.Spec.Replicas)
			}
			return nil
		}, 300, 5).Should(Succeed())
	} else {
		Eventually(func() error {
			_, err := GetStatefulSetWithCluster(cluster, name, namespace)
			if apierrors.IsNotFound(err) {
				return nil
			}
			if err != nil {
				klog.Errorf("Failed to get statefulset %s/%s: %v", name, namespace, err)
				return fmt.Errorf("failed to get statefulset %s/%s: %w", name, namespace, err)
			}
			return fmt.Errorf("statefulset %s/%s still exists", namespace, name)
		}, 300, 5).Should(Succeed())
	}
}

func CheckStatefulSetAvailabilityOnClusters(clusters []Cluster, name, namespace string, shouldExist bool) {
	for _, cluster := range clusters {
		CheckStatefulSetAvailability(cluster, name, namespace, shouldExist)
	}
}
