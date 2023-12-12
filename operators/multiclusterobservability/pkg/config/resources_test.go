// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package config

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	mcoshared "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/shared"
	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
)

func TestGetResources(t *testing.T) {
	caseList := []struct {
		name          string
		componentName string
		raw           *mcov1beta2.AdvancedConfig
		result        func(resources corev1.ResourceRequirements) bool
	}{
		{
			name:          "Have requests defined in resources",
			componentName: ObservatoriumAPI,
			raw: &mcov1beta2.AdvancedConfig{
				ObservatoriumAPI: &mcov1beta2.CommonSpec{
					Resources: &corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("1"),
							corev1.ResourceMemory: resource.MustParse("1Gi"),
						},
					},
				},
			},
			result: func(resources corev1.ResourceRequirements) bool {
				return resources.Requests.Cpu().String() == "1" &&
					resources.Requests.Memory().String() == "1Gi" &&
					resources.Limits.Cpu().String() == "0" &&
					resources.Limits.Memory().String() == "0"
			},
		},
		{
			name:          "Have limits defined in resources",
			componentName: ObservatoriumAPI,
			raw: &mcov1beta2.AdvancedConfig{
				ObservatoriumAPI: &mcov1beta2.CommonSpec{
					Resources: &corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("1"),
							corev1.ResourceMemory: resource.MustParse("1Gi"),
						},
					},
				},
			},
			result: func(resources corev1.ResourceRequirements) bool {
				return resources.Requests.Cpu().String() == ObservatoriumAPICPURequest[Small] &&
					resources.Requests.Memory().String() == ObservatoriumAPIMemoryRequest[Small] &&
					resources.Limits.Cpu().String() == "1" &&
					resources.Limits.Memory().String() == "1Gi"
			},
		},
		{
			name:          "Have limits defined in resources",
			componentName: RBACQueryProxy,
			raw: &mcov1beta2.AdvancedConfig{
				RBACQueryProxy: &mcov1beta2.CommonSpec{
					Resources: &corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("1"),
							corev1.ResourceMemory: resource.MustParse("1Gi"),
						},
					},
				},
			},
			result: func(resources corev1.ResourceRequirements) bool {
				return resources.Requests.Cpu().String() == RBACQueryProxyCPURequest[Small] &&
					resources.Requests.Memory().String() == RBACQueryProxyMemoryRequest[Small] &&
					resources.Limits.Cpu().String() == "1" &&
					resources.Limits.Memory().String() == "1Gi"
			},
		},
		{
			name:          "Have requests and limits defined in requests",
			componentName: ObservatoriumAPI,
			raw: &mcov1beta2.AdvancedConfig{
				ObservatoriumAPI: &mcov1beta2.CommonSpec{
					Resources: &corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("1"),
							corev1.ResourceMemory: resource.MustParse("1Gi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("1"),
							corev1.ResourceMemory: resource.MustParse("1Gi"),
						},
					},
				},
			},
			result: func(resources corev1.ResourceRequirements) bool {
				return resources.Requests.Cpu().String() == "1" &&
					resources.Requests.Memory().String() == "1Gi" &&
					resources.Limits.Cpu().String() == "1" &&
					resources.Limits.Memory().String() == "1Gi"
			},
		},
		{
			name:          "No CPU defined in requests",
			componentName: ObservatoriumAPI,
			raw: &mcov1beta2.AdvancedConfig{
				ObservatoriumAPI: &mcov1beta2.CommonSpec{
					Resources: &corev1.ResourceRequirements{
						Requests: corev1.ResourceList{},
					},
				},
			},
			result: func(resources corev1.ResourceRequirements) bool {
				return resources.Requests.Cpu().String() == ObservatoriumAPICPURequest[Small] &&
					resources.Requests.Memory().String() == ObservatoriumAPIMemoryRequest[Small] &&
					resources.Limits.Cpu().String() == "0" && resources.Limits.Memory().String() == "0"
			},
		},
		{
			name:          "No requests defined in resources",
			componentName: ObservatoriumAPI,
			raw: &mcov1beta2.AdvancedConfig{
				ObservatoriumAPI: &mcov1beta2.CommonSpec{
					Resources: &corev1.ResourceRequirements{},
				},
			},
			result: func(resources corev1.ResourceRequirements) bool {
				return resources.Requests.Cpu().String() == ObservatoriumAPICPURequest[Small] &&
					resources.Requests.Memory().String() == ObservatoriumAPIMemoryRequest[Small] &&
					resources.Limits.Cpu().String() == "0" && resources.Limits.Memory().String() == "0"
			},
		},
		{
			name:          "No resources defined",
			componentName: ObservatoriumAPI,
			raw: &mcov1beta2.AdvancedConfig{
				ObservatoriumAPI: &mcov1beta2.CommonSpec{},
			},
			result: func(resources corev1.ResourceRequirements) bool {
				return resources.Requests.Cpu().String() == ObservatoriumAPICPURequest[Small] &&
					resources.Requests.Memory().String() == ObservatoriumAPIMemoryRequest[Small] &&
					resources.Limits.Cpu().String() == "0" && resources.Limits.Memory().String() == "0"
			},
		},
		{
			name:          "No advanced defined",
			componentName: ObservatoriumAPI,
			raw:           nil,
			result: func(resources corev1.ResourceRequirements) bool {
				return resources.Requests.Cpu().String() == ObservatoriumAPICPURequest[Small] &&
					resources.Requests.Memory().String() == ObservatoriumAPIMemoryRequest[Small] &&
					resources.Limits.Cpu().String() == "0" && resources.Limits.Memory().String() == "0"
			},
		},
		{
			name:          "No advanced defined",
			componentName: Grafana,
			raw:           nil,
			result: func(resources corev1.ResourceRequirements) bool {
				return resources.Requests.Cpu().String() == GrafanaCPURequest[Small] &&
					resources.Requests.Memory().String() == GrafanaMemoryRequest[Small] &&
					resources.Limits.Cpu().String() == GrafanaCPULimit &&
					resources.Limits.Memory().String() == GrafanaMemoryLimit
			},
		},
		{
			name:          "Have requests defined",
			componentName: Grafana,
			raw: &mcov1beta2.AdvancedConfig{
				Grafana: &mcov1beta2.CommonSpec{
					Resources: &corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("1"),
						},
					},
				},
			},
			result: func(resources corev1.ResourceRequirements) bool {
				return resources.Requests.Cpu().String() == "1" &&
					resources.Requests.Memory().String() == GrafanaMemoryRequest[Small] &&
					resources.Limits.Cpu().String() == GrafanaCPULimit &&
					resources.Limits.Memory().String() == GrafanaMemoryLimit
			},
		},
		{
			name:          "Have limits defined",
			componentName: Grafana,
			raw: &mcov1beta2.AdvancedConfig{
				Grafana: &mcov1beta2.CommonSpec{
					Resources: &corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("1"),
						},
					},
				},
			},
			result: func(resources corev1.ResourceRequirements) bool {
				return resources.Requests.Cpu().String() == GrafanaCPURequest[Small] &&
					resources.Requests.Memory().String() == GrafanaMemoryRequest[Small] &&
					resources.Limits.Cpu().String() == "1" &&
					resources.Limits.Memory().String() == GrafanaMemoryLimit
			},
		},
		{
			name:          "Have limits defined",
			componentName: Grafana,
			raw: &mcov1beta2.AdvancedConfig{
				Grafana: &mcov1beta2.CommonSpec{
					Resources: &corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("1"),
							corev1.ResourceMemory: resource.MustParse("1Gi"),
						},
					},
				},
			},
			result: func(resources corev1.ResourceRequirements) bool {
				return resources.Requests.Cpu().String() == GrafanaCPURequest[Small] &&
					resources.Requests.Memory().String() == GrafanaMemoryRequest[Small] &&
					resources.Limits.Cpu().String() == "1" &&
					resources.Limits.Memory().String() == "1Gi"
			},
		},
		{
			name:          "Have limits defined",
			componentName: ThanosQueryFrontendMemcached,
			raw: &mcov1beta2.AdvancedConfig{
				QueryFrontendMemcached: &mcov1beta2.CacheConfig{
					CommonSpec: mcov1beta2.CommonSpec{
						Resources: &corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("1"),
								corev1.ResourceMemory: resource.MustParse("1Gi"),
							},
						},
					},
				},
			},
			result: func(resources corev1.ResourceRequirements) bool {
				return resources.Requests.Cpu().String() == ThanosCachedCPURequest[Small] &&
					resources.Requests.Memory().String() == ThanosCachedMemoryRequest[Small] &&
					resources.Limits.Cpu().String() == "1" &&
					resources.Limits.Memory().String() == "1Gi"
			},
		},
	}

	for _, c := range caseList {
		t.Run(c.componentName+":"+c.name, func(t *testing.T) {
			resources := GetResources(c.componentName, Small, c.raw)
			if !c.result(resources) {
				t.Errorf("case (%v) output (%v) is not the expected", c.componentName+":"+c.name, resources)
			}
		})
	}
}

