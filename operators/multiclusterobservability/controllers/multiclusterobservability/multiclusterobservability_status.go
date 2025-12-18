// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package multiclusterobservability

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mcoshared "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/shared"
	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
)

const (
	reasonMCOADegraded = "MultiClusterObservabilityAddonDegraded"
)

var (
	stopStatusUpdate              = make(chan struct{})
	stopCheckReady                = make(chan struct{})
	requeueStatusUpdate           = make(chan struct{})
	updateStatusIsRunnning        = false
	muUpdateStatusIsRunnning      sync.Mutex
	updateReadyStatusIsRunnning   = false
	muUpdateReadyStatusIsRunnning sync.Mutex
)

// Start goroutines to update MCO status
func StartStatusUpdate(c client.Client, instance *mcov1beta2.MultiClusterObservability) {
	muUpdateStatusIsRunnning.Lock()
	if updateStatusIsRunnning {
		muUpdateStatusIsRunnning.Unlock()
		return
	}
	updateStatusIsRunnning = true
	muUpdateStatusIsRunnning.Unlock()

	// init the stop ready check channel
	stopCheckReady = make(chan struct{})

	go func() {
		for {
			select {
			case <-stopStatusUpdate:
				muUpdateStatusIsRunnning.Lock()
				updateStatusIsRunnning = false
				muUpdateStatusIsRunnning.Unlock()
				close(stopCheckReady)
				log.V(1).Info("status update goroutine is stopped.")
				return
			case <-requeueStatusUpdate:
				log.V(1).Info("status update goroutine is triggered.")
				updateStatus(c)
				muUpdateReadyStatusIsRunnning.Lock()
				if updateReadyStatusIsRunnning && checkReadyStatus(c, instance) {
					log.V(1).Info("send singal to stop status check ready goroutine because MCO status is ready")
					stopCheckReady <- struct{}{}
				}
				muUpdateReadyStatusIsRunnning.Unlock()
			}
		}
	}()

	muUpdateReadyStatusIsRunnning.Lock()
	if updateReadyStatusIsRunnning {
		muUpdateReadyStatusIsRunnning.Unlock()
		return
	}
	updateReadyStatusIsRunnning = true
	muUpdateReadyStatusIsRunnning.Unlock()

	go func() {
		for {
			select {
			case <-stopCheckReady:
				muUpdateReadyStatusIsRunnning.Lock()
				updateReadyStatusIsRunnning = false
				muUpdateReadyStatusIsRunnning.Unlock()
				log.V(1).Info("check status ready goroutine is stopped.")
				return
			case <-time.After(2 * time.Second):
				log.V(1).Info("check status ready goroutine is triggered.")
				if checkReadyStatus(c, instance) {
					requeueStatusUpdate <- struct{}{}
				}
			}
		}
	}()
}

// updateStatus override UpdateStatus interface
func updateStatus(c client.Client) {
	instance := &mcov1beta2.MultiClusterObservability{}
	err := c.Get(context.TODO(), types.NamespacedName{
		Name: config.GetMonitoringCRName(),
	}, instance)
	if err != nil {
		log.Error(err, fmt.Sprintf("Failed to get existing mco %s", instance.Name))
		return
	}
	oldStatus := instance.Status
	newStatus := oldStatus.DeepCopy()
	updateInstallStatus(&newStatus.Conditions)
	updateReadyStatus(&newStatus.Conditions, c, instance)
	updateAddonSpecStatus(&newStatus.Conditions, instance)
	updateMCOAStatus(c, &newStatus.Conditions, instance)
	fillupStatus(&newStatus.Conditions)
	instance.Status.Conditions = newStatus.Conditions
	if !reflect.DeepEqual(newStatus.Conditions, oldStatus.Conditions) {
		err := c.Status().Update(context.TODO(), instance)
		if err != nil {
			log.Error(err, fmt.Sprintf("failed to update status of mco %s", instance.Name))
			return
		}
	}
}

// fillup the status if there is no status and lastTransitionTime in upgrade case
func fillupStatus(conditions *[]mcoshared.Condition) {
	for idx, condition := range *conditions {
		if condition.Status == "" {
			(*conditions)[idx].Status = metav1.ConditionUnknown
		}
		if condition.LastTransitionTime.IsZero() {
			(*conditions)[idx].LastTransitionTime = metav1.NewTime(time.Now())
		}
	}
}

