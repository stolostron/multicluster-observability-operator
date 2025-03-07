// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package status

import (
	"context"
	"fmt"
	"net"
	"reflect"
	"sort"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/pkg/util"
	oav1beta1 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
	"github.com/stolostron/multicluster-observability-operator/operators/pkg/status"
)

// conditionType represents the standard conditions expected by ACM in the ObservabilityAddon status.
type conditionType string

const (
	Available   conditionType = "Available"
	Progressing conditionType = "Progressing"
	Degraded    conditionType = "Degraded"
)

var (
	// componentsMap contains the types of conditions (from individual components) that must be aggregated into standard conditions.
	componentsMap = map[string]struct{}{
		string(status.MetricsCollector):    {},
		string(status.UwlMetricsCollector): {},
	}
)

// reason maps individual component reasons to standard types and assigns a priority to each reason.
// The priority is used to aggregate the conditions of the components into a single condition.
type reason struct {
	reason   string
	priority int
	stdType  conditionType
}

func newReason(s string) reason {
	switch s {
	case string(status.Disabled):
		return reason{string(status.Disabled), 1, Degraded}
	case string(status.ForwardSuccessful):
		return reason{string(status.ForwardSuccessful), 2, Available}
	case string(status.CmoReconcileLoopStopped):
		return reason{string(status.CmoReconcileLoopStopped), 3, Progressing}
	case string(status.UpdateSuccessful):
		return reason{string(status.UpdateSuccessful), 4, Progressing}
	case string(status.ForwardFailed):
		return reason{string(status.ForwardFailed), 5, Degraded}
	case string(status.CmoReconcileLoopDetected):
		return reason{string(status.CmoReconcileLoopDetected), 6, Degraded}
	case string(status.UpdateFailed):
		return reason{string(status.UpdateFailed), 7, Degraded}
	case string(status.NotSupported):
		return reason{string(status.NotSupported), 8, Degraded}
	default:
		return reason{s, -1, Degraded}
	}
}

func (r reason) String() string {
	return string(r.reason)
}

func (r reason) Priority() int {
	return r.priority
}

func (r reason) StdType() conditionType {
	return r.stdType
}

// StatusReconciler reconciles status object.
type StatusReconciler struct {
	Client       client.Client
	HubNamespace string
	Namespace    string
	HubClient    *util.ReloadableHubClient
	ObsAddonName string
	Logger       logr.Logger
}

// Reconcile reads the status' conditions of ObservabilityAddon, aggregates the individual component conditions
// into standard conditions, and updates the status in the local and hub clusters.
// It returns:
// - a TerminalError if the reconciliation fails and no requeue is needed
// - a non terminal error if the reconciliation fails and a requeue is needed
// - a result.RequeueAfter if the reconciliation fails and a requeue with delay is needed
func (r *StatusReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.Logger.WithValues("Request", req.String()).Info("Reconciling")

	if res, err := r.updateSpokeAddon(ctx); err != nil {
		return res, err
	} else if !res.IsZero() {
		return res, nil
	}

	if res, err := r.updateHubAddon(ctx); err != nil {
		return res, err
	} else if !res.IsZero() {
		return res, nil
	}

	return ctrl.Result{}, nil
}

func (s *StatusReconciler) updateSpokeAddon(ctx context.Context) (ctrl.Result, error) {
	retryErr := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		// Fetch the ObservabilityAddon instance in local cluster
		obsAddon := &oav1beta1.ObservabilityAddon{}
		if err := s.Client.Get(ctx, types.NamespacedName{Name: s.ObsAddonName, Namespace: s.Namespace}, obsAddon); err != nil {
			return err
		}

		addonNewCondition := aggregateComponentsConditions(obsAddon.Status.Conditions)
		if addonNewCondition == nil {
			return nil
		}

		if !shouldUpdateConditions(obsAddon.Status.Conditions, *addonNewCondition) {
			return nil
		}

		obsAddon.Status.Conditions = resetMainConditionsStatus(obsAddon.Status.Conditions)
		obsAddon.Status.Conditions = mutateOrAppend(obsAddon.Status.Conditions, *addonNewCondition)

		s.Logger.Info(fmt.Sprintf("Updating status of ObservabilityAddon %s/%s", obsAddon.Namespace, obsAddon.Name), "type", addonNewCondition.Type, "reason", addonNewCondition.Reason)

		return s.Client.Status().Update(ctx, obsAddon)
	})

	if retryErr != nil {
		if errors.IsConflict(retryErr) || util.IsTransientClientErr(retryErr) {
			return s.requeueWithOptionalDelay(fmt.Errorf("failed to update status in spoke cluster with retryable error: %w", retryErr))
		}
		return ctrl.Result{}, reconcile.TerminalError(retryErr)
	}

	return ctrl.Result{}, nil
}

