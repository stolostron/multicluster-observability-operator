// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package config

import (
	"strings"

	mcoshared "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/shared"
	observabilityv1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	ResourceLimits                        = "limits"
	ResourceRequests                      = "requests"
	AnnotationMCOWithoutResourcesRequests = "mco-thanos-without-resources-requests"
)

// getDefaultResourceCPU returns the default resource CPU request for a particular o11y workload.
func getDefaultResourceCPU(component string, tshirtSize observabilityv1beta2.TShirtSize) string {
	switch component {
	case ObservatoriumAPI:
		return ObservatoriumAPICPURequest[tshirtSize]
	case ThanosCompact:
		return ThanosCompactCPURequest[tshirtSize]
	case ThanosQuery:
		return ThanosQueryCPURequest[tshirtSize]
	case ThanosQueryFrontend:
		return ThanosQueryFrontendCPURequest[tshirtSize]
	case ThanosRule:
		return ThanosRuleCPURequest[tshirtSize]
	case ThanosReceive:
		return ThanosReceiveCPURequest[tshirtSize]
	case ThanosStoreShard:
		return ThanosStoreCPURequest[tshirtSize]
	case ThanosQueryFrontendMemcached:
		return ThanosQueryMemcachedCPURequest[tshirtSize]
	case ThanosStoreMemcached:
		return ThanosStoreMemcachedCPURequest[tshirtSize]
	case MemcachedExporter:
		return MemcachedExporterCPURequest[tshirtSize]
	case RBACQueryProxy:
		return RBACQueryProxyCPURequest[tshirtSize]
	case MetricsCollector:
		return MetricsCollectorCPURequest[tshirtSize]
	case Alertmanager:
		return AlertmanagerCPURequest[tshirtSize]
	case Grafana:
		return GrafanaCPURequest[tshirtSize]
	case MultiClusterObservabilityAddon:
		return MCOACPURequest[tshirtSize]
	default:
		return ""
	}
}

// getDefaultResourceMemory returns the default resource memory request for a particular o11y workload.
func getDefaultResourceMemory(component string, tshirtSize observabilityv1beta2.TShirtSize) string {
	switch component {
	case ObservatoriumAPI:
		return ObservatoriumAPIMemoryRequest[tshirtSize]
	case ThanosCompact:
		return ThanosCompactMemoryRequest[tshirtSize]
	case ThanosQuery:
		return ThanosQueryMemoryRequest[tshirtSize]
	case ThanosQueryFrontend:
		return ThanosQueryFrontendMemoryRequest[tshirtSize]
	case ThanosRule:
		return ThanosRuleMemoryRequest[tshirtSize]
	case ThanosReceive:
		return ThanosReceiveMemoryRequest[tshirtSize]
	case ThanosStoreShard:
		return ThanosStoreMemoryRequest[tshirtSize]
	case ThanosQueryFrontendMemcached:
		return ThanosQueryMemcachedMemoryRequest[tshirtSize]
	case ThanosStoreMemcached:
		return ThanosStoreMemcachedMemoryRequest[tshirtSize]
	case MemcachedExporter:
		return MemcachedExporterMemoryRequest[tshirtSize]
	case RBACQueryProxy:
		return RBACQueryProxyMemoryRequest[tshirtSize]
	case MetricsCollector:
		return MetricsCollectorMemoryRequest[tshirtSize]
	case Alertmanager:
		return AlertmanagerMemoryRequest[tshirtSize]
	case Grafana:
		return GrafanaMemoryRequest[tshirtSize]
	case MultiClusterObservabilityAddon:
		return MCOAMemoryRequest[tshirtSize]
	default:
		return ""
	}
}

// getDefaultResourceMemoryLimit returns the default resource memory limit for a particular o11y workload.
func getDefaultResourceMemoryLimit(component string) string {
	switch component {
	case Grafana:
		return GrafanaMemoryLimit
	default:
		return ""
	}
}

// getDefaultResourceCPULimit returns the default resource CPU limit for a particular o11y workload.
func getDefaultResourceCPULimit(component string) string {
	switch component {
	case Grafana:
		return GrafanaCPULimit
	default:
		return ""
	}
}

// getDefaultResourceRequirements returns the default resource requirements for a particular o11y workload.
func getDefaultResourceRequirements(component string, tshirtSize observabilityv1beta2.TShirtSize) corev1.ResourceRequirements {
	requests := corev1.ResourceList{}
	limits := corev1.ResourceList{}

	memoryRequest := getDefaultResourceMemory(component, tshirtSize)
	cpuRequest := getDefaultResourceCPU(component, tshirtSize)

	memoryLimit := getDefaultResourceMemoryLimit(component)
	cpuLimit := getDefaultResourceCPULimit(component)

	requests[corev1.ResourceMemory] = resource.MustParse(memoryRequest)
	requests[corev1.ResourceCPU] = resource.MustParse(cpuRequest)

	if memoryLimit != "" {
		limits[corev1.ResourceMemory] = resource.MustParse(memoryLimit)
	}

	if cpuLimit != "" {
		limits[corev1.ResourceCPU] = resource.MustParse(cpuLimit)
	}

	return corev1.ResourceRequirements{
		Requests: requests,
		Limits:   limits,
	}
}

