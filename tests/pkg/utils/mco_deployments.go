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

func GetDeployment(opt TestOptions, isHub bool, name string,
	namespace string) (*appv1.Deployment, error) {
	clientKube := getKubeClient(opt, isHub)

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

func GetDeploymentWithLabel(opt TestOptions, isHub bool, label string,
	namespace string) (*appv1.DeploymentList, error) {
	clientKube := getKubeClient(opt, isHub)

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
	clientKube := getKubeClient(opt, isHub)
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
	clientKube := getKubeClient(opt, isHub)
	updateDep, err := clientKube.AppsV1().Deployments(namespace).Update(context.TODO(), dep, metav1.UpdateOptions{})
	if err != nil {
		klog.Errorf("Failed to update deployment %s in namespace %s due to %v", name, namespace, err)
	}
	return updateDep, err
}
