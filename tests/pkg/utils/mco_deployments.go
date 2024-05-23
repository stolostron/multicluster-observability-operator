// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

import (
	"context"

	appv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"
)

func GetDeployment(clusterConfig Cluster, isHub bool, name string,
	namespace string) (*appv1.Deployment, error) {
	clientKube := getKubeClientForCluster(clusterConfig, isHub)
	klog.V(1).Infof("Get deployment <%v> in namespace <%v>, isHub: <%v>", name, namespace, isHub)
	dep, err := clientKube.AppsV1().Deployments(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		klog.Errorf("Failed to get deployment %s in namespace %s due to %v", name, namespace, err)
	}
	return dep, err
}

func GetDeploymentWithLabel(opt TestOptions, isHub bool, label string,
	namespace string) (*appv1.DeploymentList, error) {
	clientKube := getKubeClient(opt, isHub)
	klog.V(1).Infof("Get get deployment with label selector <%v> in namespace <%v>, isHub: <%v>",
		label,
		namespace,
		isHub)
	deps, err := clientKube.AppsV1().Deployments(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: label,
	})
	if err != nil {
		klog.Errorf("Failed to get deployment with label selector %s in namespace %s due to %v", label, namespace, err)
	}

	return deps, err
}

func DeleteDeployment(opt TestOptions, isHub bool, name string, namespace string) error {
	clientKube := getKubeClient(opt, isHub)
	err := clientKube.AppsV1().Deployments(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
	if err != nil {
		klog.Errorf("Failed to delete deployment %s in namespace %s due to %v", name, namespace, err)
	}
	return err
}

func UpdateDeployment(
	clusterConfig Cluster,
	isHub bool,
	name string,
	namespace string,
	dep *appv1.Deployment) (*appv1.Deployment, error) {
	clientKube := getKubeClientForCluster(clusterConfig, isHub)
	updateDep, err := clientKube.AppsV1().Deployments(namespace).Update(context.TODO(), dep, metav1.UpdateOptions{})
	if err != nil {
		klog.Errorf("Failed to update deployment %s in namespace %s due to %v", name, namespace, err)
	}
	return updateDep, err
}
