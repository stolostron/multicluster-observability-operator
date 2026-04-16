// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package status

import (
	"cmp"
	"context"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/go-logr/logr"
	mcoshared "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/shared"
	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/util"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	ConditionTypeReady           = "Ready"
	ConditionTypeFailed          = "Failed"
	ConditionTypeInstalling      = "Installing"
	ConditionTypeMetricsDisabled = "MetricsDisabled"
	ConditionTypeMCOADegraded    = "MultiClusterObservabilityAddonDegraded"

	ReasonDeploymentNotFound    = "DeploymentNotFound"
	ReasonDeploymentNotReady    = "DeploymentNotReady"
	ReasonStatefulSetNotFound   = "StatefulSetNotFound"
	ReasonStatefulSetNotReady   = "StatefulSetNotReady"
	ReasonObjectStorageNotFound = "ObjectStorageSecretNotFound"
	ReasonObjectStorageInvalid  = "ObjectStorageConfInvalid"
	ReasonCRDMissing            = "CRDMissing"
)

// StatusReconciler reconciles the status of a MultiClusterObservability object
type StatusReconciler struct {
	client.Client
	Log logr.Logger
}

// SetupWithManager sets up the controller with the Manager.
func (r *StatusReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Pre-compute static lists for efficiency in predicates
	expectedDeploys := config.GetExpectedDeploymentNames()
	expectedSts := config.GetExpectedStatefulSetNames()

	supportedCRDs := config.GetMCOASupportedCRDNames()
	defaultNS := config.GetDefaultNamespace()

	// Map child resource events back to the single MCO instance
	mapToMCO := func(ctx context.Context, obj client.Object) []reconcile.Request {
		instance, err := config.GetMCOInstance(ctx, r.Client)
		if err != nil || instance == nil {
			return nil
		}

		// Aggressively filter Secrets: only care about the one defined in MCO Spec
		if secret, ok := obj.(*corev1.Secret); ok {
			if instance.Spec.StorageConfig == nil ||
				instance.Spec.StorageConfig.MetricObjectStorage == nil ||
				instance.Spec.StorageConfig.MetricObjectStorage.Name != secret.GetName() {
				return nil
			}
		}

		r.Log.V(2).Info("Triggering status reconcile",
			"triggerType", fmt.Sprintf("%T", obj),
			"triggerName", obj.GetName(),
			"triggerNamespace", obj.GetNamespace())

		return []reconcile.Request{
			{NamespacedName: types.NamespacedName{
				Name: instance.GetName(),
			}},
		}
	}

	workloadPredicate := predicate.NewPredicateFuncs(func(object client.Object) bool {
		if object.GetNamespace() != defaultNS {
			return false
		}
		return slices.Contains(expectedDeploys, object.GetName()) ||
			slices.Contains(expectedSts, object.GetName())
	})

	nsPredicate := predicate.NewPredicateFuncs(func(object client.Object) bool {
		return object.GetNamespace() == defaultNS
	})

	crdPredicate := predicate.NewPredicateFuncs(func(object client.Object) bool {
		return slices.Contains(supportedCRDs, object.GetName())
	})

	return ctrl.NewControllerManagedBy(mgr).
		Named("multiclusterobservability-status").
		For(&mcov1beta2.MultiClusterObservability{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Watches(&appsv1.Deployment{}, handler.EnqueueRequestsFromMapFunc(mapToMCO), builder.WithPredicates(workloadPredicate, predicate.ResourceVersionChangedPredicate{})).
		Watches(&appsv1.StatefulSet{}, handler.EnqueueRequestsFromMapFunc(mapToMCO), builder.WithPredicates(workloadPredicate, predicate.ResourceVersionChangedPredicate{})).
		Watches(&corev1.Secret{}, handler.EnqueueRequestsFromMapFunc(mapToMCO), builder.WithPredicates(nsPredicate, predicate.ResourceVersionChangedPredicate{})).
		Watches(&apiextensionsv1.CustomResourceDefinition{}, handler.EnqueueRequestsFromMapFunc(mapToMCO), builder.WithPredicates(crdPredicate, predicate.ResourceVersionChangedPredicate{})).
		Complete(r)
}

// Reconcile evaluates the current state of operands and updates the MCO status subresource.
func (r *StatusReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	reqLogger := r.Log.WithValues("Request.Namespace", req.Namespace, "Request.Name", req.Name)

	instance := &mcov1beta2.MultiClusterObservability{}
	if err := r.Get(ctx, req.NamespacedName, instance); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Do not reconcile objects if this instance of mco is labeled "paused"
	if config.IsPaused(instance.GetAnnotations()) {
		reqLogger.Info("MCO status reconciliation is paused. Nothing more to do.")
		return ctrl.Result{}, nil
	}

	oldStatus := instance.Status.DeepCopy()
	newStatus := instance.Status.DeepCopy()

	r.updateStatus(ctx, instance, &newStatus.Conditions)

	sortConditions(oldStatus.Conditions)
	sortConditions(newStatus.Conditions)

	if !reflect.DeepEqual(newStatus.Conditions, oldStatus.Conditions) {
		changedConditions := getConditionChanges(oldStatus.Conditions, newStatus.Conditions)
		reqLogger.Info("Updating MCO status conditions", "changes", changedConditions)
		instance.Status.Conditions = newStatus.Conditions
		err := r.Status().Update(ctx, instance)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update MCO status: %w", err)
		}
	}

	return ctrl.Result{}, nil
}

func getConditionChanges(oldConds, newConds []mcoshared.Condition) []string {
	var changes []string

	oldMap := make(map[string]mcoshared.Condition, len(oldConds))
	for _, c := range oldConds {
		oldMap[c.Type] = c
	}

	newMap := make(map[string]mcoshared.Condition, len(newConds))
	for _, c := range newConds {
		newMap[c.Type] = c
	}

	for _, newCond := range newConds {
		oldCond, exists := oldMap[newCond.Type]
		if !exists {
			changes = append(changes, fmt.Sprintf("Added: %s (Status: %s, Reason: %s)", newCond.Type, newCond.Status, newCond.Reason))
		} else if oldCond.Status != newCond.Status || oldCond.Reason != newCond.Reason || oldCond.Message != newCond.Message {
			msgIndicator := ""
			if oldCond.Message != newCond.Message && oldCond.Status == newCond.Status && oldCond.Reason == newCond.Reason {
				msgIndicator = " [Message updated]"
			}
			changes = append(changes, fmt.Sprintf("Modified: %s (Status: %s->%s, Reason: %s->%s%s)", newCond.Type, oldCond.Status, newCond.Status, oldCond.Reason, newCond.Reason, msgIndicator))
		}
	}

	for _, oldCond := range oldConds {
		if _, exists := newMap[oldCond.Type]; !exists {
			changes = append(changes, fmt.Sprintf("Removed: %s", oldCond.Type))
		}
	}

	return changes
}

func (r *StatusReconciler) updateStatus(ctx context.Context, instance *mcov1beta2.MultiClusterObservability, conditions *[]mcoshared.Condition) {
	r.updateInstallStatus(conditions)
	r.updateReadyStatus(ctx, instance, conditions)
	r.updateAddonSpecStatus(instance, conditions)
	r.updateMCOAStatus(ctx, instance, conditions)
	fillupStatus(conditions)
}

func (r *StatusReconciler) updateReadyStatus(
	ctx context.Context,
	mco *mcov1beta2.MultiClusterObservability,
	conditions *[]mcoshared.Condition,
) {
	if objStorageStatus := checkObjStorageStatus(ctx, r.Client, mco); objStorageStatus != nil {
		RemoveStatusCondition(conditions, ConditionTypeReady)
		setStatusCondition(conditions, *objStorageStatus)
		return
	}

	if deployStatus := checkDeployStatus(ctx, r.Client); deployStatus != nil {
		RemoveStatusCondition(conditions, ConditionTypeReady)
		setStatusCondition(conditions, *deployStatus)
		return
	}

	if statefulStatus := checkStatefulSetStatus(ctx, r.Client); statefulStatus != nil {
		RemoveStatusCondition(conditions, ConditionTypeReady)
		setStatusCondition(conditions, *statefulStatus)
		return
	}

	setStatusCondition(conditions, *newReadyCondition())
	RemoveStatusCondition(conditions, ConditionTypeFailed)
	RemoveStatusCondition(conditions, ConditionTypeInstalling)
}

func (r *StatusReconciler) updateInstallStatus(conditions *[]mcoshared.Condition) {
	if FindStatusCondition(*conditions, ConditionTypeReady) == nil {
		setStatusCondition(conditions, *newInstallingCondition())
	}
}

func (r *StatusReconciler) updateAddonSpecStatus(
	mco *mcov1beta2.MultiClusterObservability,
	conditions *[]mcoshared.Condition,
) {
	addonSpec := mco.Spec.ObservabilityAddonSpec
	if addonSpec != nil && !addonSpec.EnableMetrics {
		r.Log.Info("Disable metrics collector")
		setStatusCondition(conditions, *newMetricsDisabledCondition())
	} else {
		RemoveStatusCondition(conditions, ConditionTypeMetricsDisabled)
	}
}

func (r *StatusReconciler) updateMCOAStatus(ctx context.Context, mco *mcov1beta2.MultiClusterObservability, conds *[]mcoshared.Condition) {
	if !util.IsMCOAEnabled(mco) {
		RemoveStatusCondition(conds, ConditionTypeMCOADegraded)
		return
	}

	var missing []string

outer:
	for _, crdName := range config.GetMCOASupportedCRDNames() {
		crd := &apiextensionsv1.CustomResourceDefinition{}
		key := client.ObjectKey{Name: crdName}

		err := r.Get(ctx, key, crd)
		if err != nil {
			if apierrors.IsNotFound(err) {
				missing = append(missing, crdName)
				continue
			}
			r.Log.Error(err, "failed to check CRD existence, skipping", "crd", crdName)
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
		RemoveStatusCondition(conds, ConditionTypeMCOADegraded)
		return
	}

	mcoaDegraded := newMCOADegradedCondition(missing)
	setStatusCondition(conds, *mcoaDegraded)
}

// --- Helper Functions ---

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

func sortConditions(conditions []mcoshared.Condition) {
	slices.SortFunc(conditions, func(a, b mcoshared.Condition) int {
		return cmp.Compare(a.Type, b.Type)
	})
}

func setStatusCondition(conditions *[]mcoshared.Condition, newCondition mcoshared.Condition) {
	if conditions == nil {
		return
	}
	existingCondition := FindStatusCondition(*conditions, newCondition.Type)
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

func RemoveStatusCondition(conditions *[]mcoshared.Condition, conditionType string) {
	if conditions == nil {
		return
	}
	*conditions = slices.DeleteFunc(*conditions, func(c mcoshared.Condition) bool {
		return c.Type == conditionType
	})
}

func FindStatusCondition(conditions []mcoshared.Condition, conditionType string) *mcoshared.Condition {
	idx := slices.IndexFunc(conditions, func(c mcoshared.Condition) bool {
		return c.Type == conditionType
	})
	if idx == -1 {
		return nil
	}
	return &conditions[idx]
}

func checkDeployStatus(ctx context.Context, c client.Client) *mcoshared.Condition {
	expectedDeploymentNames := config.GetExpectedDeploymentNames()
	for _, name := range expectedDeploymentNames {
		found := &appsv1.Deployment{}
		namespacedName := types.NamespacedName{
			Name:      name,
			Namespace: config.GetDefaultNamespace(),
		}
		err := c.Get(ctx, namespacedName, found)
		if err != nil {
			msg := fmt.Sprintf("deployment %s not found", name)
			return newFailedCondition(ReasonDeploymentNotFound, msg)
		}

		if found.Status.ReadyReplicas != found.Status.Replicas {
			msg := fmt.Sprintf("deployment %s is not ready", name)
			return newFailedCondition(ReasonDeploymentNotReady, msg)
		}
	}

	return nil
}

func checkStatefulSetStatus(ctx context.Context, c client.Client) *mcoshared.Condition {
	expectedStatefulSetNames := config.GetExpectedStatefulSetNames()
	for _, name := range expectedStatefulSetNames {
		found := &appsv1.StatefulSet{}
		namespacedName := types.NamespacedName{
			Name:      name,
			Namespace: config.GetDefaultNamespace(),
		}
		err := c.Get(ctx, namespacedName, found)
		if err != nil {
			msg := fmt.Sprintf("statefulset %s not found", name)
			return newFailedCondition(ReasonStatefulSetNotFound, msg)
		}

		if found.Status.ReadyReplicas != found.Status.Replicas {
			msg := fmt.Sprintf("statefulset %s is not ready", name)
			return newFailedCondition(ReasonStatefulSetNotReady, msg)
		}
	}

	return nil
}

func checkObjStorageStatus(
	ctx context.Context,
	c client.Client,
	mco *mcov1beta2.MultiClusterObservability,
) *mcoshared.Condition {
	objStorageConf := mco.Spec.StorageConfig.MetricObjectStorage
	secret := &corev1.Secret{}
	namespacedName := types.NamespacedName{
		Name:      objStorageConf.Name,
		Namespace: config.GetDefaultNamespace(),
	}

	err := c.Get(ctx, namespacedName, secret)
	if err != nil {
		msg := fmt.Sprintf("object storage secret %s not found", objStorageConf.Name)
		return newFailedCondition(ReasonObjectStorageNotFound, msg)
	}

	data, ok := secret.Data[objStorageConf.Key]
	if !ok {
		msg := fmt.Sprintf("object storage configuration key not found in secret %s", secret.Name)
		return newFailedCondition(ReasonObjectStorageInvalid, msg)
	}

	ok, err = config.CheckObjStorageConf(data)
	if !ok {
		msg := "object storage configuration is invalid"
		if err != nil {
			msg = fmt.Sprintf("object storage configuration is invalid: %v", err)
		}
		return newFailedCondition(ReasonObjectStorageInvalid, msg)
	}

	return nil
}

func newInstallingCondition() *mcoshared.Condition {
	return &mcoshared.Condition{
		Type:    ConditionTypeInstalling,
		Status:  metav1.ConditionTrue,
		Reason:  ConditionTypeInstalling,
		Message: "Installation is in progress",
	}
}

func newReadyCondition() *mcoshared.Condition {
	return &mcoshared.Condition{
		Type:    ConditionTypeReady,
		Status:  metav1.ConditionTrue,
		Reason:  ConditionTypeReady,
		Message: "Observability components are deployed and running",
	}
}

func newFailedCondition(reason string, msg string) *mcoshared.Condition {
	return &mcoshared.Condition{
		Type:    ConditionTypeFailed,
		Status:  metav1.ConditionTrue,
		Reason:  reason,
		Message: msg,
	}
}

func newMetricsDisabledCondition() *mcoshared.Condition {
	return &mcoshared.Condition{
		Type:    ConditionTypeMetricsDisabled,
		Status:  metav1.ConditionTrue,
		Reason:  ConditionTypeMetricsDisabled,
		Message: "Collect metrics from the managed clusters is disabled",
	}
}

func newMCOADegradedCondition(missing []string) *mcoshared.Condition {
	tmpl := "MultiCluster-Observability-Addon degraded because the following CRDs are not installed on the hub: %s"

	missingVersions := make([]string, 0, len(missing))
	for _, name := range missing {
		version := config.GetMCOASupportedCRDVersion(name)
		missingVersions = append(missingVersions, fmt.Sprintf("%s(%s)", name, version))
	}

	msg := fmt.Sprintf(tmpl, strings.Join(missingVersions, ", "))

	return &mcoshared.Condition{
		Type:    ConditionTypeMCOADegraded,
		Status:  metav1.ConditionTrue,
		Reason:  ReasonCRDMissing,
		Message: msg,
	}
}
