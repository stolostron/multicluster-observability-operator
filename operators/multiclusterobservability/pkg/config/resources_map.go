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
		Minimal:     "500m",
		Default:     "100m",
		Small:       "1",
		Medium:      "1",
		Large:       "2",
		XLarge:      "2",
		TwoXLarge:   "4",
		FourXLarge:  "4",
		EightXLarge: "6",
	}
	ThanosQueryFrontendMemoryRequest ResourceSizeMap = map[observabilityv1beta2.TShirtSize]string{
		Minimal:     "256Mi",
		Default:     "256Mi",
		Small:       "500Mi",
		Medium:      "2Gi",
		Large:       "5Gi",
		XLarge:      "8Gi",
		TwoXLarge:   "10Gi",
		FourXLarge:  "12Gi",
		EightXLarge: "15Gi",
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
		Minimal:     "1",
		Default:     "300m",
		Small:       "1500m",
		Medium:      "2",
		Large:       "4",
		XLarge:      "6",
		TwoXLarge:   "6",
		FourXLarge:  "7",
		EightXLarge: "7",
	}
	ThanosQueryMemoryRequest ResourceSizeMap = map[observabilityv1beta2.TShirtSize]string{
		Minimal:     "1Gi",
		Default:     "1Gi",
		Small:       "4Gi",
		Medium:      "6Gi",
		Large:       "8Gi",
		XLarge:      "10Gi",
		TwoXLarge:   "15Gi",
		FourXLarge:  "18Gi",
		EightXLarge: "20Gi",
	}

	ThanosCompactCPURequest ResourceSizeMap = map[observabilityv1beta2.TShirtSize]string{
		Minimal:     "250m",
		Default:     "100m",
		Small:       "500m",
		Medium:      "1",
		Large:       "3",
		XLarge:      "3",
		TwoXLarge:   "4",
		FourXLarge:  "6",
		EightXLarge: "8",
	}
	ThanosCompactMemoryRequest ResourceSizeMap = map[observabilityv1beta2.TShirtSize]string{
		Minimal:     "512Mi",
		Default:     "512Mi",
		Small:       "1Gi",
		Medium:      "2Gi",
		Large:       "4Gi",
		XLarge:      "8Gi",
		TwoXLarge:   "12Gi",
		FourXLarge:  "18Gi",
		EightXLarge: "24Gi",
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
		Minimal:     "500m",
		Default:     "300m",
		Small:       "2",
		Medium:      "4",
		Large:       "5",
		XLarge:      "5",
		TwoXLarge:   "6",
		FourXLarge:  "10",
		EightXLarge: "20",
	}
	ThanosReceiveMemoryRequest ResourceSizeMap = map[observabilityv1beta2.TShirtSize]string{
		Minimal:     "2Gi",
		Default:     "512Mi",
		Small:       "6Gi",
		Medium:      "12Gi",
		Large:       "24Gi",
		XLarge:      "36Gi",
		TwoXLarge:   "52Gi",
		FourXLarge:  "128Gi",
		EightXLarge: "220Gi",
	}

	ThanosRuleCPURequest ResourceSizeMap = map[observabilityv1beta2.TShirtSize]string{
		Minimal:     "250m",
		Default:     "50m",
		Small:       "500m",
		Medium:      "1",
		Large:       "3",
		XLarge:      "3",
		TwoXLarge:   "4",
		FourXLarge:  "6",
		EightXLarge: "6",
	}
	ThanosRuleMemoryRequest ResourceSizeMap = map[observabilityv1beta2.TShirtSize]string{
		Minimal:     "512Mi",
		Default:     "512Mi",
		Small:       "1Gi",
		Medium:      "2Gi",
		Large:       "4Gi",
		XLarge:      "6Gi",
		TwoXLarge:   "10Gi",
		FourXLarge:  "15Gi",
		EightXLarge: "18Gi",
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
		Minimal:     "2",
		Default:     "100m",
		Small:       "2",
		Medium:      "3",
		Large:       "3",
		XLarge:      "3",
		TwoXLarge:   "4",
		FourXLarge:  "6",
		EightXLarge: "8",
	}
	ThanosStoreMemoryRequest ResourceSizeMap = map[observabilityv1beta2.TShirtSize]string{
		Minimal:     "1Gi",
		Default:     "1Gi",
		Small:       "4Gi",
		Medium:      "6Gi",
		Large:       "8Gi",
		XLarge:      "12Gi",
		TwoXLarge:   "15Gi",
		FourXLarge:  "20Gi",
		EightXLarge: "36Gi",
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

func intptr(i int32) *int32 {
	return &i
}

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
			Minimal:     intptr(2),
			Default:     intptr(2),
			Small:       intptr(3),
			Medium:      intptr(4),
			Large:       intptr(6),
			XLarge:      intptr(8),
			TwoXLarge:   intptr(8),
			FourXLarge:  intptr(10),
			EightXLarge: intptr(10),
		},
		ThanosQueryFrontend: {
			Minimal:     intptr(2),
			Default:     intptr(2),
			Small:       intptr(3),
			Medium:      intptr(3),
			Large:       intptr(3),
			XLarge:      intptr(6),
			TwoXLarge:   intptr(8),
			FourXLarge:  intptr(10),
			EightXLarge: intptr(10),
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
			Minimal:     intptr(6),
			Default:     &Replicas3,
			Small:       intptr(6),
			Medium:      intptr(6),
			Large:       intptr(6),
			XLarge:      intptr(9),
			TwoXLarge:   intptr(12),
			FourXLarge:  intptr(12),
			EightXLarge: intptr(21),
		},
		ThanosStoreShard: {
			Minimal:     intptr(3),
			Default:     intptr(3),
			Small:       intptr(3),
			Medium:      intptr(3),
			Large:       intptr(6),
			XLarge:      intptr(6),
			TwoXLarge:   intptr(6),
			FourXLarge:  intptr(6),
			EightXLarge: intptr(12),
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