func TestGetReplicas(t *testing.T) {
	var replicas0 int32 = 0
	caseList := []struct {
		name          string
		componentName string
		raw           *mcov1beta2.AdvancedConfig
		result        func(replicas *int32) bool
	}{
		{
			name:          "Have replicas defined",
			componentName: ObservatoriumAPI,
			raw: &mcov1beta2.AdvancedConfig{
				ObservatoriumAPI: &mcov1beta2.CommonSpec{
					Replicas: &Replicas1,
				},
			},
			result: func(replicas *int32) bool {
				return replicas == &Replicas1
			},
		},
		{
			name:          "Do not allow to set 0",
			componentName: ObservatoriumAPI,
			raw: &mcov1beta2.AdvancedConfig{
				ObservatoriumAPI: &mcov1beta2.CommonSpec{
					Replicas: &replicas0,
				},
			},
			result: func(replicas *int32) bool {
				return replicas == &Replicas2
			},
		},
		{
			name:          "No advanced defined",
			componentName: ObservatoriumAPI,
			raw:           nil,
			result: func(replicas *int32) bool {
				return replicas == &Replicas2
			},
		},
		{
			name:          "No replicas defined",
			componentName: ObservatoriumAPI,
			raw: &mcov1beta2.AdvancedConfig{
				ObservatoriumAPI: &mcov1beta2.CommonSpec{},
			},
			result: func(replicas *int32) bool {
				return replicas == &Replicas2
			},
		},
	}
	for _, c := range caseList {
		t.Run(c.componentName+":"+c.name, func(t *testing.T) {
			replicas := GetReplicas(c.componentName, c.raw)
			if !c.result(replicas) {
				t.Errorf("case (%v) output (%v) is not the expected", c.componentName+":"+c.name, replicas)
			}
		})
	}
}