// getAdvancedConfigResourceOverride returns the AdvancedConfig overridden resource requirements for a particular o11y workload.
func getAdvancedConfigResourceOverride(component string, tshirtSize observabilityv1beta2.TShirtSize, advanced *observabilityv1beta2.AdvancedConfig) corev1.ResourceRequirements {
	resourcesReq := &corev1.ResourceRequirements{}
	switch component {
	case ObservatoriumAPI:
		if advanced.ObservatoriumAPI != nil {
			resourcesReq = advanced.ObservatoriumAPI.Resources
		}
	case ThanosCompact:
		if advanced.Compact != nil {
			resourcesReq = advanced.Compact.Resources
		}
	case ThanosQuery:
		if advanced.Query != nil {
			resourcesReq = advanced.Query.Resources
		}
	case ThanosQueryFrontend:
		if advanced.QueryFrontend != nil {
			resourcesReq = advanced.QueryFrontend.Resources
		}
	case ThanosQueryFrontendMemcached:
		if advanced.QueryFrontendMemcached != nil {
			resourcesReq = advanced.QueryFrontendMemcached.Resources
		}
	case ThanosRule:
		if advanced.Rule != nil {
			resourcesReq = advanced.Rule.Resources
		}
	case ThanosReceive:
		if advanced.Receive != nil {
			resourcesReq = advanced.Receive.Resources
		}
	case ThanosStoreMemcached:
		if advanced.StoreMemcached != nil {
			resourcesReq = advanced.StoreMemcached.Resources
		}
	case ThanosStoreShard:
		if advanced.Store != nil {
			resourcesReq = advanced.Store.Resources
		}
	case RBACQueryProxy:
		if advanced.RBACQueryProxy != nil {
			resourcesReq = advanced.RBACQueryProxy.Resources
		}
	case Grafana:
		if advanced.Grafana != nil {
			resourcesReq = advanced.Grafana.Resources
		}
	case Alertmanager:
		if advanced.Alertmanager != nil {
			resourcesReq = advanced.Alertmanager.Resources
		}
	case MultiClusterObservabilityAddon:
		if advanced.MultiClusterObservabilityAddon != nil {
			resourcesReq = advanced.MultiClusterObservabilityAddon.Resources
		}
	}

	final := corev1.ResourceRequirements{}
	// Validate config and combine defaults.
	if resourcesReq != nil {
		if len(resourcesReq.Requests) != 0 {
			final.Requests = resourcesReq.Requests
			if resourcesReq.Requests.Cpu().String() == "0" {
				final.Requests[corev1.ResourceCPU] = getDefaultResourceRequirements(component, tshirtSize).Requests[corev1.ResourceCPU]
			}
			if resourcesReq.Requests.Memory().String() == "0" {
				final.Requests[corev1.ResourceMemory] = getDefaultResourceRequirements(component, tshirtSize).Requests[corev1.ResourceMemory]
			}
		} else {
			final.Requests = getDefaultResourceRequirements(component, tshirtSize).Requests
		}

		if len(resourcesReq.Limits) != 0 {
			final.Limits = resourcesReq.Limits
			if resourcesReq.Limits.Cpu().String() == "0" {
				final.Limits[corev1.ResourceCPU] = getDefaultResourceRequirements(component, tshirtSize).Limits[corev1.ResourceCPU]
			}
			if resourcesReq.Limits.Memory().String() == "0" {
				final.Limits[corev1.ResourceMemory] = getDefaultResourceRequirements(component, tshirtSize).Limits[corev1.ResourceMemory]
			}
		} else {
			final.Limits = getDefaultResourceRequirements(component, tshirtSize).Limits
		}

		return final
	}

	return getDefaultResourceRequirements(component, tshirtSize)
}

// GetResources returns the pre-set resource requirements for a particular o11y workload.
// Always default unless configured via advancedConfig, in which case it is overridden.
func GetResources(component string, tshirtSize observabilityv1beta2.TShirtSize, advanced *observabilityv1beta2.AdvancedConfig) corev1.ResourceRequirements {
	if tshirtSize == "" {
		tshirtSize = Default
	}

	resourceReq := getDefaultResourceRequirements(component, tshirtSize)
	if advanced != nil {
		resourceReq = getAdvancedConfigResourceOverride(component, tshirtSize, advanced)
	}

	return resourceReq
}

