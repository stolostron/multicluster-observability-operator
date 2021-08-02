// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package utils

import (
	"context"
	"errors"

	appv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"
)

func GetDeployment(opt TestOptions, isHub bool, name string,
	namespace string) (*appv1.Deployment, error) {
	clientKube := getKubeClient(opt, isHub)
	dep, err := clientKube.AppsV1().Deployments(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		klog.Errorf("Failed to get deployment %s in namespace %s due to %v", name, namespace, err)
	}
	return dep, err
}

func GetDeploymentWithLabel(opt TestOptions, isHub bool, label string,
	namespace string) (*appv1.DeploymentList, error) {
	clientKube := getKubeClient(opt, isHub)
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

func UpdateDeploymentReplicas(opt TestOptions, deployName, crProperty string, desiredReplicas, expectedReplicas int32) error {
	clientDynamic := GetKubeClientDynamic(opt, true)
	deploy, err := GetDeployment(opt, true, deployName, MCO_NAMESPACE)
	if err != nil {
		return err
	}
	deploy.Spec.Replicas = &desiredReplicas
	_, err = UpdateDeployment(opt, true, deployName, MCO_NAMESPACE, deploy)
	if err != nil {
		return err
	}

	obs, err := clientDynamic.Resource(NewMCOMObservatoriumGVR()).Namespace(MCO_NAMESPACE).Get(context.TODO(), MCO_CR_NAME, metav1.GetOptions{})
	if err != nil {
		return err
	}
	thanos := obs.Object["spec"].(map[string]interface{})["thanos"]
	currentReplicas := thanos.(map[string]interface{})[crProperty].(map[string]interface{})["replicas"].(int64)
	if int(currentReplicas) != int(expectedReplicas) {
		klog.Errorf("Failed to update deployment %s replicas to %v", deployName, expectedReplicas)
		return errors.New("the replicas was not updated successfully")
	}
	return nil
}