func TestGetOBAResources(t *testing.T) {
	caseList := []struct {
		name          string
		componentName string
		raw           *mcoshared.ObservabilityAddonSpec
		result        func(resources corev1.ResourceRequirements) bool
	}{
		{
			name:          "Have requests defined",
			componentName: ObservatoriumAPI,
			raw: &mcoshared.ObservabilityAddonSpec{
				Resources: &corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("1"),
						corev1.ResourceMemory: resource.MustParse("1Gi"),
					},
				},
			},
			result: func(resources corev1.ResourceRequirements) bool {
				return resources.Requests.Cpu().String() == "1" &&
					resources.Requests.Memory().String() == "1Gi" &&
					resources.Limits.Cpu().String() == "0" &&
					resources.Limits.Memory().String() == "0"
			},
		},
		{
			name:          "Have limits defined",
			componentName: ObservatoriumAPI,
			raw: &mcoshared.ObservabilityAddonSpec{
				Resources: &corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU: resource.MustParse("1"),
					},
				},
			},
			result: func(resources corev1.ResourceRequirements) bool {
				return resources.Requests.Cpu().String() == MetricsCollectorCPURequest[Small] &&
					resources.Requests.Memory().String() == MetricsCollectorMemoryRequest[Small] &&
					resources.Limits.Cpu().String() == "1" &&
					resources.Limits.Memory().String() == "0"
			},
		},
		{
			name:          "no resources defined",
			componentName: ObservatoriumAPI,
			raw: &mcoshared.ObservabilityAddonSpec{
				Resources: &corev1.ResourceRequirements{},
			},
			result: func(resources corev1.ResourceRequirements) bool {
				return resources.Requests.Cpu().String() == MetricsCollectorCPURequest[Small] &&
					resources.Requests.Memory().String() == MetricsCollectorMemoryRequest[Small] &&
					resources.Limits.Cpu().String() == "0" &&
					resources.Limits.Memory().String() == "0"
			},
		},
	}
	for _, c := range caseList {
		t.Run(c.componentName+":"+c.name, func(t *testing.T) {
			resources := GetOBAResources(c.raw, Small)
			if !c.result(*resources) {
				t.Errorf("case (%v) output (%v) is not the expected", c.componentName+":"+c.name, resources)
			}
		})
	}
}
