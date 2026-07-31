// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package util

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	"k8s.io/apimachinery/pkg/api/meta"
	addonv1beta1 "open-cluster-management.io/api/addon/v1beta1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
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

// HasMCOAManifestWorks checks for remaining MCOA ManifestWorks that contain metrics-specific
// resources (PrometheusAgent), and returns a sorted list of namespaces where such ManifestWorks
// are blocking the legacy addon deployment.
//
// RS-only ManifestWorks (containing only PrometheusRules) are safe to coexist with the legacy
// addon and are not included. PrometheusAgent is the definitive metrics marker: it is always
// present when MCOA platform metrics collection is active, always absent when disabled, and
// never produced by right-sizing.
//
// ManifestWorks on unavailable ManagedClusters are ignored to prevent disconnected spokes from
// hanging the cleanup process.
func HasMCOAManifestWorks(ctx context.Context, c client.Client) ([]string, error) {
	clusterList := &clusterv1.ManagedClusterList{}
	if err := c.List(ctx, clusterList); err != nil {
		return nil, fmt.Errorf("failed to list ManagedClusters: %w", err)
	}

	ignoredNamespaces := make(map[string]struct{})
	for _, mc := range clusterList.Items {
		isAvailable := meta.IsStatusConditionTrue(mc.Status.Conditions, clusterv1.ManagedClusterConditionAvailable)
		if !isAvailable {
			ignoredNamespaces[mc.Name] = struct{}{}
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
		if _, ignored := ignoredNamespaces[work.Namespace]; ignored {
			continue
		}
		if containsPrometheusAgent(work) {
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

// containsPrometheusAgent returns true if any manifest in the work has Kind "PrometheusAgent".
func containsPrometheusAgent(work workv1.ManifestWork) bool {
	for _, manifest := range work.Spec.Workload.Manifests {
		var partial struct {
			Kind string `json:"kind"`
		}
		if err := json.Unmarshal(manifest.Raw, &partial); err != nil {
			continue
		}
		if partial.Kind == "PrometheusAgent" {
			return true
		}
	}
	return false
}