func updateInstallStatus(conditions *[]mcoshared.Condition) {
	setStatusCondition(conditions, *newInstallingCondition())
}

func checkReadyStatus(c client.Client, mco *mcov1beta2.MultiClusterObservability) bool {
	if findStatusCondition(mco.Status.Conditions, "Ready") != nil {
		return true
	}

	objStorageStatus := checkObjStorageStatus(c, mco)
	if objStorageStatus != nil {
		return false
	}

	deployStatus := checkDeployStatus(c)
	if deployStatus != nil {
		return false
	}

	statefulStatus := checkStatefulSetStatus(c)
	return statefulStatus == nil
}

func updateReadyStatus(
	conditions *[]mcoshared.Condition,
	c client.Client,
	mco *mcov1beta2.MultiClusterObservability,
) {
	if findStatusCondition(*conditions, "Ready") != nil {
		return
	}

	objStorageStatus := checkObjStorageStatus(c, mco)
	if objStorageStatus != nil {
		setStatusCondition(conditions, *objStorageStatus)
		return
	}

	deployStatus := checkDeployStatus(c)
	if deployStatus != nil {
		setStatusCondition(conditions, *deployStatus)
		return
	}

	statefulStatus := checkStatefulSetStatus(c)
	if statefulStatus != nil {
		setStatusCondition(conditions, *statefulStatus)
		return
	}

	setStatusCondition(conditions, *newReadyCondition())
	removeStatusCondition(conditions, "Failed")
}

// setStatusCondition sets the corresponding condition in conditions to newCondition.
// conditions must be non-nil.
//  1. if the condition of the specified type already exists (all fields of the existing condition are updated to
//     newCondition, LastTransitionTime is set to now if the new status differs from the old status)
//  2. if a condition of the specified type does not exist (LastTransitionTime is set to now() if unset,
//     and newCondition is appended)
func setStatusCondition(conditions *[]mcoshared.Condition, newCondition mcoshared.Condition) {
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
func removeStatusCondition(conditions *[]mcoshared.Condition, conditionType string) {
	if conditions == nil || len(*conditions) == 0 {
		return
	}
	newConditions := make([]mcoshared.Condition, 0, len(*conditions)-1)
	for _, condition := range *conditions {
		if condition.Type != conditionType {
			newConditions = append(newConditions, condition)
		}
	}

	*conditions = newConditions
}

// findStatusCondition finds the conditionType in conditions.
func findStatusCondition(conditions []mcoshared.Condition, conditionType string) *mcoshared.Condition {
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return &conditions[i]
		}
	}

	return nil
}

func updateAddonSpecStatus(
	conditions *[]mcoshared.Condition,
	mco *mcov1beta2.MultiClusterObservability,
) {
	addonStatus := checkAddonSpecStatus(mco)
	if addonStatus != nil {
		setStatusCondition(conditions, *addonStatus)
	} else {
		removeStatusCondition(conditions, "MetricsDisabled")
	}
}

func updateMCOAStatus(c client.Client, conds *[]mcoshared.Condition, mco *mcov1beta2.MultiClusterObservability) {
	if mco.Spec.Capabilities == nil {
		removeStatusCondition(conds, reasonMCOADegraded)
		return
	}

	if mco.Spec.Capabilities.Platform == nil && mco.Spec.Capabilities.UserWorkloads == nil {
		removeStatusCondition(conds, reasonMCOADegraded)
		return
	}

	// Return early if MCOA components like Logs, Metrics collection and IncidentDetection are disabled
	// in the Platform configuration (NamespaceRightSizingRecommendation can be enabled/disabled).
	if mco.Spec.Capabilities.Platform != nil &&
		!mco.Spec.Capabilities.Platform.Logs.Collection.Enabled &&
		!mco.Spec.Capabilities.Platform.Metrics.Default.Enabled &&
		!mco.Spec.Capabilities.Platform.Analytics.IncidentDetection.Enabled {
		removeStatusCondition(conds, reasonMCOADegraded)
		return
	}

	var missing []string

outer:
	for _, crdName := range config.GetMCOASupportedCRDNames() {
		crd := &apiextensionsv1.CustomResourceDefinition{}
		key := client.ObjectKey{Name: crdName}

		err := c.Get(context.TODO(), key, crd)
		if client.IgnoreAlreadyExists(err) != nil {
			missing = append(missing, crdName)
			continue
		}

		version := config.GetMCOASupportedCRDVersion(crdName)

		for _, crdVersion := range crd.Spec.Versions {
			if crdVersion.Name == version && crdVersion.Served {
				continue outer
			}
		}

		missing = append(missing, crdName)
	}

	if len(missing) == 0 {
		removeStatusCondition(conds, reasonMCOADegraded)
		return
	}

	mcoaDegraded := newMCOADegradedCondition(missing)
	setStatusCondition(conds, *mcoaDegraded)
}

