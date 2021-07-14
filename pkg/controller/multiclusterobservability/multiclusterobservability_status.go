// Copyright (c) 2021 Red Hat, Inc.

package multiclusterobservability

import (
	"context"
	"fmt"
	"time"

	mcov1beta1 "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/apis/observability/v1beta1"
	"github.com/open-cluster-management/multicluster-monitoring-operator/pkg/config"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// fillup the status if there is no status and lastTransitionTime in upgrade case
func fillupStatus(conditions *[]mcov1beta1.Condition) {
	for idx, condition := range *conditions {
		if condition.Status == "" {
			(*conditions)[idx].Status = metav1.ConditionUnknown
		}
		if condition.LastTransitionTime.IsZero() {
			(*conditions)[idx].LastTransitionTime = metav1.NewTime(time.Now())
		}
	}
}

func updateInstallStatus(conditions *[]mcov1beta1.Condition) {
	setStatusCondition(conditions, *newInstallingCondition())
}

func updateReadyStatus(
	conditions *[]mcov1beta1.Condition,
	c client.Client,
	mco *mcov1beta1.MultiClusterObservability) {

	if findStatusCondition(*conditions, "Ready") != nil {
		return
	}

	objStorageStatus := checkObjStorageStatus(c, mco)
	if objStorageStatus != nil {
		setStatusCondition(conditions, *objStorageStatus)
		return
	}

	deployStatus := checkDeployStatus(c, mco)
	if deployStatus != nil {
		setStatusCondition(conditions, *deployStatus)
		return
	}

	statefulStatus := checkStatefulSetStatus(c, mco)
	if statefulStatus != nil {
		setStatusCondition(conditions, *statefulStatus)
		return
	}

	setStatusCondition(conditions, *newReadyCondition())
	removeStatusCondition(conditions, "Failed")
}

// setStatusCondition sets the corresponding condition in conditions to newCondition.
// conditions must be non-nil.
// 1. if the condition of the specified type already exists (all fields of the existing condition are updated to
//    newCondition, LastTransitionTime is set to now if the new status differs from the old status)
// 2. if a condition of the specified type does not exist (LastTransitionTime is set to now() if unset, and newCondition is appended)
func setStatusCondition(conditions *[]mcov1beta1.Condition, newCondition mcov1beta1.Condition) {
	if conditions == nil {
		return
	}
	existingCondition := findStatusCondition(*conditions, newCondition.Type)
	if existingCondition == nil {
		if newCondition.LastTransitionTime.IsZero() {
			newCondition.LastTransitionTime = metav1.NewTime(time.Now())
		}
		*conditions = append(*conditions, newCondition)
		return
	}

	if existingCondition.Status != newCondition.Status {
		existingCondition.Status = newCondition.Status
		if !newCondition.LastTransitionTime.IsZero() {
			existingCondition.LastTransitionTime = newCondition.LastTransitionTime
		} else {
			existingCondition.LastTransitionTime = metav1.NewTime(time.Now())
		}
	}

	existingCondition.Reason = newCondition.Reason
	existingCondition.Message = newCondition.Message
}

// removeStatusCondition removes the corresponding conditionType from conditions.
// conditions must be non-nil.
func removeStatusCondition(conditions *[]mcov1beta1.Condition, conditionType string) {
	if conditions == nil {
		return
	}
	newConditions := make([]mcov1beta1.Condition, 0, len(*conditions)-1)
	for _, condition := range *conditions {
		if condition.Type != conditionType {
			newConditions = append(newConditions, condition)
		}
	}

	*conditions = newConditions
}

// findStatusCondition finds the conditionType in conditions.
func findStatusCondition(conditions []mcov1beta1.Condition, conditionType string) *mcov1beta1.Condition {
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return &conditions[i]
		}
	}

	return nil
}

func updateAddonSpecStatus(
	conditions *[]mcov1beta1.Condition,
	mco *mcov1beta1.MultiClusterObservability) {
	addonStatus := checkAddonSpecStatus(mco)
	if addonStatus != nil {
		setStatusCondition(conditions, *addonStatus)
	} else {
		removeStatusCondition(conditions, "MetricsDisabled")
	}
}

func getExpectedDeploymentNames(mcoCRName string) []string {
	return []string{
		config.Grafana,
		config.GetOperandNamePrefix() + "observatorium-observatorium-api",
		config.GetOperandNamePrefix() + "observatorium-thanos-query",
		config.GetOperandNamePrefix() + "observatorium-thanos-query-frontend",
		config.GetOperandNamePrefix() + "observatorium-thanos-receive-controller",
		config.ObservatoriumOperator,
		config.RBACQueryProxy,
	}
}

func checkDeployStatus(
	c client.Client,
	mco *mcov1beta1.MultiClusterObservability) *mcov1beta1.Condition {
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
		config.Alertmanager,
		config.GetOperandNamePrefix() + "observatorium-thanos-compact",
		config.GetOperandNamePrefix() + "observatorium-thanos-receive-default",
		config.GetOperandNamePrefix() + "observatorium-thanos-rule",
		config.GetOperandNamePrefix() + "observatorium-thanos-store-memcached",
		config.GetOperandNamePrefix() + "observatorium-thanos-store-shard-0",
	}
}

func checkStatefulSetStatus(
	c client.Client,
	mco *mcov1beta1.MultiClusterObservability) *mcov1beta1.Condition {
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
	mco *mcov1beta1.MultiClusterObservability) *mcov1beta1.Condition {
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

func checkAddonSpecStatus(mco *mcov1beta1.MultiClusterObservability) *mcov1beta1.Condition {
	addonSpec := mco.Spec.ObservabilityAddonSpec
	if addonSpec != nil && addonSpec.EnableMetrics == false {
		log.Info("Disable metrics collocter")
		return newMetricsDisabledCondition()
	}
	return nil
}

func newInstallingCondition() *mcov1beta1.Condition {
	return &mcov1beta1.Condition{
		Type:    "Installing",
		Status:  "True",
		Reason:  "Installing",
		Message: "Installation is in progress",
	}
}

func newReadyCondition() *mcov1beta1.Condition {
	return &mcov1beta1.Condition{
		Type:    "Ready",
		Status:  "True",
		Reason:  "Ready",
		Message: "Observability components are deployed and running",
	}
}

func newFailedCondition(reason string, msg string) *mcov1beta1.Condition {
	return &mcov1beta1.Condition{
		Type:    "Failed",
		Status:  "False",
		Reason:  reason,
		Message: msg,
	}
}

func newMetricsDisabledCondition() *mcov1beta1.Condition {
	return &mcov1beta1.Condition{
		Type:    "MetricsDisabled",
		Status:  "True",
		Reason:  "MetricsDisabled",
		Message: "Collect metrics from the managed clusters is disabled",
	}
}
