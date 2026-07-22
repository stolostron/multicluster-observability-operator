// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package util

import (
	"context"
	"fmt"
	"slices"

	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	addonv1beta1 "open-cluster-management.io/api/addon/v1beta1"
	workv1 "open-cluster-management.io/api/work/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// IsMCOAEnabled returns true if any MCOA capability is enabled.
func IsMCOAEnabled(mco *mcov1beta2.MultiClusterObservability) bool {
	if mco == nil || mco.Spec.Capabilities == nil {
		return false
	}

	if mco.Spec.Capabilities.Platform != nil {
		if mco.Spec.Capabilities.Platform.Logs.Collection.Enabled ||
			mco.Spec.Capabilities.Platform.Metrics.Default.Enabled ||
			mco.Spec.Capabilities.Platform.Analytics.IncidentDetection.Enabled {
			return true
		}
	}

	if mco.Spec.Capabilities.UserWorkloads != nil {
		if mco.Spec.Capabilities.UserWorkloads.Logs.Collection.ClusterLogForwarder.Enabled ||
			mco.Spec.Capabilities.UserWorkloads.Metrics.Default.Enabled {
			return true
		}
	}

	return false
}

// HasMCOAManifestWorks checks for remaining ManifestWorks for the MCOA addon on the hub,
// and returns a sorted list of namespaces where ManifestWorks are blocking the deletion.
func HasMCOAManifestWorks(ctx context.Context, c client.Client) ([]string, error) {
	addonList := &addonv1beta1.ManagedClusterAddOnList{}
	if err := c.List(ctx, addonList, client.MatchingLabels{
		addonv1beta1.AddonLabelKey: config.MultiClusterObservabilityAddon,
	}); err != nil {
		return nil, fmt.Errorf("failed to list ManagedClusterAddOns: %w", err)
	}

	ignoredNamespaces := make(map[string]struct{})
	for _, addon := range addonList.Items {
		isAvailable := false
		for _, cond := range addon.Status.Conditions {
			if cond.Type == "Available" && cond.Status == metav1.ConditionTrue {
				isAvailable = true
				break
			}
		}
		// If the ManagedClusterAddOn exists on the Hub but is NOT available (stalled, offline, etc.),
		// we ignore its ManifestWorks to prevent disconnected spokes from hanging the cleanup process.
		if !isAvailable {
			ignoredNamespaces[addon.Namespace] = struct{}{}
		}
	}

	workList := &workv1.ManifestWorkList{}
	opts := []client.ListOption{
		client.MatchingLabels{
			addonv1beta1.AddonLabelKey: config.MultiClusterObservabilityAddon,
		},
	}
	if err := c.List(ctx, workList, opts...); err != nil {
		return nil, fmt.Errorf("failed to list ManifestWorks: %w", err)
	}

	blockingMap := make(map[string]struct{})
	for _, work := range workList.Items {
		if _, ignored := ignoredNamespaces[work.Namespace]; !ignored {
			blockingMap[work.Namespace] = struct{}{}
		}
	}

	if len(blockingMap) == 0 {
		return nil, nil
	}

	blockingNamespaces := make([]string, 0, len(blockingMap))
	for ns := range blockingMap {
		blockingNamespaces = append(blockingNamespaces, ns)
	}
	slices.Sort(blockingNamespaces)

	return blockingNamespaces, nil
}
