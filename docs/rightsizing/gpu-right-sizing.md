# GPU Right-Sizing (recording rules)

This document describes how **GPU right-sizing** metrics are produced in MCO via **Prometheus recording rules**, which **base metrics** are used, what the **PrometheusRule** contains, and the end-to-end **working flow**.

## Overview

When enabled, the MCO analytics right-sizing controller generates a `PrometheusRule` that records GPU-related time series at multiple scopes:

- **namespace**
- **pod**
- **workload**
- **cluster**

It records “raw” 5m-window maxima and 1d rollups, and produces recommendations as a percentage of the max usage over the last day.

## Base metrics used (inputs)

GPU right-sizing recording rules are derived from these base metrics/series:

- **GPU requests (kube-state-metrics)**
  - `kube_pod_container_resource_requests{resource=~"nvidia.com/gpu|amd.com/gpu",container!=""}`
- **Kubernetes ownership / workload mapping (kube-state-metrics)** (used to attribute pod GPU metrics to workloads)
  - `kube_pod_owner`
  - `kube_replicaset_owner`
  - `kube_job_owner`
- **GPU utilization + device stats** (GPU exporter / device plugin integration; names as used by the rule generator)
  - `accelerator_gpu_utilization`
  - `accelerator_memory_used_bytes`
  - `accelerator_memory_total_bytes`
  - `accelerator_power_usage_watts`
  - `accelerator_temperature_celsius`
  - `accelerator_sm_clock_hertz`
  - `accelerator_memory_clock_hertz`

## Generated PrometheusRule

- **PrometheusRule name**: `acm-rs-gpu-prometheus-rules`
- **Namespace**: `openshift-monitoring`
- **Labels (required for OpenShift rule selection)**:
  - `prometheus: k8s`
  - `role: alert-rules`
- **Rule groups**:
  - `acm-right-sizing-gpu-namespace-5m.rules` (interval `5m`)
  - `acm-right-sizing-gpu-workload-5m.rules` (interval `5m`)
  - `acm-right-sizing-gpu-namespace-1d.rules` (interval `1h`)
  - `acm-right-sizing-gpu-workload-1d.rules` (interval `1h`)
  - `acm-right-sizing-gpu-cluster-5m.rules` (interval `5m`)
  - `acm-right-sizing-gpu-cluster-1d.rules` (interval `1h`)

## Prometheus rules (record → PromQL)

The expressions below are shown in their generated form, using placeholders:

- **`<NS_FILTER>`**: comes from `prometheusRuleConfig.namespaceFilterCriteria` (defaults to `namespace!~"openshift.*"`; if unset, it becomes `namespace!=""`).
- **`<LABEL_JOIN>`**: optional label-based join appended to *every* rule expression (currently supports filtering by `label_env` via `kube_namespace_labels`).

### Group: `acm-right-sizing-gpu-namespace-5m.rules`

- **`acm_rs:namespace:gpu_request:5m`**

```promql
max_over_time(sum by (namespace) (
  kube_pod_container_resource_requests{<NS_FILTER>, resource=~"nvidia.com/gpu|amd.com/gpu", container!=""}
)[5m:])
<LABEL_JOIN>
```

- **`acm_rs:namespace:gpu_usage:5m`**

```promql
max_over_time(sum by (namespace) (accelerator_gpu_utilization{<NS_FILTER>})[5m:])
<LABEL_JOIN>
```

- **`acm_rs:namespace:gpu_utilization:5m`**

```promql
max_over_time(max by (namespace) (accelerator_gpu_utilization{<NS_FILTER>})[5m:])
<LABEL_JOIN>
```

- **`acm_rs:namespace:gpu_memory_used:5m`**

```promql
max_over_time(sum by (namespace) (accelerator_memory_used_bytes{<NS_FILTER>})[5m:])
<LABEL_JOIN>
```

- **`acm_rs:namespace:gpu_memory_total:5m`**

```promql
max_over_time(sum by (namespace) (accelerator_memory_total_bytes{<NS_FILTER>})[5m:])
<LABEL_JOIN>
```

- **`acm_rs:namespace:gpu_power_usage_watts:5m`**

```promql
max_over_time(sum by (namespace) (accelerator_power_usage_watts{<NS_FILTER>})[5m:])
<LABEL_JOIN>
```

