// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package config

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	mcoshared "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/shared"
	observabilityv1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
)

// Specifies resources for all components.
const (
	RBACQueryProxyCPURequest    = "20m"
	RBACQueryProxyMemoryRequest = "100Mi"

	GrafanaCPURequest    = "4m"
	GrafanaMemoryRequest = "100Mi"
	GrafanaCPULimit      = "500m"
	GrafanaMemoryLimit   = "1Gi"

	AlertmanagerCPURequest    = "4m"
	AlertmanagerMemoryRequest = "200Mi"

	ObservatoriumAPICPURequest    = "20m"
	ObservatoriumAPIMemoryRequest = "128Mi"

	ThanosQueryFrontendCPURequest    = "100m"
	ThanosQueryFrontendMemoryRequest = "256Mi"

	MemcachedExporterCPURequest    = "5m"
	MemcachedExporterMemoryRequest = "50Mi"

	ThanosQueryCPURequest    = "300m"
	ThanosQueryMemoryRequest = "1Gi"

	ThanosCompactCPURequest    = "100m"
	ThanosCompactMemoryRequest = "512Mi"

	ObservatoriumReceiveControllerCPURequest    = "4m"
	ObservatoriumReceiveControllerMemoryRequest = "32Mi"

	ThanosReceiveCPURequest    = "300m"
	ThanosReceiveMemoryRequest = "512Mi"

	ThanosRuleCPURequest            = "50m"
	ThanosRuleMemoryRequest         = "512Mi"
	ThanosRuleReloaderCPURequest    = "4m"
	ThanosRuleReloaderMemoryRequest = "25Mi"

	ThanosCachedCPURequest            = "45m"
	ThanosCachedMemoryRequest         = "128Mi"
	ThanosCachedExporterCPURequest    = "5m"
	ThanosCachedExporterMemoryRequest = "50Mi"

	ThanosStoreCPURequest    = "100m"
	ThanosStoreMemoryRequest = "1Gi"

	MetricsCollectorCPURequest    = "10m"
	MetricsCollectorMemoryRequest = "100Mi"
	MetricsCollectorCPULimits     = ""
	MetricsCollectorMemoryLimits  = ""

	ResourceLimits                        = "limits"
	ResourceRequests                      = "requests"
	AnnotationMCOWithoutResourcesRequests = "mco-thanos-without-resources-requests"
)

// Specifies replicas for all components.
var (
	Replicas1 int32 = 1
	Replicas2 int32 = 2
	Replicas3 int32 = 3
	Replicas        = map[string]*int32{
		ObservatoriumAPI:    &Replicas2,
		ThanosQuery:         &Replicas2,
		ThanosQueryFrontend: &Replicas2,
		Grafana:             &Replicas2,
		RBACQueryProxy:      &Replicas2,

		ThanosRule:                   &Replicas3,
		ThanosReceive:                &Replicas3,
		ThanosStoreShard:             &Replicas3,
		ThanosStoreMemcached:         &Replicas3,
		ThanosQueryFrontendMemcached: &Replicas3,
		Alertmanager:                 &Replicas3,
	}
)

// getDefaultResourceCPU returns the default resource CPU request for a particular o11y workload.
func getDefaultResourceCPU(component string) string {
	switch component {
	case ObservatoriumAPI:
		return ObservatoriumAPICPURequest
	case ThanosCompact:
		return ThanosCompactCPURequest
	case ThanosQuery:
		return ThanosQueryCPURequest
	case ThanosQueryFrontend:
		return ThanosQueryFrontendCPURequest
	case ThanosRule:
		return ThanosRuleCPURequest
	case ThanosReceive:
		return ThanosReceiveCPURequest
	case ThanosStoreShard:
		return ThanosStoreCPURequest
	case ThanosQueryFrontendMemcached, ThanosStoreMemcached:
		return ThanosCachedCPURequest
	case MemcachedExporter:
		return MemcachedExporterCPURequest
	case RBACQueryProxy:
		return RBACQueryProxyCPURequest
	case MetricsCollector:
		return MetricsCollectorCPURequest
	case Alertmanager:
		return AlertmanagerCPURequest
	case Grafana:
		return GrafanaCPURequest
	default:
		return ""
	}
}