func getExpectedDeploymentNames() []string {
	return []string{
		config.GetOperandNamePrefix() + config.Grafana,
		config.GetOperandNamePrefix() + config.ObservatoriumAPI,
		config.GetOperandNamePrefix() + config.ThanosQuery,
		config.GetOperandNamePrefix() + config.ThanosQueryFrontend,
		config.GetOperandNamePrefix() + config.ThanosReceiveController,
		config.GetOperandNamePrefix() + config.ObservatoriumOperator,
		config.GetOperandNamePrefix() + config.RBACQueryProxy,
	}
}

func checkDeployStatus(c client.Client) *mcoshared.Condition {
	expectedDeploymentNames := getExpectedDeploymentNames()
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

		if found.Status.ReadyReplicas != found.Status.Replicas {
			msg := fmt.Sprintf("Deployment %s is not ready", name)
			return newFailedCondition("DeploymentNotReady", msg)
		}
	}

	return nil
}

func getExpectedStatefulSetNames() []string {
	return []string{
		config.GetOperandNamePrefix() + config.Alertmanager,
		config.GetOperandNamePrefix() + config.ThanosCompact,
		config.GetOperandNamePrefix() + config.ThanosReceive,
		config.GetOperandNamePrefix() + config.ThanosRule,
		config.GetOperandNamePrefix() + config.ThanosStoreMemcached,
		config.GetOperandNamePrefix() + config.ThanosStoreShard + "-0",
	}
}

func checkStatefulSetStatus(c client.Client) *mcoshared.Condition {
	expectedStatefulSetNames := getExpectedStatefulSetNames()
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

		if found.Status.ReadyReplicas != found.Status.Replicas {
			msg := fmt.Sprintf("StatefulSet %s is not ready", name)
			return newFailedCondition("StatefulSetNotReady", msg)
		}
	}

	return nil
}

func checkObjStorageStatus(
	c client.Client,
	mco *mcov1beta2.MultiClusterObservability,
) *mcoshared.Condition {
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

func checkAddonSpecStatus(mco *mcov1beta2.MultiClusterObservability) *mcoshared.Condition {
	addonSpec := mco.Spec.ObservabilityAddonSpec
	if addonSpec != nil && !addonSpec.EnableMetrics {
		log.Info("Disable metrics collector")
		return newMetricsDisabledCondition()
	}
	return nil
}

func newInstallingCondition() *mcoshared.Condition {
	return &mcoshared.Condition{
		Type:    "Installing",
		Status:  metav1.ConditionTrue,
		Reason:  "Installing",
		Message: "Installation is in progress",
	}
}

func newReadyCondition() *mcoshared.Condition {
	return &mcoshared.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionTrue,
		Reason:  "Ready",
		Message: "Observability components are deployed and running",
	}
}

func newFailedCondition(reason string, msg string) *mcoshared.Condition {
	return &mcoshared.Condition{
		Type:    "Failed",
		Status:  "False",
		Reason:  reason,
		Message: msg,
	}
}

func newMetricsDisabledCondition() *mcoshared.Condition {
	return &mcoshared.Condition{
		Type:    "MetricsDisabled",
		Status:  metav1.ConditionTrue,
		Reason:  "MetricsDisabled",
		Message: "Collect metrics from the managed clusters is disabled",
	}
}

func newMCOADegradedCondition(missing []string) *mcoshared.Condition {
	tmpl := "MultiCluster-Observability-Addon degraded because the following CRDs are not installed on the hub: %s"

	var missingVersions []string
	for _, name := range missing {
		version := config.GetMCOASupportedCRDVersion(name)
		missingVersions = append(missingVersions, fmt.Sprintf("%s(%s)", name, version))
	}

	msg := fmt.Sprintf(tmpl, strings.Join(missingVersions, ", "))

	return &mcoshared.Condition{
		Type:    reasonMCOADegraded,
		Status:  metav1.ConditionTrue,
		Reason:  reasonMCOADegraded,
		Message: msg,
	}
}