- **`acm_rs:namespace:gpu_temperature_celsius:5m`**

```promql
max_over_time(max by (namespace) (accelerator_temperature_celsius{<NS_FILTER>})[5m:])
<LABEL_JOIN>
```

- **`acm_rs:namespace:gpu_sm_clock_hertz:5m`**

```promql
max_over_time(max by (namespace) (accelerator_sm_clock_hertz{<NS_FILTER>})[5m:])
<LABEL_JOIN>
```

- **`acm_rs:namespace:gpu_memory_clock_hertz:5m`**

```promql
max_over_time(max by (namespace) (accelerator_memory_clock_hertz{<NS_FILTER>})[5m:])
<LABEL_JOIN>
```

### Group: `acm-right-sizing-gpu-workload-5m.rules`

#### Shared mapping (pod → workload/workload_type)

GPU right-sizing may include the shared mapping rule `acm_rs:pod_workload:relabel:5m`. To avoid duplicate definitions:

- If **workload+pod right-sizing** is enabled, the GPU PrometheusRule is generated **without** this mapping rule.
- If workload+pod right-sizing is disabled, the GPU PrometheusRule **includes** the mapping rule.

When present, the mapping expression matches the workload/pod document.

#### Pod series (GPU)

- **`acm_rs:pod:gpu_request:5m`**

```promql
max_over_time(sum by (namespace, pod, workload, workload_type) (
  kube_pod_container_resource_requests{<NS_FILTER>, resource=~"nvidia.com/gpu|amd.com/gpu", container!=""}
  * on (namespace, pod) group_left(workload, workload_type)
    acm_rs:pod_workload:relabel:5m
)[5m:])
<LABEL_JOIN>
```

- **`acm_rs:pod:gpu_usage:5m`**

```promql
max_over_time(sum by (namespace, pod, workload, workload_type) (
  accelerator_gpu_utilization{<NS_FILTER>}
  * on (namespace, pod) group_left(workload, workload_type)
    acm_rs:pod_workload:relabel:5m
)[5m:])
<LABEL_JOIN>
```

- **`acm_rs:pod:gpu_memory_used:5m`**

```promql
max_over_time(sum by (namespace, pod, workload, workload_type) (
  accelerator_memory_used_bytes{<NS_FILTER>}
  * on (namespace, pod) group_left(workload, workload_type)
    acm_rs:pod_workload:relabel:5m
)[5m:])
<LABEL_JOIN>
```

- **`acm_rs:pod:gpu_memory_total:5m`**

```promql
max_over_time(sum by (namespace, pod, workload, workload_type) (
  accelerator_memory_total_bytes{<NS_FILTER>}
  * on (namespace, pod) group_left(workload, workload_type)
    acm_rs:pod_workload:relabel:5m
)[5m:])
<LABEL_JOIN>
```

- **`acm_rs:pod:gpu_power_usage_watts:5m`**

```promql
max_over_time(sum by (namespace, pod, workload, workload_type) (
  accelerator_power_usage_watts{<NS_FILTER>}
  * on (namespace, pod) group_left(workload, workload_type)
    acm_rs:pod_workload:relabel:5m
)[5m:])
<LABEL_JOIN>
```

- **`acm_rs:pod:gpu_temperature_celsius:5m`**

```promql
max_over_time(max by (namespace, pod, workload, workload_type) (
  accelerator_temperature_celsius{<NS_FILTER>}
  * on (namespace, pod) group_left(workload, workload_type)
    acm_rs:pod_workload:relabel:5m
)[5m:])
<LABEL_JOIN>
```

- **`acm_rs:pod:gpu_sm_clock_hertz:5m`**

```promql
max_over_time(max by (namespace, pod, workload, workload_type) (
  accelerator_sm_clock_hertz{<NS_FILTER>}
  * on (namespace, pod) group_left(workload, workload_type)
    acm_rs:pod_workload:relabel:5m
)[5m:])
<LABEL_JOIN>
```

- **`acm_rs:pod:gpu_memory_clock_hertz:5m`**

```promql
max_over_time(max by (namespace, pod, workload, workload_type) (
  accelerator_memory_clock_hertz{<NS_FILTER>}
  * on (namespace, pod) group_left(workload, workload_type)
    acm_rs:pod_workload:relabel:5m
)[5m:])
<LABEL_JOIN>
```

