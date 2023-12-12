// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package config

// T Shirt size class for a particular o11y resource.
type TShirtSize string

const (
	Small       TShirtSize = "small"
	Medium      TShirtSize = "medium"
	Large       TShirtSize = "large"
	XLarge      TShirtSize = "xlarge"
	TwoXLarge   TShirtSize = "2xlarge"
	FourXLarge  TShirtSize = "4xlarge"
	EightXLarge TShirtSize = "8xlarge"
)

type ResourceSizeMap map[TShirtSize]string

// Specifies resources for all components and their respective TShirt sizes.
// TODO(saswatamcode): Figure out the right values for these. They are all the same at the moment.
var (
	RBACQueryProxyCPURequest ResourceSizeMap = map[TShirtSize]string{
		Small:       "20m",
		Medium:      "20m",
		Large:       "20m",
		XLarge:      "20m",
		TwoXLarge:   "20m",
		FourXLarge:  "20m",
		EightXLarge: "20m",
	}
	RBACQueryProxyMemoryRequest ResourceSizeMap = map[TShirtSize]string{
		Small:       "100Mi",
		Medium:      "100Mi",
		Large:       "100Mi",
		XLarge:      "100Mi",
		TwoXLarge:   "100Mi",
		FourXLarge:  "100Mi",
		EightXLarge: "100Mi",
	}

	GrafanaCPURequest ResourceSizeMap = map[TShirtSize]string{
		Small:       "4m",
		Medium:      "4m",
		Large:       "4m",
		XLarge:      "4m",
		TwoXLarge:   "4m",
		FourXLarge:  "4m",
		EightXLarge: "4m",
	}
	GrafanaMemoryRequest ResourceSizeMap = map[TShirtSize]string{
		Small:       "100Mi",
		Medium:      "100Mi",
		Large:       "100Mi",
		XLarge:      "100Mi",
		TwoXLarge:   "100Mi",
		FourXLarge:  "100Mi",
		EightXLarge: "100Mi",
	}

	AlertmanagerCPURequest ResourceSizeMap = map[TShirtSize]string{
		Small:       "4m",
		Medium:      "4m",
		Large:       "4m",
		XLarge:      "4m",
		TwoXLarge:   "4m",
		FourXLarge:  "4m",
		EightXLarge: "4m",
	}
	AlertmanagerMemoryRequest ResourceSizeMap = map[TShirtSize]string{
		Small:       "200Mi",
		Medium:      "200Mi",
		Large:       "200Mi",
		XLarge:      "200Mi",
		TwoXLarge:   "200Mi",
		FourXLarge:  "200Mi",
		EightXLarge: "200Mi",
	}

	ObservatoriumAPICPURequest ResourceSizeMap = map[TShirtSize]string{
		Small:       "20m",
		Medium:      "20m",
		Large:       "20m",
		XLarge:      "20m",
		TwoXLarge:   "20m",
		FourXLarge:  "20m",
		EightXLarge: "20m",
	}
	ObservatoriumAPIMemoryRequest ResourceSizeMap = map[TShirtSize]string{
		Small:       "128Mi",
		Medium:      "128Mi",
		Large:       "128Mi",
		XLarge:      "128Mi",
		TwoXLarge:   "128Mi",
		FourXLarge:  "128Mi",
		EightXLarge: "128Mi",
	}

	ThanosQueryFrontendCPURequest ResourceSizeMap = map[TShirtSize]string{
		Small:       "100m",
		Medium:      "100m",
		Large:       "100m",
		XLarge:      "100m",
		TwoXLarge:   "100m",
		FourXLarge:  "100m",
		EightXLarge: "100m",
	}
	ThanosQueryFrontendMemoryRequest ResourceSizeMap = map[TShirtSize]string{
		Small:       "256Mi",
		Medium:      "256Mi",
		Large:       "256Mi",
		XLarge:      "256Mi",
		TwoXLarge:   "256Mi",
		FourXLarge:  "256Mi",
		EightXLarge: "256Mi",
	}

	MemcachedExporterCPURequest ResourceSizeMap = map[TShirtSize]string{
		Small:       "5m",
		Medium:      "5m",
		Large:       "5m",
		XLarge:      "5m",
		TwoXLarge:   "5m",
		FourXLarge:  "5m",
		EightXLarge: "5m",
	}
	MemcachedExporterMemoryRequest ResourceSizeMap = map[TShirtSize]string{
		Small:       "50Mi",
		Medium:      "50Mi",
		Large:       "50Mi",
		XLarge:      "50Mi",
		TwoXLarge:   "50Mi",
		FourXLarge:  "50Mi",
		EightXLarge: "50Mi",
	}

	ThanosQueryCPURequest ResourceSizeMap = map[TShirtSize]string{
		Small:       "300m",
		Medium:      "300m",
		Large:       "300m",
		XLarge:      "300m",
		TwoXLarge:   "300m",
		FourXLarge:  "300m",
		EightXLarge: "300m",
	}
	ThanosQueryMemoryRequest ResourceSizeMap = map[TShirtSize]string{
		Small:       "1Gi",
		Medium:      "1Gi",
		Large:       "1Gi",
		XLarge:      "1Gi",
		TwoXLarge:   "1Gi",
		FourXLarge:  "1Gi",
		EightXLarge: "1Gi",
	}

	ThanosCompactCPURequest ResourceSizeMap = map[TShirtSize]string{
		Small:       "100m",
		Medium:      "100m",
		Large:       "100m",
		XLarge:      "100m",
		TwoXLarge:   "100m",
		FourXLarge:  "100m",
		EightXLarge: "100m",
	}
	ThanosCompactMemoryRequest ResourceSizeMap = map[TShirtSize]string{
		Small:       "512Mi",
		Medium:      "512Mi",
		Large:       "512Mi",
		XLarge:      "512Mi",
		TwoXLarge:   "512Mi",
		FourXLarge:  "512Mi",
		EightXLarge: "512Mi",
	}

	ObservatoriumReceiveControllerCPURequest ResourceSizeMap = map[TShirtSize]string{
		Small:       "4m",
		Medium:      "4m",
		Large:       "4m",
		XLarge:      "4m",
		TwoXLarge:   "4m",
		FourXLarge:  "4m",
		EightXLarge: "4m",
	}
	ObservatoriumReceiveControllerMemoryRequest ResourceSizeMap = map[TShirtSize]string{
		Small:       "32Mi",
		Medium:      "32Mi",
		Large:       "32Mi",
		XLarge:      "32Mi",
		TwoXLarge:   "32Mi",
		FourXLarge:  "32Mi",
		EightXLarge: "32Mi",
	}

	ThanosReceiveCPURequest ResourceSizeMap = map[TShirtSize]string{
		Small:       "300m",
		Medium:      "300m",
		Large:       "300m",
		XLarge:      "300m",
		TwoXLarge:   "300m",
		FourXLarge:  "300m",
		EightXLarge: "300m",
	}
	ThanosReceiveMemoryRequest ResourceSizeMap = map[TShirtSize]string{
		Small:       "512Mi",
		Medium:      "512Mi",
		Large:       "512Mi",
		XLarge:      "512Mi",
		TwoXLarge:   "512Mi",
		FourXLarge:  "512Mi",
		EightXLarge: "512Mi",
	}

	ThanosRuleCPURequest ResourceSizeMap = map[TShirtSize]string{
		Small:       "50m",
		Medium:      "50m",
		Large:       "50m",
		XLarge:      "50m",
		TwoXLarge:   "50m",
		FourXLarge:  "50m",
		EightXLarge: "50m",
	}
	ThanosRuleMemoryRequest ResourceSizeMap = map[TShirtSize]string{
		Small:       "512Mi",
		Medium:      "512Mi",
		Large:       "512Mi",
		XLarge:      "512Mi",
		TwoXLarge:   "512Mi",
		FourXLarge:  "512Mi",
		EightXLarge: "512Mi",
	}

	ThanosRuleReloaderCPURequest ResourceSizeMap = map[TShirtSize]string{
		Small:       "4m",
		Medium:      "4m",
		Large:       "4m",
		XLarge:      "4m",
		TwoXLarge:   "4m",
		FourXLarge:  "4m",
		EightXLarge: "4m",
	}
	ThanosRuleReloaderMemoryRequest ResourceSizeMap = map[TShirtSize]string{
		Small:       "25Mi",
		Medium:      "25Mi",
		Large:       "25Mi",
		XLarge:      "25Mi",
		TwoXLarge:   "25Mi",
		FourXLarge:  "25Mi",
		EightXLarge: "25Mi",
	}

	ThanosCachedCPURequest ResourceSizeMap = map[TShirtSize]string{
		Small:       "45m",
		Medium:      "45m",
		Large:       "45m",
		XLarge:      "45m",
		TwoXLarge:   "45m",
		FourXLarge:  "45m",
		EightXLarge: "45m",
	}
	ThanosCachedMemoryRequest ResourceSizeMap = map[TShirtSize]string{
		Small:       "128Mi",
		Medium:      "128Mi",
		Large:       "128Mi",
		XLarge:      "128Mi",
		TwoXLarge:   "128Mi",
		FourXLarge:  "128Mi",
		EightXLarge: "128Mi",
	}

	ThanosCachedExporterCPURequest ResourceSizeMap = map[TShirtSize]string{
		Small:       "5m",
		Medium:      "5m",
		Large:       "5m",
		XLarge:      "5m",
		TwoXLarge:   "5m",
		FourXLarge:  "5m",
		EightXLarge: "5m",
	}
	ThanosCachedExporterMemoryRequest ResourceSizeMap = map[TShirtSize]string{
		Small:       "50Mi",
		Medium:      "50Mi",
		Large:       "50Mi",
		XLarge:      "50Mi",
		TwoXLarge:   "50Mi",
		FourXLarge:  "50Mi",
		EightXLarge: "50Mi",
	}

	ThanosStoreCPURequest ResourceSizeMap = map[TShirtSize]string{
		Small:       "100m",
		Medium:      "100m",
		Large:       "100m",
		XLarge:      "100m",
		TwoXLarge:   "100m",
		FourXLarge:  "100m",
		EightXLarge: "100m",
	}
	ThanosStoreMemoryRequest ResourceSizeMap = map[TShirtSize]string{
		Small:       "1Gi",
		Medium:      "1Gi",
		Large:       "1Gi",
		XLarge:      "1Gi",
		TwoXLarge:   "1Gi",
		FourXLarge:  "1Gi",
		EightXLarge: "1Gi",
	}

	MetricsCollectorCPURequest ResourceSizeMap = map[TShirtSize]string{
		Small:       "10m",
		Medium:      "10m",
		Large:       "10m",
		XLarge:      "10m",
		TwoXLarge:   "10m",
		FourXLarge:  "10m",
		EightXLarge: "10m",
	}
	MetricsCollectorMemoryRequest ResourceSizeMap = map[TShirtSize]string{
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

type ReplicaMap map[TShirtSize]*int32

// Specifies replicas for all components.
// TODO(saswatamcode): Figure out the right values for these. They are all the same at the moment.
var (
	Replicas1 int32 = 1
	Replicas2 int32 = 2
	Replicas3 int32 = 3

	Replicas = map[string]ReplicaMap{
		ObservatoriumAPI: {
			Small:       &Replicas2,
			Medium:      &Replicas2,
			Large:       &Replicas2,
			XLarge:      &Replicas2,
			TwoXLarge:   &Replicas2,
			FourXLarge:  &Replicas2,
			EightXLarge: &Replicas2,
		},
		ThanosQuery: {
			Small:       &Replicas2,
			Medium:      &Replicas2,
			Large:       &Replicas2,
			XLarge:      &Replicas2,
			TwoXLarge:   &Replicas2,
			FourXLarge:  &Replicas2,
			EightXLarge: &Replicas2,
		},
		ThanosQueryFrontend: {
			Small:       &Replicas2,
			Medium:      &Replicas2,
			Large:       &Replicas2,
			XLarge:      &Replicas2,
			TwoXLarge:   &Replicas2,
			FourXLarge:  &Replicas2,
			EightXLarge: &Replicas2,
		},
		Grafana: {
			Small:       &Replicas2,
			Medium:      &Replicas2,
			Large:       &Replicas2,
			XLarge:      &Replicas2,
			TwoXLarge:   &Replicas2,
			FourXLarge:  &Replicas2,
			EightXLarge: &Replicas2,
		},
		RBACQueryProxy: {
			Small:       &Replicas2,
			Medium:      &Replicas2,
			Large:       &Replicas2,
			XLarge:      &Replicas2,
			TwoXLarge:   &Replicas2,
			FourXLarge:  &Replicas2,
			EightXLarge: &Replicas2,
		},

		ThanosRule: {
			Small:       &Replicas3,
			Medium:      &Replicas3,
			Large:       &Replicas3,
			XLarge:      &Replicas3,
			TwoXLarge:   &Replicas3,
			FourXLarge:  &Replicas3,
			EightXLarge: &Replicas3,
		},
		ThanosReceive: {
			Small:       &Replicas3,
			Medium:      &Replicas3,
			Large:       &Replicas3,
			XLarge:      &Replicas3,
			TwoXLarge:   &Replicas3,
			FourXLarge:  &Replicas3,
			EightXLarge: &Replicas3,
		},
		ThanosStoreShard: {
			Small:       &Replicas3,
			Medium:      &Replicas3,
			Large:       &Replicas3,
			XLarge:      &Replicas3,
			TwoXLarge:   &Replicas3,
			FourXLarge:  &Replicas3,
			EightXLarge: &Replicas3,
		},
		ThanosStoreMemcached: {
			Small:       &Replicas3,
			Medium:      &Replicas3,
			Large:       &Replicas3,
			XLarge:      &Replicas3,
			TwoXLarge:   &Replicas3,
			FourXLarge:  &Replicas3,
			EightXLarge: &Replicas3,
		},
		ThanosQueryFrontendMemcached: {
			Small:       &Replicas3,
			Medium:      &Replicas3,
			Large:       &Replicas3,
			XLarge:      &Replicas3,
			TwoXLarge:   &Replicas3,
			FourXLarge:  &Replicas3,
			EightXLarge: &Replicas3,
		},
		Alertmanager: {
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
