// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package config

import (
	observabilityv1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
)

const (
	// To be only used for testing.
	Minimal observabilityv1beta2.TShirtSize = "minimal"

	// To be used for actual setups.
	Default     observabilityv1beta2.TShirtSize = "default"
	Small       observabilityv1beta2.TShirtSize = "small"
	Medium      observabilityv1beta2.TShirtSize = "medium"
	Large       observabilityv1beta2.TShirtSize = "large"
	XLarge      observabilityv1beta2.TShirtSize = "xlarge"
	TwoXLarge   observabilityv1beta2.TShirtSize = "2xlarge"
	FourXLarge  observabilityv1beta2.TShirtSize = "4xlarge"
	EightXLarge observabilityv1beta2.TShirtSize = "8xlarge"
)

type ResourceSizeMap map[observabilityv1beta2.TShirtSize]string

// Specifies resources for all components and their respective TShirt sizes.
// TODO(saswatamcode): Figure out the right values for these. They are all the same at the moment.
var (
	RBACQueryProxyCPURequest ResourceSizeMap = map[observabilityv1beta2.TShirtSize]string{
		Minimal:     "20m",
		Default:     "20m",
		Small:       "20m",
		Medium:      "20m",
		Large:       "20m",
		XLarge:      "20m",
		TwoXLarge:   "20m",
		FourXLarge:  "20m",
		EightXLarge: "20m",
	}
	RBACQueryProxyMemoryRequest ResourceSizeMap = map[observabilityv1beta2.TShirtSize]string{
		Minimal:     "100Mi",
		Default:     "100Mi",
		Small:       "100Mi",
		Medium:      "100Mi",
		Large:       "100Mi",
		XLarge:      "100Mi",
		TwoXLarge:   "100Mi",
		FourXLarge:  "100Mi",
		EightXLarge: "100Mi",
	}

	GrafanaCPURequest ResourceSizeMap = map[observabilityv1beta2.TShirtSize]string{
		Minimal:     "4m",
		Default:     "4m",
		Small:       "4m",
		Medium:      "4m",
		Large:       "4m",
		XLarge:      "4m",
		TwoXLarge:   "4m",
		FourXLarge:  "4m",
		EightXLarge: "4m",
	}
	GrafanaMemoryRequest ResourceSizeMap = map[observabilityv1beta2.TShirtSize]string{
		Minimal:     "100Mi",
		Default:     "100Mi",
		Small:       "100Mi",
		Medium:      "100Mi",
		Large:       "100Mi",
		XLarge:      "100Mi",
		TwoXLarge:   "100Mi",
		FourXLarge:  "100Mi",
		EightXLarge: "100Mi",
	}

	AlertmanagerCPURequest ResourceSizeMap = map[observabilityv1beta2.TShirtSize]string{
		Minimal:     "4m",
		Default:     "4m",
		Small:       "4m",
		Medium:      "4m",
		Large:       "4m",
		XLarge:      "4m",
		TwoXLarge:   "4m",
		FourXLarge:  "4m",
		EightXLarge: "4m",
	}
	AlertmanagerMemoryRequest ResourceSizeMap = map[observabilityv1beta2.TShirtSize]string{
		Minimal:     "200Mi",
		Default:     "200Mi",
		Small:       "200Mi",
		Medium:      "200Mi",
		Large:       "200Mi",
		XLarge:      "200Mi",
		TwoXLarge:   "200Mi",
		FourXLarge:  "200Mi",
		EightXLarge: "200Mi",
	}

	ObservatoriumAPICPURequest ResourceSizeMap = map[observabilityv1beta2.TShirtSize]string{
		Minimal:     "20m",
		Default:     "20m",
		Small:       "20m",
		Medium:      "20m",
		Large:       "20m",
		XLarge:      "20m",
		TwoXLarge:   "20m",
		FourXLarge:  "20m",
		EightXLarge: "20m",
	}
	ObservatoriumAPIMemoryRequest ResourceSizeMap = map[observabilityv1beta2.TShirtSize]string{
		Minimal:     "128Mi",
		Default:     "128Mi",
		Small:       "128Mi",
		Medium:      "128Mi",
		Large:       "128Mi",
		XLarge:      "128Mi",
		TwoXLarge:   "128Mi",
		FourXLarge:  "128Mi",
		EightXLarge: "128Mi",
	}

	ThanosQueryFrontendCPURequest ResourceSizeMap = map[observabilityv1beta2.TShirtSize]string{
		Minimal:     "100m",
		Default:     "100m",
		Small:       "100m",
		Medium:      "100m",
		Large:       "100m",
		XLarge:      "100m",
		TwoXLarge:   "100m",
		FourXLarge:  "100m",
		EightXLarge: "100m",
	}
	ThanosQueryFrontendMemoryRequest ResourceSizeMap = map[observabilityv1beta2.TShirtSize]string{
		Minimal:     "256Mi",
		Default:     "256Mi",
		Small:       "256Mi",
		Medium:      "256Mi",
		Large:       "256Mi",
		XLarge:      "256Mi",
		TwoXLarge:   "256Mi",
		FourXLarge:  "256Mi",
		EightXLarge: "256Mi",
	}

	MemcachedExporterCPURequest ResourceSizeMap = map[observabilityv1beta2.TShirtSize]string{
		Minimal:     "20m",
		Default:     "20m",
		Small:       "5m",
		Medium:      "5m",
		Large:       "5m",
		XLarge:      "5m",
		TwoXLarge:   "5m",
		FourXLarge:  "5m",
		EightXLarge: "5m",
	}
	MemcachedExporterMemoryRequest ResourceSizeMap = map[observabilityv1beta2.TShirtSize]string{
		Minimal:     "50Mi",
		Default:     "50Mi",
		Small:       "50Mi",
		Medium:      "50Mi",
		Large:       "50Mi",
		XLarge:      "50Mi",
		TwoXLarge:   "50Mi",
		FourXLarge:  "50Mi",
		EightXLarge: "50Mi",
	}

	ThanosQueryCPURequest ResourceSizeMap = map[observabilityv1beta2.TShirtSize]string{
		Minimal:     "300m",
		Default:     "300m",
		Small:       "300m",
		Medium:      "300m",
		Large:       "300m",
		XLarge:      "300m",
		TwoXLarge:   "300m",
		FourXLarge:  "300m",
		EightXLarge: "300m",
	}
	ThanosQueryMemoryRequest ResourceSizeMap = map[observabilityv1beta2.TShirtSize]string{
		Minimal:     "1Gi",
		Default:     "1Gi",
		Small:       "1Gi",
		Medium:      "1Gi",
		Large:       "1Gi",
		XLarge:      "1Gi",
		TwoXLarge:   "1Gi",
		FourXLarge:  "1Gi",
		EightXLarge: "1Gi",
	}

	ThanosCompactCPURequest ResourceSizeMap = map[observabilityv1beta2.TShirtSize]string{
		Minimal:     "100m",
		Default:     "100m",
		Small:       "100m",
		Medium:      "100m",
		Large:       "100m",
		XLarge:      "100m",
		TwoXLarge:   "100m",
		FourXLarge:  "100m",
		EightXLarge: "100m",
	}
	ThanosCompactMemoryRequest ResourceSizeMap = map[observabilityv1beta2.TShirtSize]string{
		Minimal:     "512Mi",
		Default:     "512Mi",
		Small:       "512Mi",
		Medium:      "512Mi",
		Large:       "512Mi",
		XLarge:      "512Mi",
		TwoXLarge:   "512Mi",
		FourXLarge:  "512Mi",
		EightXLarge: "512Mi",
	}

	ObservatoriumReceiveControllerCPURequest ResourceSizeMap = map[observabilityv1beta2.TShirtSize]string{
		Minimal:     "4m",
		Default:     "4m",
		Small:       "4m",
		Medium:      "4m",
		Large:       "4m",
		XLarge:      "4m",
		TwoXLarge:   "4m",
		FourXLarge:  "4m",
		EightXLarge: "4m",
	}
	ObservatoriumReceiveControllerMemoryRequest ResourceSizeMap = map[observabilityv1beta2.TShirtSize]string{
		Minimal:     "32Mi",
		Default:     "32Mi",
		Small:       "32Mi",
		Medium:      "32Mi",
		Large:       "32Mi",
		XLarge:      "32Mi",
		TwoXLarge:   "32Mi",
		FourXLarge:  "32Mi",
		EightXLarge: "32Mi",
	}

	ThanosReceiveCPURequest ResourceSizeMap = map[observabilityv1beta2.TShirtSize]string{
		Minimal:     "300m",
		Default:     "300m",
		Small:       "300m",
		Medium:      "300m",
		Large:       "300m",
		XLarge:      "300m",
		TwoXLarge:   "300m",
		FourXLarge:  "300m",
		EightXLarge: "300m",
	}
	ThanosReceiveMemoryRequest ResourceSizeMap = map[observabilityv1beta2.TShirtSize]string{
		Minimal:     "512Mi",
		Default:     "512Mi",
		Small:       "512Mi",
		Medium:      "512Mi",
		Large:       "512Mi",
		XLarge:      "512Mi",
		TwoXLarge:   "512Mi",
		FourXLarge:  "512Mi",
		EightXLarge: "512Mi",
	}

	ThanosRuleCPURequest ResourceSizeMap = map[observabilityv1beta2.TShirtSize]string{
		Minimal:     "50m",
		Default:     "50m",
		Small:       "50m",
		Medium:      "50m",
		Large:       "50m",
		XLarge:      "50m",
		TwoXLarge:   "50m",
		FourXLarge:  "50m",
		EightXLarge: "50m",
	}
	ThanosRuleMemoryRequest ResourceSizeMap = map[observabilityv1beta2.TShirtSize]string{
		Minimal:     "512Mi",
		Default:     "512Mi",
		Small:       "512Mi",
		Medium:      "512Mi",
		Large:       "512Mi",
		XLarge:      "512Mi",
		TwoXLarge:   "512Mi",
		FourXLarge:  "512Mi",
		EightXLarge: "512Mi",
	}

	ThanosRuleReloaderCPURequest ResourceSizeMap = map[observabilityv1beta2.TShirtSize]string{
		Minimal:     "4m",
		Default:     "4m",
		Small:       "4m",
		Medium:      "4m",
		Large:       "4m",
		XLarge:      "4m",
		TwoXLarge:   "4m",
		FourXLarge:  "4m",
		EightXLarge: "4m",
	}
	ThanosRuleReloaderMemoryRequest ResourceSizeMap = map[observabilityv1beta2.TShirtSize]string{
		Minimal:     "25Mi",
		Default:     "25Mi",
		Small:       "25Mi",
		Medium:      "25Mi",
		Large:       "25Mi",
		XLarge:      "25Mi",
		TwoXLarge:   "25Mi",
		FourXLarge:  "25Mi",
		EightXLarge: "25Mi",
	}

	ThanosCachedCPURequest ResourceSizeMap = map[observabilityv1beta2.TShirtSize]string{
		Minimal:     "45m",
		Default:     "45m",
		Small:       "45m",
		Medium:      "45m",
		Large:       "45m",
		XLarge:      "45m",
		TwoXLarge:   "45m",
		FourXLarge:  "45m",
		EightXLarge: "45m",
	}
	ThanosCachedMemoryRequest ResourceSizeMap = map[observabilityv1beta2.TShirtSize]string{
		Minimal:     "128Mi",
		Default:     "128Mi",
		Small:       "128Mi",
		Medium:      "128Mi",
		Large:       "128Mi",
		XLarge:      "128Mi",
		TwoXLarge:   "128Mi",
		FourXLarge:  "128Mi",
		EightXLarge: "128Mi",
	}

	ThanosCachedExporterCPURequest ResourceSizeMap = map[observabilityv1beta2.TShirtSize]string{
		Minimal:     "5m",
		Default:     "5m",
		Small:       "5m",
		Medium:      "5m",
		Large:       "5m",
		XLarge:      "5m",
		TwoXLarge:   "5m",
		FourXLarge:  "5m",
		EightXLarge: "5m",
	}
	ThanosCachedExporterMemoryRequest ResourceSizeMap = map[observabilityv1beta2.TShirtSize]string{
		Minimal:     "50Mi",
		Default:     "50Mi",
		Small:       "50Mi",
		Medium:      "50Mi",
		Large:       "50Mi",
		XLarge:      "50Mi",
		TwoXLarge:   "50Mi",
		FourXLarge:  "50Mi",
		EightXLarge: "50Mi",
	}

	ThanosStoreCPURequest ResourceSizeMap = map[observabilityv1beta2.TShirtSize]string{
		Minimal:     "100m",
		Default:     "100m",
		Small:       "100m",
		Medium:      "100m",
		Large:       "100m",
		XLarge:      "100m",
		TwoXLarge:   "100m",
		FourXLarge:  "100m",
		EightXLarge: "100m",
	}
	ThanosStoreMemoryRequest ResourceSizeMap = map[observabilityv1beta2.TShirtSize]string{
		Minimal:     "1Gi",
		Default:     "1Gi",
		Small:       "1Gi",
		Medium:      "1Gi",
		Large:       "1Gi",
		XLarge:      "1Gi",
		TwoXLarge:   "1Gi",
		FourXLarge:  "1Gi",
		EightXLarge: "1Gi",
	}

	MetricsCollectorCPURequest ResourceSizeMap = map[observabilityv1beta2.TShirtSize]string{
		Minimal:     "10m",
		Default:     "10m",
		Small:       "10m",
		Medium:      "10m",
		Large:       "10m",
		XLarge:      "10m",
		TwoXLarge:   "10m",
		FourXLarge:  "10m",
		EightXLarge: "10m",
	}
	MetricsCollectorMemoryRequest ResourceSizeMap = map[observabilityv1beta2.TShirtSize]string{
		Minimal:     "100Mi",
		Default:     "100Mi",
		Small:       "100Mi",
		Medium:      "100Mi",
		Large:       "100Mi",
		XLarge:      "100Mi",
		TwoXLarge:   "100Mi",
		FourXLarge:  "100Mi",
		EightXLarge: "100Mi",
	}
)

