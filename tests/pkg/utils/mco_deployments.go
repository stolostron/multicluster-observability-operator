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

func GetDeployment(opt TestOptions, isHub bool, name string,
	namespace string) (*appv1.Deployment, error) {
	clientKube := GetKubeClient(opt, isHub)

	cluster := opt.HubCluster.BaseDomain
	if !isHub {
		cluster = opt.ManagedClusters[0].BaseDomain
	}

	klog.V(1).Infof("Get deployment <%v> in namespace <%v>, isHub: <%v>, cluster: <%v>", name, namespace, isHub, cluster)
	dep, err := clientKube.AppsV1().Deployments(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		klog.Errorf("Failed to get deployment %s in namespace %s due to %v", name, namespace, err)
	}
	return dep, err
}

func GetDeploymentWithCluster(cluster Cluster, name string,
	namespace string) (*appv1.Deployment, error) {
	clientKube := GetKubeClientWithCluster(cluster)
	klog.V(1).Infof("Get deployment <%v> in namespace <%v> on cluster <%v>", name, namespace, cluster.Name)
	dep, err := clientKube.AppsV1().Deployments(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	return dep, err
}

func GetDeploymentWithLabel(opt TestOptions, isHub bool, label string,
	namespace string) (*appv1.DeploymentList, error) {
	clientKube := GetKubeClient(opt, isHub)

	cluster := opt.HubCluster.BaseDomain
	if !isHub {
		cluster = opt.ManagedClusters[0].BaseDomain
	}

	klog.V(1).Infof("Get get deployment with label selector <%v> in namespace <%v>, isHub: <%v>, cluster: <%v>",
		label,
		namespace,
		isHub,
		cluster)
	deps, err := clientKube.AppsV1().Deployments(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: label,
	})
	if err != nil {
		klog.Errorf("Failed to get deployment with label selector %s in namespace %s due to %v", label, namespace, err)
	}

	return deps, err
}

func DeleteDeployment(opt TestOptions, isHub bool, name string, namespace string) error {
	clientKube := GetKubeClient(opt, isHub)
	err := clientKube.AppsV1().Deployments(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
	if err != nil {
		klog.Errorf("Failed to delete deployment %s in namespace %s due to %v", name, namespace, err)
	}
	return err
}

func UpdateDeployment(
	opt TestOptions,
	isHub bool,
	name string,
	namespace string,
	dep *appv1.Deployment) (*appv1.Deployment, error) {
	clientKube := GetKubeClient(opt, isHub)
	updateDep, err := clientKube.AppsV1().Deployments(namespace).Update(context.TODO(), dep, metav1.UpdateOptions{})
	if err != nil {
		klog.Errorf("Failed to update deployment %s in namespace %s due to %v", name, namespace, err)
	}
	return updateDep, err
}

func CheckDeploymentAvailability(cluster Cluster, name, namespace string, shouldExist bool) {
	if shouldExist {
		Eventually(func() error {
			dep, err := GetDeploymentWithCluster(cluster, name, namespace)
			if err != nil {
				klog.Errorf("Failed to get deployment %s in namespace %s due to %v", name, namespace, err)
				return err
			}
			if dep.Status.ReadyReplicas != *dep.Spec.Replicas {
				return fmt.Errorf("deployment %s/%s is not ready: %d/%d", namespace, name, dep.Status.ReadyReplicas, *dep.Spec.Replicas)
			}
			return nil
		}, 300, 2).Should(Not(HaveOccurred()))
	} else {
		Eventually(func() error {
			_, err := GetDeploymentWithCluster(cluster, name, namespace)
			if apierrors.IsNotFound(err) {
				return nil
			}
			if err != nil {
				return err
			}
			return fmt.Errorf("deployment %s/%s still exists", namespace, name)
		}, 300, 2).Should(Succeed())
	}
}

func CheckDeploymentAvailabilityOnClusters(clusters []Cluster, name, namespace string, shouldExist bool) {
	for _, cluster := range clusters {
		CheckDeploymentAvailability(cluster, name, namespace, shouldExist)
	}
}