func (s *StatusReconciler) updateHubAddon(ctx context.Context) (ctrl.Result, error) {
	// Fetch the ObservabilityAddon instance in hub cluster
	hubObsAddon := &oav1beta1.ObservabilityAddon{}
	err := s.HubClient.Get(ctx, types.NamespacedName{Name: s.ObsAddonName, Namespace: s.HubNamespace}, hubObsAddon)
	if err != nil {
		if isAuthOrConnectionErr(err) {
			// Try reloading the kubeconfig for the hub cluster
			var reloadErr error
			if s.HubClient, reloadErr = s.HubClient.Reload(); reloadErr != nil {
				return ctrl.Result{}, reconcile.TerminalError(fmt.Errorf("failed to reload the hub client: %w", reloadErr))
			}
			return ctrl.Result{}, fmt.Errorf("failed to get ObservabilityAddon in hub cluster, reloaded hub client: %w", err)
		}

		if util.IsTransientClientErr(err) {
			s.Logger.Info("Failed to get ObservabilityAddon in hub cluster, requeue with delay", "error", err)
			return s.requeueWithOptionalDelay(err)
		}

		return ctrl.Result{}, reconcile.TerminalError(fmt.Errorf("failed to get ObservabilityAddon in hub cluster: %w", err))
	}

	// Retry on conflict as operation happens in other cluster
	// on a shared resource that can be updated by multiple controllers.
	retryErr := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		// Fetch the ObservabilityAddon instance in local cluster
		obsAddon := &oav1beta1.ObservabilityAddon{}
		if err != s.Client.Get(ctx, types.NamespacedName{Name: s.ObsAddonName, Namespace: s.Namespace}, obsAddon) {
			return err
		}

		// Only update the status in hub cluster if needed
		if reflect.DeepEqual(hubObsAddon.Status, obsAddon.Status) {
			return nil
		}

		updatedAddon := hubObsAddon.DeepCopy()
		updatedAddon.Status = obsAddon.Status

		// Update the status in hub cluster
		return s.HubClient.Status().Update(ctx, updatedAddon)
	})
	if retryErr != nil {
		if util.IsTransientClientErr(retryErr) || errors.IsConflict(retryErr) {
			s.Logger.Info("Retryable error while updating status, request will be retried.", "error", retryErr)
			return s.requeueWithOptionalDelay(retryErr)
		}

		return ctrl.Result{}, reconcile.TerminalError(fmt.Errorf("failed to update status in hub cluster: %w", retryErr))
	}

	return ctrl.Result{}, nil
}

// requeueWithOptionalDelay requeues the request with a delay if suggested by the error
// Otherwise, it requeues the request without a delay by returning an error
// The runtime will requeue the request without a delay if the error is non-nil
func (r *StatusReconciler) requeueWithOptionalDelay(err error) (ctrl.Result, error) {
	if delay, ok := errors.SuggestsClientDelay(err); ok {
		r.Logger.Info("Requeue with delay", "error", err, "delay", delay)
		return ctrl.Result{RequeueAfter: time.Duration(delay) * time.Second}, nil
	}

	return ctrl.Result{}, err
}

// SetupWithManager sets up the controller with the Manager.
func (r *StatusReconciler) SetupWithManager(mgr ctrl.Manager) error {
	filterOutStandardConditions := func(c []oav1beta1.StatusCondition) []oav1beta1.StatusCondition {
		var filtered []oav1beta1.StatusCondition
		for _, condition := range c {
			if condition.Type == "Available" || condition.Type == "Progressing" || condition.Type == "Degraded" {
				continue
			}
			filtered = append(filtered, condition)
		}
		return filtered
	}
	pred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectNew.GetNamespace() != r.Namespace {
				return false
			}

			newConditions := filterOutStandardConditions(e.ObjectNew.(*oav1beta1.ObservabilityAddon).Status.Conditions)
			oldConditions := filterOutStandardConditions(e.ObjectOld.(*oav1beta1.ObservabilityAddon).Status.Conditions)
			return !reflect.DeepEqual(newConditions, oldConditions)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&oav1beta1.ObservabilityAddon{}, builder.WithPredicates(pred)).
		Complete(r)
}

// isAuthOrConnectionErr checks if the error is an authentication error or a connection error
// This suggests an issue with the client configuration and a reload might be needed
func isAuthOrConnectionErr(err error) bool {
	if errors.IsUnauthorized(err) || errors.IsForbidden(err) || errors.IsTimeout(err) {
		return true
	}

	if _, ok := err.(net.Error); ok {
		return true
	}

	return false
}