#### Workload series (GPU)

- **`acm_rs:workload:gpu_request:5m`**

```promql
max_over_time(sum by (namespace, workload, workload_type) (
  kube_pod_container_resource_requests{<NS_FILTER>, resource=~"nvidia.com/gpu|amd.com/gpu", container!=""}
  * on (namespace, pod) group_left(workload, workload_type)
    acm_rs:pod_workload:relabel:5m
)[5m:])
<LABEL_JOIN>
```

- **`acm_rs:workload:gpu_usage:5m`**

```promql
max_over_time(sum by (namespace, workload, workload_type) (
  accelerator_gpu_utilization{<NS_FILTER>}
  * on (namespace, pod) group_left(workload, workload_type)
    acm_rs:pod_workload:relabel:5m
)[5m:])
<LABEL_JOIN>
```

- **`acm_rs:workload:gpu_memory_used:5m`**

```promql
max_over_time(sum by (namespace, workload, workload_type) (
  accelerator_memory_used_bytes{<NS_FILTER>}
  * on (namespace, pod) group_left(workload, workload_type)
    acm_rs:pod_workload:relabel:5m
)[5m:])
<LABEL_JOIN>
```

- **`acm_rs:workload:gpu_memory_total:5m`**

```promql
max_over_time(sum by (namespace, workload, workload_type) (
  accelerator_memory_total_bytes{<NS_FILTER>}
  * on (namespace, pod) group_left(workload, workload_type)
    acm_rs:pod_workload:relabel:5m
)[5m:])
<LABEL_JOIN>
```

- **`acm_rs:workload:gpu_power_usage_watts:5m`**

```promql
max_over_time(sum by (namespace, workload, workload_type) (
  accelerator_power_usage_watts{<NS_FILTER>}
  * on (namespace, pod) group_left(workload, workload_type)
    acm_rs:pod_workload:relabel:5m
)[5m:])
<LABEL_JOIN>
```

- **`acm_rs:workload:gpu_temperature_celsius:5m`**

```promql
max_over_time(max by (namespace, workload, workload_type) (
  accelerator_temperature_celsius{<NS_FILTER>}
  * on (namespace, pod) group_left(workload, workload_type)
    acm_rs:pod_workload:relabel:5m
)[5m:])
<LABEL_JOIN>
```

- **`acm_rs:workload:gpu_sm_clock_hertz:5m`**

```promql
max_over_time(max by (namespace, workload, workload_type) (
  accelerator_sm_clock_hertz{<NS_FILTER>}
  * on (namespace, pod) group_left(workload, workload_type)
    acm_rs:pod_workload:relabel:5m
)[5m:])
<LABEL_JOIN>
```

- **`acm_rs:workload:gpu_memory_clock_hertz:5m`**

```promql
max_over_time(max by (namespace, workload, workload_type) (
  accelerator_memory_clock_hertz{<NS_FILTER>}
  * on (namespace, pod) group_left(workload, workload_type)
    acm_rs:pod_workload:relabel:5m
)[5m:])
<LABEL_JOIN>
```

### Group: `acm-right-sizing-gpu-namespace-1d.rules`

All 1d rules have labels `profile="Max OverAll"` and `aggregation="1d"`.

- **`acm_rs:namespace:gpu_request`**: `max_over_time(acm_rs:namespace:gpu_request:5m[1d])`
- **`acm_rs:namespace:gpu_usage`**: `max_over_time(acm_rs:namespace:gpu_usage:5m[1d])`
- **`acm_rs:namespace:gpu_recommendation`**: `max_over_time(acm_rs:namespace:gpu_usage:5m[1d]) * (<RECOMMENDATION_PERCENT>/100)`
- **`acm_rs:namespace:gpu_memory_used`**: `max_over_time(acm_rs:namespace:gpu_memory_used:5m[1d])`
- **`acm_rs:namespace:gpu_memory_recommendation`**: `max_over_time(acm_rs:namespace:gpu_memory_used:5m[1d]) * (<RECOMMENDATION_PERCENT>/100)`
- **`acm_rs:namespace:gpu_memory_total`**: `max_over_time(acm_rs:namespace:gpu_memory_total:5m[1d])`
- **`acm_rs:namespace:gpu_power_usage_watts`**: `max_over_time(acm_rs:namespace:gpu_power_usage_watts:5m[1d])`
- **`acm_rs:namespace:gpu_temperature_celsius`**: `max_over_time(acm_rs:namespace:gpu_temperature_celsius:5m[1d])`
- **`acm_rs:namespace:gpu_sm_clock_hertz`**: `max_over_time(acm_rs:namespace:gpu_sm_clock_hertz:5m[1d])`
- **`acm_rs:namespace:gpu_memory_clock_hertz`**: `max_over_time(acm_rs:namespace:gpu_memory_clock_hertz:5m[1d])`