// GetOBAResources returns the pre-set resource requirements for metrics collector.
func GetOBAResources(oba *mcoshared.ObservabilityAddonSpec, tshirtSize observabilityv1beta2.TShirtSize) *corev1.ResourceRequirements {
	if tshirtSize == "" {
		tshirtSize = Default
	}

	cpuRequests := MetricsCollectorCPURequest[tshirtSize]
	cpuLimits := MetricsCollectorCPULimits
	memoryRequests := MetricsCollectorMemoryRequest[tshirtSize]
	memoryLimits := MetricsCollectorMemoryLimits
	resourceReq := &corev1.ResourceRequirements{}

	if oba.Resources != nil {
		if len(oba.Resources.Requests) != 0 {
			if oba.Resources.Requests.Cpu().String() != "0" {
				cpuRequests = oba.Resources.Requests.Cpu().String()
			}
			if oba.Resources.Requests.Memory().String() != "0" {
				memoryRequests = oba.Resources.Requests.Memory().String()
			}
		}
		if len(oba.Resources.Limits) != 0 {
			if oba.Resources.Limits.Cpu().String() != "0" {
				cpuLimits = oba.Resources.Limits.Cpu().String()
			}
			if oba.Resources.Limits.Memory().String() != "0" {
				memoryLimits = oba.Resources.Limits.Memory().String()
			}
		}
	}

	requests := corev1.ResourceList{}
	limits := corev1.ResourceList{}
	if cpuRequests != "" {
		requests[corev1.ResourceCPU] = resource.MustParse(cpuRequests)
	}
	if memoryRequests != "" {
		requests[corev1.ResourceMemory] = resource.MustParse(memoryRequests)
	}
	if cpuLimits != "" {
		limits[corev1.ResourceCPU] = resource.MustParse(cpuLimits)
	}
	if memoryLimits != "" {
		limits[corev1.ResourceMemory] = resource.MustParse(memoryLimits)
	}
	resourceReq.Limits = limits
	resourceReq.Requests = requests

	return resourceReq
}

// GetReplicas returns the default replicas for a particular o11y workload.
func GetReplicas(component string, tshirtSize observabilityv1beta2.TShirtSize, advanced *observabilityv1beta2.AdvancedConfig) *int32 {
	if tshirtSize == "" {
		tshirtSize = Default
	}

	if advanced == nil {
		return Replicas[component][tshirtSize]
	}
	var replicas *int32
	switch component {
	case ObservatoriumAPI:
		if advanced.ObservatoriumAPI != nil {
			replicas = advanced.ObservatoriumAPI.Replicas
		}
	case ThanosQuery:
		if advanced.Query != nil {
			replicas = advanced.Query.Replicas
		}
	case ThanosQueryFrontend:
		if advanced.QueryFrontend != nil {
			replicas = advanced.QueryFrontend.Replicas
		}
	case ThanosQueryFrontendMemcached:
		if advanced.QueryFrontendMemcached != nil {
			replicas = advanced.QueryFrontendMemcached.Replicas
		}
	case ThanosRule:
		if advanced.Rule != nil {
			replicas = advanced.Rule.Replicas
		}
	case ThanosReceive:
		if advanced.Receive != nil {
			replicas = advanced.Receive.Replicas
		}
	case ThanosStoreMemcached:
		if advanced.StoreMemcached != nil {
			replicas = advanced.StoreMemcached.Replicas
		}
	case ThanosStoreShard:
		if advanced.Store != nil {
			replicas = advanced.Store.Replicas
		}
	case RBACQueryProxy:
		if advanced.RBACQueryProxy != nil {
			replicas = advanced.RBACQueryProxy.Replicas
		}
	case Grafana:
		if advanced.Grafana != nil {
			replicas = advanced.Grafana.Replicas
		}
	case Alertmanager:
		if advanced.Alertmanager != nil {
			replicas = advanced.Alertmanager.Replicas
		}
	case MultiClusterObservabilityAddon:
		if advanced.MultiClusterObservabilityAddon != nil {
			replicas = advanced.MultiClusterObservabilityAddon.Replicas
		}
	}

	if replicas == nil || *replicas == 0 {
		replicas = Replicas[component][tshirtSize]
	}
	return replicas
}

// WithoutResourcesRequests returns true if the multiclusterobservability instance has annotation:
// mco-thanos-without-resources-requests: "true"
// This is just for test purpose: the KinD cluster does not have enough resources for the requests.
// We won't expose this annotation to the customer.
func WithoutResourcesRequests(annotations map[string]string) bool {
	if annotations == nil {
		return false
	}

	if annotations[AnnotationMCOWithoutResourcesRequests] != "" &&
		strings.EqualFold(annotations[AnnotationMCOWithoutResourcesRequests], "true") {
		return true
	}

	return false
}