// mutateOrAppend updates the status conditions with the new condition.
// If the condition already exists, it updates it with the new condition.
// If the condition does not exist, it appends the new condition to the status conditions.
func mutateOrAppend(conditions []oav1beta1.StatusCondition, newCondition oav1beta1.StatusCondition) []oav1beta1.StatusCondition {
	if len(conditions) == 0 {
		return []oav1beta1.StatusCondition{newCondition}
	}

	for i, condition := range conditions {
		if condition.Type == newCondition.Type {
			// Update the existing condition
			conditions[i] = newCondition
			return conditions
		}
	}
	// If the condition type does not exist, append the new condition
	return append(conditions, newCondition)
}

// shouldAppendCondition checks if the new condition should be appended to the status conditions
// based on the last condition in the slice.
func shouldUpdateConditions(conditions []oav1beta1.StatusCondition, newCondition oav1beta1.StatusCondition) bool {
	filteredConditions := []oav1beta1.StatusCondition{}
	validTypes := map[string]struct{}{
		string(Available):   {},
		string(Progressing): {},
		string(Degraded):    {},
	}
	for _, condition := range conditions {
		if _, ok := validTypes[condition.Type]; ok {
			filteredConditions = append(filteredConditions, condition)
		}
	}

	if len(filteredConditions) == 0 {
		return true
	}

	sort.Slice(filteredConditions, func(i, j int) bool {
		if filteredConditions[i].Status == metav1.ConditionFalse && filteredConditions[j].Status == metav1.ConditionTrue {
			return true
		}
		return filteredConditions[i].LastTransitionTime.Before(&filteredConditions[j].LastTransitionTime)
	})

	lastCondition := filteredConditions[len(filteredConditions)-1]

	return lastCondition.Type != newCondition.Type ||
		lastCondition.Status != newCondition.Status ||
		lastCondition.Reason != newCondition.Reason ||
		lastCondition.Message != newCondition.Message
}

// aggregateComponentsConditions aggregates the conditions of the components into a single condition
// the condition type and reason are set based on the priority of the reasons of the components
// the m
func aggregateComponentsConditions(conditions []oav1beta1.StatusCondition) *oav1beta1.StatusCondition {
	// Filter out standard conditions
	filteredConditions := []oav1beta1.StatusCondition{}
	for _, condition := range conditions {
		if _, ok := componentsMap[condition.Type]; ok {
			filteredConditions = append(filteredConditions, condition)
		}
	}

	if len(filteredConditions) == 0 {
		return nil
	}

	// Sort the conditions by decreasing priority of the reason
	// If same priority, order by the type of the condition
	sort.Slice(filteredConditions, func(i, j int) bool {
		if newReason(filteredConditions[i].Reason).Priority() == newReason(filteredConditions[j].Reason).Priority() {
			return filteredConditions[i].Type < filteredConditions[j].Type
		}
		return newReason(filteredConditions[i].Reason).Priority() > newReason(filteredConditions[j].Reason).Priority()
	})

	// Aggregate the conditions based on the priority of the reason
	aggregatedCondition := &oav1beta1.StatusCondition{
		Status:             metav1.ConditionTrue,
		LastTransitionTime: metav1.Now(),
		Reason:             filteredConditions[0].Reason,
		Type:               string(newReason(filteredConditions[0].Reason).StdType()),
		Message:            fmt.Sprintf("%s: %s", filteredConditions[0].Type, filteredConditions[0].Message),
	}

	// Set some standard messages for the aggregated condition. It aligns with the registration-agent available message (see below)
	if aggregatedCondition.Type == string(Available) {
		// If the aggregated condition is Available, override the message with the same message as the registration-agent
		// It avoids confusion for the user. Because at some point, the registration-agent overrides the "Available" condition
		// with its own message.
		aggregatedCondition.Message = "observability-controller add-on is available."
	} else if aggregatedCondition.Type == string(Progressing) {
		aggregatedCondition.Message = "observability-controller add-on is progressing."
	} else if aggregatedCondition.Type == string(Degraded) && aggregatedCondition.Reason == string(status.Disabled) {
		aggregatedCondition.Message = "observability-controller add-on is disabled."
	}

	// truncate the message if it exceeds the limit
	limit := 256
	if len(aggregatedCondition.Message) > limit {
		aggregatedCondition.Message = aggregatedCondition.Message[:limit-3] + "..."
	}

	return aggregatedCondition
}

func resetMainConditionsStatus(conditions []oav1beta1.StatusCondition) []oav1beta1.StatusCondition {
	for i := range conditions {
		if conditions[i].Type == string(Available) || conditions[i].Type == string(Degraded) || conditions[i].Type == string(Progressing) {
			conditions[i].Status = metav1.ConditionFalse
		}
	}
	return conditions
}