### Group: `acm-right-sizing-gpu-workload-1d.rules`

All 1d rules have labels `profile="Max OverAll"` and `aggregation="1d"`.

**Pod 1d series**

- `acm_rs:pod:gpu_request`: `max_over_time(acm_rs:pod:gpu_request:5m[1d])`
- `acm_rs:pod:gpu_usage`: `max_over_time(acm_rs:pod:gpu_usage:5m[1d])`
- `acm_rs:pod:gpu_recommendation`: `max_over_time(acm_rs:pod:gpu_usage:5m[1d]) * (<RECOMMENDATION_PERCENT>/100)`
- `acm_rs:pod:gpu_memory_used`: `max_over_time(acm_rs:pod:gpu_memory_used:5m[1d])`
- `acm_rs:pod:gpu_memory_recommendation`: `max_over_time(acm_rs:pod:gpu_memory_used:5m[1d]) * (<RECOMMENDATION_PERCENT>/100)`
- `acm_rs:pod:gpu_memory_total`: `max_over_time(acm_rs:pod:gpu_memory_total:5m[1d])`
- `acm_rs:pod:gpu_power_usage_watts`: `max_over_time(acm_rs:pod:gpu_power_usage_watts:5m[1d])`
- `acm_rs:pod:gpu_temperature_celsius`: `max_over_time(acm_rs:pod:gpu_temperature_celsius:5m[1d])`
- `acm_rs:pod:gpu_sm_clock_hertz`: `max_over_time(acm_rs:pod:gpu_sm_clock_hertz:5m[1d])`
- `acm_rs:pod:gpu_memory_clock_hertz`: `max_over_time(acm_rs:pod:gpu_memory_clock_hertz:5m[1d])`

**Workload 1d series**

- `acm_rs:workload:gpu_request`: `max_over_time(acm_rs:workload:gpu_request:5m[1d])`
- `acm_rs:workload:gpu_usage`: `max_over_time(acm_rs:workload:gpu_usage:5m[1d])`
- `acm_rs:workload:gpu_recommendation`: `max_over_time(acm_rs:workload:gpu_usage:5m[1d]) * (<RECOMMENDATION_PERCENT>/100)`
- `acm_rs:workload:gpu_memory_used`: `max_over_time(acm_rs:workload:gpu_memory_used:5m[1d])`
- `acm_rs:workload:gpu_memory_recommendation`: `max_over_time(acm_rs:workload:gpu_memory_used:5m[1d]) * (<RECOMMENDATION_PERCENT>/100)`
- `acm_rs:workload:gpu_memory_total`: `max_over_time(acm_rs:workload:gpu_memory_total:5m[1d])`
- `acm_rs:workload:gpu_power_usage_watts`: `max_over_time(acm_rs:workload:gpu_power_usage_watts:5m[1d])`
- `acm_rs:workload:gpu_temperature_celsius`: `max_over_time(acm_rs:workload:gpu_temperature_celsius:5m[1d])`
- `acm_rs:workload:gpu_sm_clock_hertz`: `max_over_time(acm_rs:workload:gpu_sm_clock_hertz:5m[1d])`
- `acm_rs:workload:gpu_memory_clock_hertz`: `max_over_time(acm_rs:workload:gpu_memory_clock_hertz:5m[1d])`

### Group: `acm-right-sizing-gpu-cluster-5m.rules`