// TODO(saswatamcode): Add tshirt sized limits for all components.
const (
	GrafanaCPULimit    = "500m"
	GrafanaMemoryLimit = "1Gi"

	MetricsCollectorCPULimits    = ""
	MetricsCollectorMemoryLimits = ""
)

type ReplicaMap map[observabilityv1beta2.TShirtSize]*int32

// Specifies replicas for all components.
// TODO(saswatamcode): Figure out the right values for these. They are all the same at the moment.
var (
	Replicas1 int32 = 1
	Replicas2 int32 = 2
	Replicas3 int32 = 3

	Replicas = map[string]ReplicaMap{
		ObservatoriumAPI: {
			Minimal:     &Replicas2,
			Default:     &Replicas2,
			Small:       &Replicas2,
			Medium:      &Replicas2,
			Large:       &Replicas2,
			XLarge:      &Replicas2,
			TwoXLarge:   &Replicas2,
			FourXLarge:  &Replicas2,
			EightXLarge: &Replicas2,
		},
		ThanosQuery: {
			Minimal:     &Replicas2,
			Default:     &Replicas2,
			Small:       &Replicas2,
			Medium:      &Replicas2,
			Large:       &Replicas2,
			XLarge:      &Replicas2,
			TwoXLarge:   &Replicas2,
			FourXLarge:  &Replicas2,
			EightXLarge: &Replicas2,
		},
		ThanosQueryFrontend: {
			Minimal:     &Replicas2,
			Default:     &Replicas2,
			Small:       &Replicas2,
			Medium:      &Replicas2,
			Large:       &Replicas2,
			XLarge:      &Replicas2,
			TwoXLarge:   &Replicas2,
			FourXLarge:  &Replicas2,
			EightXLarge: &Replicas2,
		},
		Grafana: {
			Minimal:     &Replicas2,
			Default:     &Replicas2,
			Small:       &Replicas2,
			Medium:      &Replicas2,
			Large:       &Replicas2,
			XLarge:      &Replicas2,
			TwoXLarge:   &Replicas2,
			FourXLarge:  &Replicas2,
			EightXLarge: &Replicas2,
		},
		RBACQueryProxy: {
			Minimal:     &Replicas2,
			Default:     &Replicas2,
			Small:       &Replicas2,
			Medium:      &Replicas2,
			Large:       &Replicas2,
			XLarge:      &Replicas2,
			TwoXLarge:   &Replicas2,
			FourXLarge:  &Replicas2,
			EightXLarge: &Replicas2,
		},

		ThanosRule: {
			Minimal:     &Replicas3,
			Default:     &Replicas3,
			Small:       &Replicas3,
			Medium:      &Replicas3,
			Large:       &Replicas3,
			XLarge:      &Replicas3,
			TwoXLarge:   &Replicas3,
			FourXLarge:  &Replicas3,
			EightXLarge: &Replicas3,
		},
		ThanosReceive: {
			Minimal:     &Replicas3,
			Default:     &Replicas3,
			Small:       &Replicas3,
			Medium:      &Replicas3,
			Large:       &Replicas3,
			XLarge:      &Replicas3,
			TwoXLarge:   &Replicas3,
			FourXLarge:  &Replicas3,
			EightXLarge: &Replicas3,
		},
		ThanosStoreShard: {
			Minimal:     &Replicas3,
			Default:     &Replicas3,
			Small:       &Replicas3,
			Medium:      &Replicas3,
			Large:       &Replicas3,
			XLarge:      &Replicas3,
			TwoXLarge:   &Replicas3,
			FourXLarge:  &Replicas3,
			EightXLarge: &Replicas3,
		},
		ThanosStoreMemcached: {
			Minimal:     &Replicas3,
			Default:     &Replicas3,
			Small:       &Replicas3,
			Medium:      &Replicas3,
			Large:       &Replicas3,
			XLarge:      &Replicas3,
			TwoXLarge:   &Replicas3,
			FourXLarge:  &Replicas3,
			EightXLarge: &Replicas3,
		},
		ThanosQueryFrontendMemcached: {
			Minimal:     &Replicas3,
			Default:     &Replicas3,
			Small:       &Replicas3,
			Medium:      &Replicas3,
			Large:       &Replicas3,
			XLarge:      &Replicas3,
			TwoXLarge:   &Replicas3,
			FourXLarge:  &Replicas3,
			EightXLarge: &Replicas3,
		},
		Alertmanager: {
			Minimal:     &Replicas3,
			Default:     &Replicas3,
			Small:       &Replicas3,
			Medium:      &Replicas3,
			Large:       &Replicas3,
			XLarge:      &Replicas3,
			TwoXLarge:   &Replicas3,
			FourXLarge:  &Replicas3,
			EightXLarge: &Replicas3,
		},
	}
)
