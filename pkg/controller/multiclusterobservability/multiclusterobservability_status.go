// Copyright (c) 2021 Red Hat, Inc.

package multiclusterobservability

import (
	"context"
	"fmt"

	mcov1beta1 "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/apis/observability/v1beta1"
	"github.com/open-cluster-management/multicluster-monitoring-operator/pkg/config"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func updateInstallStatus(conditions *[]metav1.Condition) {
	meta.SetStatusCondition(conditions, *newInstallingCondition())
}

func updateReadyStatus(
	conditions *[]metav1.Condition,
	c client.Client,
	mco *mcov1beta1.MultiClusterObservability) {

	if meta.FindStatusCondition(*conditions, "Ready") != nil {
		return
	}

	objStorageStatus := checkObjStorageStatus(c, mco)
	if objStorageStatus != nil {
		meta.SetStatusCondition(conditions, *objStorageStatus)
		return
	}

	deployStatus := checkDeployStatus(c, mco)
	if deployStatus != nil {
		meta.SetStatusCondition(conditions, *deployStatus)
		return
	}

	statefulStatus := checkStatefulSetStatus(c, mco)
	if statefulStatus != nil {
		meta.SetStatusCondition(conditions, *statefulStatus)
		return
	}

	meta.SetStatusCondition(conditions, *newReadyCondition())
	meta.RemoveStatusCondition(conditions, "Failed")
}

func updateAddonSpecStatus(
	conditions *[]metav1.Condition,
	mco *mcov1beta1.MultiClusterObservability) {
	addonStatus := checkAddonSpecStatus(mco)
	if addonStatus != nil {
		meta.SetStatusCondition(conditions, *addonStatus)
	} else {
		meta.RemoveStatusCondition(conditions, "MetricsDisabled")
	}
}

func getExpectedDeploymentNames(mcoCRName string) []string {
	return []string{
		"grafana",
		mcoCRName + "-observatorium-observatorium-api",
		mcoCRName + "-observatorium-thanos-query",
		mcoCRName + "-observatorium-thanos-query-frontend",
		mcoCRName + "-observatorium-thanos-receive-controller",
		"observatorium-operator",
		"rbac-query-proxy",
	}
}

func checkDeployStatus(
	c client.Client,
	mco *mcov1beta1.MultiClusterObservability) *metav1.Condition {
	mcoCRName := config.GetMonitoringCRName()
	expectedDeploymentNames := getExpectedDeploymentNames(mcoCRName)
	for _, name := range expectedDeploymentNames {
		found := &appsv1.Deployment{}
		namespacedName := types.NamespacedName{
			Name:      name,
			Namespace: config.GetDefaultNamespace(),
		}
		err := c.Get(context.TODO(), namespacedName, found)
		if err != nil {
			msg := fmt.Sprintf("Failed to found expected deployment %s", name)
			return newFailedCondition("DeploymentNotFound", msg)
		}

		if found.Status.ReadyReplicas < 1 {
			msg := fmt.Sprintf("Deployment %s is not ready", name)
			return newFailedCondition("DeploymentNotReady", msg)
		}
	}

	return nil
}

func getExpectedStatefulSetNames(mcoCRName string) []string {
	return []string{
		"alertmanager",
		mcoCRName + "-observatorium-thanos-compact",
		mcoCRName + "-observatorium-thanos-receive-default",
		mcoCRName + "-observatorium-thanos-rule",
		mcoCRName + "-observatorium-thanos-store-memcached",
		mcoCRName + "-observatorium-thanos-store-shard-0",
	}
}

func checkStatefulSetStatus(
	c client.Client,
	mco *mcov1beta1.MultiClusterObservability) *metav1.Condition {
	expectedStatefulSetNames := getExpectedStatefulSetNames(config.GetMonitoringCRName())
	for _, name := range expectedStatefulSetNames {
		found := &appsv1.StatefulSet{}
		namespacedName := types.NamespacedName{
			Name:      name,
			Namespace: config.GetDefaultNamespace(),
		}
		err := c.Get(context.TODO(), namespacedName, found)
		if err != nil {
			msg := fmt.Sprintf("Failed to found expected stateful set %s", name)
			return newFailedCondition("StatefulSetNotFound", msg)
		}

		if found.Status.ReadyReplicas < 1 {
			msg := fmt.Sprintf("StatefulSet %s is not ready", name)
			return newFailedCondition("StatefulSetNotReady", msg)
		}
	}

	return nil
}

func checkObjStorageStatus(
	c client.Client,
	mco *mcov1beta1.MultiClusterObservability) *metav1.Condition {
	objStorageConf := mco.Spec.StorageConfig.MetricObjectStorage
	secret := &corev1.Secret{}
	namespacedName := types.NamespacedName{
		Name:      objStorageConf.Name,
		Namespace: config.GetDefaultNamespace(),
	}

	err := c.Get(context.TODO(), namespacedName, secret)
	if err != nil {
		return newFailedCondition("ObjectStorageSecretNotFound", err.Error())
	}

	data, ok := secret.Data[objStorageConf.Key]
	if !ok {
		msg := fmt.Sprintf("Failed to found the object storage configuration key from secret %s", secret.Name)
		return newFailedCondition("ObjectStorageConfInvalid", msg)
	}

	ok, err = config.CheckObjStorageConf(data)
	if !ok {
		return newFailedCondition("ObjectStorageConfInvalid", err.Error())
	}

	return nil
}

func checkAddonSpecStatus(mco *mcov1beta1.MultiClusterObservability) *metav1.Condition {
	addonSpec := mco.Spec.ObservabilityAddonSpec
	if addonSpec != nil && addonSpec.EnableMetrics == false {
		log.Info("Disable metrics collocter")
		return newMetricsDisabledCondition()
	}
	return nil
}

func newInstallingCondition() *metav1.Condition {
	return &metav1.Condition{
		Type:    "Installing",
		Status:  "True",
		Reason:  "Installing",
		Message: "Installation is in progress",
	}
}

func newReadyCondition() *metav1.Condition {
	return &metav1.Condition{
		Type:    "Ready",
		Status:  "True",
		Reason:  "Ready",
		Message: "Observability components are deployed and running",
	}
}

func newFailedCondition(reason string, msg string) *metav1.Condition {
	return &metav1.Condition{
		Type:    "Failed",
		Status:  "False",
		Reason:  reason,
		Message: msg,
	}
}

func newMetricsDisabledCondition() *metav1.Condition {
	return &metav1.Condition{
		Type:    "MetricsDisabled",
		Status:  "True",
		Reason:  "MetricsDisabled",
		Message: "Collect metrics from the managed clusters is disabled",
	}
}