- `acm_rs:cluster:gpu_request:5m`: `max_over_time(sum by (cluster) (kube_pod_container_resource_requests{<NS_FILTER>, resource=~"nvidia.com/gpu|amd.com/gpu", container!=""})[5m:])`
- `acm_rs:cluster:gpu_usage:5m`: `max_over_time(sum by (cluster) (accelerator_gpu_utilization{<NS_FILTER>})[5m:])`
- `acm_rs:cluster:gpu_memory_used:5m`: `max_over_time(sum by (cluster) (accelerator_memory_used_bytes{<NS_FILTER>})[5m:])`
- `acm_rs:cluster:gpu_memory_total:5m`: `max_over_time(sum by (cluster) (accelerator_memory_total_bytes{<NS_FILTER>})[5m:])`

### Group: `acm-right-sizing-gpu-cluster-1d.rules`

All 1d rules have labels `profile="Max OverAll"` and `aggregation="1d"`.

- `acm_rs:cluster:gpu_request`: `max_over_time(acm_rs:cluster:gpu_request:5m[1d])`
- `acm_rs:cluster:gpu_usage`: `max_over_time(acm_rs:cluster:gpu_usage:5m[1d])`
- `acm_rs:cluster:gpu_recommendation`: `max_over_time(acm_rs:cluster:gpu_usage:5m[1d]) * (<RECOMMENDATION_PERCENT>/100)`
- `acm_rs:cluster:gpu_memory_used`: `max_over_time(acm_rs:cluster:gpu_memory_used:5m[1d])`
- `acm_rs:cluster:gpu_memory_recommendation`: `max_over_time(acm_rs:cluster:gpu_memory_used:5m[1d]) * (<RECOMMENDATION_PERCENT>/100)`
- `acm_rs:cluster:gpu_memory_total`: `max_over_time(acm_rs:cluster:gpu_memory_total:5m[1d])`

## Configuration

### Enablement (MCO spec)

GPU right-sizing is controlled by the MCO CR:

- `spec.capabilities.platform.analytics.gpuRightSizingRecommendation.enabled` (boolean)
- `spec.capabilities.platform.analytics.gpuRightSizingRecommendation.namespaceBinding` (string; default `open-cluster-management-global-set`)

### ConfigMap (`rs-gpu-config`)

The controller creates/uses the ConfigMap:

- **Name**: `rs-gpu-config`
- **Namespace**: `open-cluster-management-observability` (MCO operator namespace)
- **Keys**:
  - `prometheusRuleConfig` (YAML)
  - `placementConfiguration` (YAML; OCM `Placement` spec used to select clusters)

The configuration schema matches workload/pod right-sizing (`namespaceFilterCriteria`, optional `label_env` filtering, `recommendationPercentage`).

- **Default `recommendationPercentage`**: `110` (meaning recommendation = `1.10 * max_usage_1d`)

## Working flow (end-to-end)

1. **Hub**: `AnalyticsReconciler` watches the single `MultiClusterObservability` CR and the right-sizing ConfigMaps.
2. **Enable**: when `gpuRightSizingRecommendation.enabled=true`, MCO ensures `rs-gpu-config` exists (with defaults if missing).
3. **Render rules**: MCO reads the ConfigMap YAML, builds `<NS_FILTER>` and optional `<LABEL_JOIN>`, and generates the GPU `PrometheusRule`.
4. **Deploy rules**
   - **Managed clusters**: MCO creates/updates an ACM `Policy` that embeds a `ConfigurationPolicy` enforcing the `PrometheusRule` in `openshift-monitoring`. An OCM `Placement` selects clusters, and a `PlacementBinding` binds the Policy to the Placement.
   - **Hub local-cluster**: MCO also applies the same `PrometheusRule` directly on the hub in `openshift-monitoring`.
5. **Evaluate**: on each selected cluster, `prometheus-k8s` evaluates the recording rules and emits `acm_rs:*` GPU series.
6. **Federate (MCOA deployments)**: `platform-metrics-collector` federates selected GPU right-sizing series via `manifests/base/grafana/analytics/scrape-config.yaml`.
7. **Visualize**: the GPU dashboard (`dash-acm-gpu-utilization.yaml`) uses these recording rules for “request vs usage” and device stats.

## Source of truth (implementation)

- Rule generator: `operators/multiclusterobservability/controllers/analytics/rightsizing/rs-gpu/prometheusrule.go`
- Deployment wiring (Policy/Placement/ConfigMap): `operators/multiclusterobservability/controllers/analytics/rightsizing/rs-gpu/*.go`
- Shared utilities: `operators/multiclusterobservability/controllers/analytics/rightsizing/rs-utility/*.go`

