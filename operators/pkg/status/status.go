// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package status

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	oav1beta1 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Component defines the components of the ObservabilityAddon
// each reports its own status condition
type Component string

const (
	MetricsCollector    Component = "MetricsCollector"
	UwlMetricsCollector Component = "UwlMetricsCollector"
)

// Reason defines the Reason for the status condition
type Reason string

var (
	// When adding a new Reason, make sure to update the status controller package
	// to aggreagate correctly the status of the ObservabilityAddon
	UpdateSuccessful  Reason = "UpdateSuccessful"
	UpdateFailed      Reason = "UpdateFailed"
	ForwardSuccessful Reason = "ForwardSuccessful"
	ForwardFailed     Reason = "ForwardFailed"
	Disabled          Reason = "Disabled"
	NotSupported      Reason = "NotSupported"
)

var (
	// componentTransitions defines the valid transitions between component conditions
	componentTransitions = map[Reason]map[Reason]struct{}{
		UpdateSuccessful: {
			UpdateFailed:      {},
			ForwardSuccessful: {},
			ForwardFailed:     {},
			Disabled:          {},
			NotSupported:      {},
		},
		UpdateFailed: {
			UpdateSuccessful: {},
			Disabled:         {},
			NotSupported:     {},
		},
		ForwardSuccessful: {
			ForwardFailed:    {},
			UpdateSuccessful: {},
			UpdateFailed:     {},
			Disabled:         {},
			NotSupported:     {},
		},
		ForwardFailed: {
			ForwardSuccessful: {},
			UpdateSuccessful:  {},
			UpdateFailed:      {},
			Disabled:          {},
			NotSupported:      {},
		},
		Disabled: {
			UpdateSuccessful: {},
			UpdateFailed:     {},
			NotSupported:     {},
		},
		NotSupported: {
			UpdateSuccessful: {},
			UpdateFailed:     {},
			Disabled:         {},
		},
	}
)

// Status provides a method to update the status of the ObservabilityAddon for a specific component
type Status struct {
	client    client.Client
	addonName string
	addonNs   string
	logger    logr.Logger
}

// NewStatus creates a new Status instance
func NewStatus(client client.Client, addonName, addonNs string, logger logr.Logger) Status {
	return Status{
		client:    client,
		addonName: addonName,
		addonNs:   addonNs,
		logger:    logger,
	}
}

// UpdateComponentCondition updates the status condition of a specific component of the ObservabilityAddon
// It returns an error if the update fails for a permanent reason or after exhausting retries on conflict.
// It will also return an error if the transition between conditions is invalid, to avoid flapping.
// Finally it return a boolean indicating if the condition was updated or not. This is useful to avoid
// unnecessary logging when the condition is not updated because it is the same as the current one.
func (s Status) UpdateComponentCondition(ctx context.Context, componentName Component, newReason Reason, newMessage string) (bool, error) {
	var wasUpdated bool
	retryErr := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		addon, err := s.fetchAddon(ctx)
		if err != nil {
			return err
		}

		newCondition := oav1beta1.StatusCondition{
			Type:               string(componentName),
			Reason:             string(newReason),
			Message:            newMessage,
			Status:             metav1.ConditionTrue,
			LastTransitionTime: metav1.NewTime(time.Now()),
		}

		currentCondition := getConditionByType(addon.Status.Conditions, string(componentName))

		// check if the condition needs to be updated
		isSameCondition := currentCondition != nil && currentCondition.Reason == newCondition.Reason && currentCondition.Message == newCondition.Message && currentCondition.Status == newCondition.Status
		if isSameCondition {
			return nil
		}

		// check if the transition is valid for the component
		// this is to avoid flapping between conditions
		if currentCondition != nil {
			if _, ok := componentTransitions[Reason(currentCondition.Reason)][newReason]; !ok {
				return fmt.Errorf("invalid transition from %s to %s for component %s", currentCondition.Reason, newReason, componentName)
			}
		}

		addon.Status.Conditions = mutateOrAppend(addon.Status.Conditions, newCondition)
		wasUpdated = true

		return s.client.Status().Update(ctx, addon)
	})
	if retryErr != nil {
		return wasUpdated, retryErr
	}

	return wasUpdated, nil
}

func (s Status) fetchAddon(ctx context.Context) (*oav1beta1.ObservabilityAddon, error) {
	obsAddon := &oav1beta1.ObservabilityAddon{}
	if err := s.client.Get(ctx, types.NamespacedName{Name: s.addonName, Namespace: s.addonNs}, obsAddon); err != nil {
		return nil, fmt.Errorf("failed to get ObservabilityAddon %s/%s: %w", s.addonNs, s.addonName, err)
	}
	return obsAddon, nil
}

func getConditionByType(conditions []oav1beta1.StatusCondition, conditionType string) *oav1beta1.StatusCondition {
	for _, condition := range conditions {
		if condition.Type == conditionType {
			return &condition
		}
	}
	return nil
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
