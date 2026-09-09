// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

import (
	"context"
	"fmt"
	"strings"

	"github.com/onsi/gomega"
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

func GetDeployment(opt TestOptions, isHub bool, name string,
	namespace string,
) (*appv1.Deployment, error) {
	clientKube := GetKubeClient(opt, isHub)

	cluster := opt.HubCluster.BaseDomain
	if !isHub {
		cluster = opt.ManagedClusters[0].BaseDomain
	}

	klog.V(3).Infof("Get deployment <%v> in namespace <%v>, isHub: <%v>, cluster: <%v>", name, namespace, isHub, cluster)
	dep, err := clientKube.AppsV1().Deployments(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		klog.Errorf("Failed to get deployment %s in namespace %s due to %v", name, namespace, err)
	}
	return dep, err
}

func GetDeploymentWithCluster(cluster Cluster, name string,
	namespace string,
) (*appv1.Deployment, error) {
	clientKube := GetKubeClientWithCluster(cluster)
	klog.V(3).Infof("Get deployment <%v> in namespace <%v> on cluster <%v>", name, namespace, cluster.Name)
	dep, err := clientKube.AppsV1().Deployments(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	return dep, err
}

func GetDeploymentWithLabel(opt TestOptions, isHub bool, label string,
	namespace string,
) (*appv1.DeploymentList, error) {
	clientKube := GetKubeClient(opt, isHub)

	cluster := opt.HubCluster.BaseDomain
	if !isHub {
		cluster = opt.ManagedClusters[0].BaseDomain
	}

	klog.V(3).Infof("Get deployment with label selector <%v> in namespace <%v>, isHub: <%v>, cluster: <%v>",
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
	dep *appv1.Deployment,
) (*appv1.Deployment, error) {
	clientKube := GetKubeClient(opt, isHub)
	updateDep, err := clientKube.AppsV1().Deployments(namespace).Update(context.TODO(), dep, metav1.UpdateOptions{})
	if err != nil {
		klog.Errorf("Failed to update deployment %s in namespace %s due to %v", name, namespace, err)
	}
	return updateDep, err
}

// FormatDeploymentNotReadyError formats an informative error message when a deployment
// has not yet reached its expected ready replicas, including any negative conditions.
func FormatDeploymentNotReadyError(dep *appv1.Deployment, namespace, name string) error {
	if dep == nil {
		return fmt.Errorf("deployment %s/%s is nil", namespace, name)
	}
	expectedReplicas := int32(1)
	if dep.Spec.Replicas != nil {
		expectedReplicas = *dep.Spec.Replicas
	}
	conds := make([]string, 0, len(dep.Status.Conditions))
	for _, c := range dep.Status.Conditions {
		if (c.Type == appv1.DeploymentReplicaFailure && c.Status == corev1.ConditionTrue) ||
			(c.Type != appv1.DeploymentReplicaFailure && c.Status == corev1.ConditionFalse) {
			conds = append(conds, fmt.Sprintf("%s=%s(%s: %s)", c.Type, c.Status, c.Reason, c.Message))
		}
	}
	condMsg := ""
	if len(conds) > 0 {
		condMsg = fmt.Sprintf(", conditions: [%s]", strings.Join(conds, ", "))
	}
	return fmt.Errorf("deployment %s/%s is not ready: %d/%d ready replicas%s",
		namespace, name, dep.Status.ReadyReplicas, expectedReplicas, condMsg)
}

// FormatDeploymentStillExistsError formats an informative error message when a deployment
// unexpectedly still exists, including termination state, finalizers, and replica details.
func FormatDeploymentStillExistsError(dep *appv1.Deployment, namespace, name string) error {
	if dep == nil {
		return fmt.Errorf("deployment %s/%s is nil", namespace, name)
	}
	if dep.DeletionTimestamp != nil {
		return fmt.Errorf("deployment %s/%s still exists (terminating since %v, finalizers: %v)",
			namespace, name, dep.DeletionTimestamp.Time, dep.Finalizers)
	}
	expectedReplicas := int32(1)
	if dep.Spec.Replicas != nil {
		expectedReplicas = *dep.Spec.Replicas
	}
	return fmt.Errorf("deployment %s/%s still exists (replicas: %d/%d, generation: %d, observedGeneration: %d)",
		namespace, name, dep.Status.ReadyReplicas, expectedReplicas, dep.Generation, dep.Status.ObservedGeneration)
}

func CheckDeploymentAvailability(cluster Cluster, name, namespace string, shouldExist bool) {
	if shouldExist {
		gomega.Eventually(func() error {
			dep, err := GetDeploymentWithCluster(cluster, name, namespace)
			if err != nil {
				return fmt.Errorf("failed to get deployment %s/%s: %w", namespace, name, err)
			}
			expectedReplicas := int32(1)
			if dep.Spec.Replicas != nil {
				expectedReplicas = *dep.Spec.Replicas
			}
			if dep.Status.ReadyReplicas != expectedReplicas {
				return FormatDeploymentNotReadyError(dep, namespace, name)
			}
			return nil
		}, 300, 5).Should(gomega.Not(gomega.HaveOccurred()))
	} else {
		gomega.Eventually(func() error {
			dep, err := GetDeploymentWithCluster(cluster, name, namespace)
			if apierrors.IsNotFound(err) {
				return nil
			}
			if err != nil {
				return fmt.Errorf("failed to get deployment %s/%s: %w", namespace, name, err)
			}
			return FormatDeploymentStillExistsError(dep, namespace, name)
		}, 120, 5).Should(gomega.Succeed())
	}
}

func CheckDeploymentAvailabilityOnClusters(clusters []Cluster, name, namespace string, shouldExist bool) {
	for _, cluster := range clusters {
		CheckDeploymentAvailability(cluster, name, namespace, shouldExist)
	}
}