// getDefaultResourceMemory returns the default resource memory request for a particular o11y workload.
func getDefaultResourceMemory(component string) string {
	switch component {
	case ObservatoriumAPI:
		return ObservatoriumAPIMemoryRequest
	case ThanosCompact:
		return ThanosCompactMemoryRequest
	case ThanosQuery:
		return ThanosQueryMemoryRequest
	case ThanosQueryFrontend:
		return ThanosQueryFrontendMemoryRequest
	case ThanosRule:
		return ThanosRuleMemoryRequest
	case ThanosReceive:
		return ThanosReceiveMemoryRequest
	case ThanosStoreShard:
		return ThanosStoreMemoryRequest
	case ThanosQueryFrontendMemcached, ThanosStoreMemcached:
		return ThanosCachedMemoryRequest
	case MemcachedExporter:
		return MemcachedExporterMemoryRequest
	case RBACQueryProxy:
		return RBACQueryProxyMemoryRequest
	case MetricsCollector:
		return MetricsCollectorMemoryRequest
	case Alertmanager:
		return AlertmanagerMemoryRequest
	case Grafana:
		return GrafanaMemoryRequest
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
func getDefaultResourceRequirements(component string) corev1.ResourceRequirements {
	requests := corev1.ResourceList{}
	limits := corev1.ResourceList{}

	memoryRequest := getDefaultResourceMemory(component)
	cpuRequest := getDefaultResourceCPU(component)

	memoryLimit := getDefaultResourceMemoryLimit(component)
	cpuLimit := getDefaultResourceCPULimit(component)

	requests[corev1.ResourceName(corev1.ResourceMemory)] = resource.MustParse(memoryRequest)
	requests[corev1.ResourceName(corev1.ResourceCPU)] = resource.MustParse(cpuRequest)

	if memoryLimit != "" {
		limits[corev1.ResourceName(corev1.ResourceMemory)] = resource.MustParse(memoryLimit)
	}

	if cpuLimit != "" {
		limits[corev1.ResourceName(corev1.ResourceCPU)] = resource.MustParse(cpuLimit)
	}

	return corev1.ResourceRequirements{
		Requests: requests,
		Limits:   limits,
	}
}

// getAdvancedConfigResourceOverride returns the AdvancedConfig overriden resource requirements for a particular o11y workload.
func getAdvancedConfigResourceOverride(component string, advanced *observabilityv1beta2.AdvancedConfig) corev1.ResourceRequirements {
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
	}

	final := corev1.ResourceRequirements{}
	// Validate config and combine defaults.
	if resourcesReq != nil {
		if len(resourcesReq.Requests) != 0 {
			final.Requests = resourcesReq.Requests
			if resourcesReq.Requests.Cpu().String() == "0" {
				final.Requests[corev1.ResourceCPU] = getDefaultResourceRequirements(component).Requests[corev1.ResourceCPU]
			}
			if resourcesReq.Requests.Memory().String() == "0" {
				final.Requests[corev1.ResourceMemory] = getDefaultResourceRequirements(component).Requests[corev1.ResourceMemory]
			}
		} else {
			final.Requests = getDefaultResourceRequirements(component).Requests
		}

		if len(resourcesReq.Limits) != 0 {
			final.Limits = resourcesReq.Limits
			if resourcesReq.Limits.Cpu().String() == "0" {
				final.Limits[corev1.ResourceCPU] = getDefaultResourceRequirements(component).Limits[corev1.ResourceCPU]
			}
			if resourcesReq.Limits.Memory().String() == "0" {
				final.Limits[corev1.ResourceMemory] = getDefaultResourceRequirements(component).Limits[corev1.ResourceMemory]
			}
		} else {
			final.Limits = getDefaultResourceRequirements(component).Limits
		}

		return final
	}

	return getDefaultResourceRequirements(component)
}

// GetResources returns the pre-set resource requirements for a particular o11y workload.
// Always default unless configured via advancedConfig, in which case it is overriden.
func GetResources(component string, advanced *observabilityv1beta2.AdvancedConfig) corev1.ResourceRequirements {
	resourceReq := getDefaultResourceRequirements(component)
	if advanced != nil {
		resourceReq = getAdvancedConfigResourceOverride(component, advanced)
	}

	return resourceReq
}

// GetOBAResources returns the pre-set resource requirements for metrics collector.
func GetOBAResources(oba *mcoshared.ObservabilityAddonSpec) *corev1.ResourceRequirements {
	cpuRequests := MetricsCollectorCPURequest
	cpuLimits := MetricsCollectorCPULimits
	memoryRequests := MetricsCollectorMemoryRequest
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
		requests[corev1.ResourceName(corev1.ResourceCPU)] = resource.MustParse(cpuRequests)
	}
	if memoryRequests != "" {
		requests[corev1.ResourceName(corev1.ResourceMemory)] = resource.MustParse(memoryRequests)
	}
	if cpuLimits != "" {
		limits[corev1.ResourceName(corev1.ResourceCPU)] = resource.MustParse(cpuLimits)
	}
	if memoryLimits != "" {
		limits[corev1.ResourceName(corev1.ResourceMemory)] = resource.MustParse(memoryLimits)
	}
	resourceReq.Limits = limits
	resourceReq.Requests = requests

	return resourceReq
}

// GetReplicas returns the default replicas for a particular o11y workload.
func GetReplicas(component string, advanced *observabilityv1beta2.AdvancedConfig) *int32 {
	if advanced == nil {
		return Replicas[component]
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
	}

	if replicas == nil || *replicas == 0 {
		replicas = Replicas[component]
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
