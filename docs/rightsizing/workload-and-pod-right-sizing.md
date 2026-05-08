# Workload and Pod Right-Sizing (recording rules)

This document describes how **workload** and **pod** right-sizing metrics are produced in MCO via **Prometheus recording rules**, which **base metrics** are used, what the **PrometheusRule** contains, and the end-to-end **working flow**.

## Overview

When enabled, the MCO analytics right-sizing controller generates a `PrometheusRule` that records:

- **Requests** (CPU, memory)
- **Limits** (CPU, memory)
- **Usage** (CPU, memory)
- **Recommendations** (CPU, memory) as a multiplier of the max usage over a 1-day window

The rules produce time series with the `acm_rs:*` prefix, which are then consumed by analytics dashboards (and, in MCOA deployments, federated to the hub via a `ScrapeConfig`).

## Base metrics used (inputs)

The workload/pod right-sizing recording rules are derived from these base metrics/series:

- **Kubernetes ownership / workload mapping (kube-state-metrics)**
  - `kube_pod_owner`
  - `kube_replicaset_owner`
  - `kube_job_owner`
- **Requests/limits (kube-state-metrics)**
  - `kube_pod_container_resource_requests{resource="cpu"|"memory",container!=""}`
  - `kube_pod_container_resource_limits{resource="cpu"|"memory",container!=""}`
- **CPU usage (OpenShift monitoring recording rule)**
  - `node_namespace_pod_container:container_cpu_usage_seconds_total:sum_irate{container!=""}`
- **Memory usage (cAdvisor)**
  - `container_memory_working_set_bytes{container!=""}`

## Generated PrometheusRule

- **PrometheusRule name**: `acm-rs-workload-prometheus-rules`
- **Namespace**: `openshift-monitoring`
- **Labels (required for OpenShift rule selection)**:
  - `prometheus: k8s`
  - `role: alert-rules`
- **Rule groups**:
  - `acm-right-sizing-workload-5m.rules` (interval `5m`): records “raw” 5m-window maxima
  - `acm-right-sizing-workload-1d.rules` (interval `1h`): rolls up to “max over last 1 day” and produces recommendations

## Prometheus rules (record → PromQL)

The expressions below are shown in their generated form, using placeholders:

- **`<NS_FILTER>`**: comes from `prometheusRuleConfig.namespaceFilterCriteria` (defaults to `namespace!~"openshift.*"`; if unset, it becomes `namespace!=""`).
- **`<LABEL_JOIN>`**: optional label-based join appended to *every* rule expression (currently supports filtering by `label_env` via `kube_namespace_labels`). See [Configuration](#configuration).

### Group: `acm-right-sizing-workload-5m.rules`

#### Shared mapping (pod → workload/workload_type)

- **`acm_rs:pod_workload:relabel:5m`**:

```promql
(
  max by (namespace, pod, workload, workload_type) (
    label_replace(
      label_replace(
        kube_pod_owner{<NS_FILTER>, owner_kind=~"StatefulSet|DaemonSet"},
        "workload", "$1", "owner_name", "(.*)"
      ),
      "workload_type", "$1", "owner_kind", "(.*)"
    )
  )
)
or
(
  max by (namespace, pod, workload, workload_type) (
    label_replace(
      label_replace(
        (
          label_replace(
            kube_pod_owner{<NS_FILTER>, owner_kind="ReplicaSet"},
            "replicaset", "$1", "owner_name", "(.*)"
          )
          * on (namespace, replicaset) group_left(owner_name)
            topk by (namespace, replicaset) (
              1,
              max by (namespace, replicaset, owner_name) (
                kube_replicaset_owner{<NS_FILTER>, owner_kind="Deployment"}
              )
            )
        ),
        "workload", "$1", "owner_name", "(.*)"
      ),
      "workload_type", "Deployment", "workload", ".*"
    )
  )
)
or
(
  max by (namespace, pod, workload, workload_type) (
    label_replace(
      label_replace(
        (
          label_replace(
            kube_pod_owner{<NS_FILTER>, owner_kind="ReplicaSet"},
            "replicaset", "$1", "owner_name", "(.*)"
          )
          unless on (namespace, replicaset)
            kube_replicaset_owner{<NS_FILTER>, owner_kind="Deployment"}
        ),
        "workload", "$1", "replicaset", "(.*)"
      ),
      "workload_type", "ReplicaSet", "workload", ".*"
    )
  )
)
or
(
  max by (namespace, pod, workload, workload_type) (
    label_replace(
      label_replace(
        (
          label_replace(
            kube_pod_owner{<NS_FILTER>, owner_kind="Job"},
            "job_name", "$1", "owner_name", "(.*)"
          )
          * on (namespace, job_name) group_left(owner_name)
            max by (namespace, job_name, owner_name) (
              kube_job_owner{<NS_FILTER>, owner_kind="CronJob"}
            )
        ),
        "workload", "$1", "owner_name", "(.*)"
      ),
      "workload_type", "CronJob", "workload", ".*"
    )
  )
)
or
(
  max by (namespace, pod, workload, workload_type) (
    label_replace(
      label_replace(
        (
          kube_pod_owner{<NS_FILTER>, owner_kind="Job"}
          unless on (namespace, owner_name)
            max by (namespace, owner_name) (
              label_replace(
                kube_job_owner{<NS_FILTER>, owner_kind="CronJob"},
                "owner_name", "$1", "job_name", "(.*)"
              )
            )
        ),
        "workload", "$1", "owner_name", "(.*)"
      ),
      "workload_type", "Job", "workload", ".*"
    )
  )
)
<LABEL_JOIN>
```

#### Pod series (requests, limits, usage)

- **`acm_rs:pod:cpu_request:5m`**

```promql
max_over_time(sum by (namespace, pod, workload, workload_type) (
  kube_pod_container_resource_requests{<NS_FILTER>, resource="cpu", container!=""}
  * on (namespace, pod) group_left(workload, workload_type)
    acm_rs:pod_workload:relabel:5m
)[5m:])
<LABEL_JOIN>
```

- **`acm_rs:pod:cpu_limit:5m`**

```promql
max_over_time(sum by (namespace, pod, workload, workload_type) (
  kube_pod_container_resource_limits{<NS_FILTER>, resource="cpu", container!=""}
  * on (namespace, pod) group_left(workload, workload_type)
    acm_rs:pod_workload:relabel:5m
)[5m:])
<LABEL_JOIN>
```

- **`acm_rs:pod:cpu_usage:5m`**

```promql
max_over_time(sum by (namespace, pod, workload, workload_type) (
  node_namespace_pod_container:container_cpu_usage_seconds_total:sum_irate{<NS_FILTER>, container!=""}
  * on (namespace, pod) group_left(workload, workload_type)
    acm_rs:pod_workload:relabel:5m
)[5m:])
<LABEL_JOIN>
```

- **`acm_rs:pod:memory_request:5m`**

```promql
max_over_time(sum by (namespace, pod, workload, workload_type) (
  kube_pod_container_resource_requests{<NS_FILTER>, resource="memory", container!=""}
  * on (namespace, pod) group_left(workload, workload_type)
    acm_rs:pod_workload:relabel:5m
)[5m:])
<LABEL_JOIN>
```

- **`acm_rs:pod:memory_limit:5m`**

```promql
max_over_time(sum by (namespace, pod, workload, workload_type) (
  kube_pod_container_resource_limits{<NS_FILTER>, resource="memory", container!=""}
  * on (namespace, pod) group_left(workload, workload_type)
    acm_rs:pod_workload:relabel:5m
)[5m:])
<LABEL_JOIN>
```

- **`acm_rs:pod:memory_usage:5m`**

```promql
max_over_time(sum by (namespace, pod, workload, workload_type) (
  container_memory_working_set_bytes{<NS_FILTER>, container!=""}
  * on (namespace, pod) group_left(workload, workload_type)
    acm_rs:pod_workload:relabel:5m
)[5m:])
<LABEL_JOIN>
```

#### Workload series (requests, limits, usage)

- **`acm_rs:workload:cpu_request:5m`**

```promql
max_over_time(sum by (namespace, workload, workload_type) (
  kube_pod_container_resource_requests{<NS_FILTER>, resource="cpu", container!=""}
  * on (namespace, pod) group_left(workload, workload_type)
    acm_rs:pod_workload:relabel:5m
)[5m:])
<LABEL_JOIN>
```

- **`acm_rs:workload:cpu_limit:5m`**

```promql
max_over_time(sum by (namespace, workload, workload_type) (
  kube_pod_container_resource_limits{<NS_FILTER>, resource="cpu", container!=""}
  * on (namespace, pod) group_left(workload, workload_type)
    acm_rs:pod_workload:relabel:5m
)[5m:])
<LABEL_JOIN>
```

- **`acm_rs:workload:cpu_usage:5m`**

```promql
max_over_time(sum by (namespace, workload, workload_type) (
  node_namespace_pod_container:container_cpu_usage_seconds_total:sum_irate{<NS_FILTER>, container!=""}
  * on (namespace, pod) group_left(workload, workload_type)
    acm_rs:pod_workload:relabel:5m
)[5m:])
<LABEL_JOIN>
```

- **`acm_rs:workload:memory_request:5m`**

```promql
max_over_time(sum by (namespace, workload, workload_type) (
  kube_pod_container_resource_requests{<NS_FILTER>, resource="memory", container!=""}
  * on (namespace, pod) group_left(workload, workload_type)
    acm_rs:pod_workload:relabel:5m
)[5m:])
<LABEL_JOIN>
```

- **`acm_rs:workload:memory_limit:5m`**

```promql
max_over_time(sum by (namespace, workload, workload_type) (
  kube_pod_container_resource_limits{<NS_FILTER>, resource="memory", container!=""}
  * on (namespace, pod) group_left(workload, workload_type)
    acm_rs:pod_workload:relabel:5m
)[5m:])
<LABEL_JOIN>
```

- **`acm_rs:workload:memory_usage:5m`**

```promql
max_over_time(sum by (namespace, workload, workload_type) (
  container_memory_working_set_bytes{<NS_FILTER>, container!=""}
  * on (namespace, pod) group_left(workload, workload_type)
    acm_rs:pod_workload:relabel:5m
)[5m:])
<LABEL_JOIN>
```

### Group: `acm-right-sizing-workload-1d.rules`

All 1d rules have labels:

- `profile="Max OverAll"`
- `aggregation="1d"`

They represent a roll-up: **max over the last 1 day** of the corresponding `:5m` rule.

#### Pod series (1d maxima + recommendations)

- **`acm_rs:pod:cpu_request`**: `max_over_time(acm_rs:pod:cpu_request:5m[1d])`
- **`acm_rs:pod:cpu_limit`**: `max_over_time(acm_rs:pod:cpu_limit:5m[1d])`
- **`acm_rs:pod:cpu_usage`**: `max_over_time(acm_rs:pod:cpu_usage:5m[1d])`
- **`acm_rs:pod:cpu_recommendation`**: `max_over_time(acm_rs:pod:cpu_usage:5m[1d]) * (<RECOMMENDATION_PERCENT>/100)`
- **`acm_rs:pod:memory_request`**: `max_over_time(acm_rs:pod:memory_request:5m[1d])`
- **`acm_rs:pod:memory_limit`**: `max_over_time(acm_rs:pod:memory_limit:5m[1d])`
- **`acm_rs:pod:memory_usage`**: `max_over_time(acm_rs:pod:memory_usage:5m[1d])`
- **`acm_rs:pod:memory_recommendation`**: `max_over_time(acm_rs:pod:memory_usage:5m[1d]) * (<RECOMMENDATION_PERCENT>/100)`

#### Workload series (1d maxima + recommendations)

- **`acm_rs:workload:cpu_request`**: `max_over_time(acm_rs:workload:cpu_request:5m[1d])`
- **`acm_rs:workload:cpu_limit`**: `max_over_time(acm_rs:workload:cpu_limit:5m[1d])`
- **`acm_rs:workload:cpu_usage`**: `max_over_time(acm_rs:workload:cpu_usage:5m[1d])`
- **`acm_rs:workload:cpu_recommendation`**: `max_over_time(acm_rs:workload:cpu_usage:5m[1d]) * (<RECOMMENDATION_PERCENT>/100)`
- **`acm_rs:workload:memory_request`**: `max_over_time(acm_rs:workload:memory_request:5m[1d])`
- **`acm_rs:workload:memory_limit`**: `max_over_time(acm_rs:workload:memory_limit:5m[1d])`
- **`acm_rs:workload:memory_usage`**: `max_over_time(acm_rs:workload:memory_usage:5m[1d])`
- **`acm_rs:workload:memory_recommendation`**: `max_over_time(acm_rs:workload:memory_usage:5m[1d]) * (<RECOMMENDATION_PERCENT>/100)`

## Configuration

### Enablement (MCO spec)

Workload+pod right-sizing is controlled by the MCO CR:

- `spec.capabilities.platform.analytics.workloadPodRightSizingRecommendation.enabled` (boolean)
- `spec.capabilities.platform.analytics.workloadPodRightSizingRecommendation.namespaceBinding` (string; default `open-cluster-management-global-set`)

### ConfigMap (`rs-workload-config`)

The controller creates/uses the ConfigMap:

- **Name**: `rs-workload-config`
- **Namespace**: `open-cluster-management-observability` (MCO operator namespace)
- **Keys**:
  - `prometheusRuleConfig` (YAML)
  - `placementConfiguration` (YAML; OCM `Placement` spec used to select clusters)

`prometheusRuleConfig` supports:

- **`namespaceFilterCriteria`** (one of):
  - `inclusionCriteria: ["ns-a","ns-b"]` → `namespace=~"ns-a|ns-b"`
  - `exclusionCriteria: ["openshift.*"]` → `namespace!~"openshift.*"` (default)
- **`labelFilterCriteria`**:
  - currently supports `label_env` only, implemented as a join against `kube_namespace_labels`
- **`recommendationPercentage`**:
  - default `110` (meaning recommendation = `1.10 * max_usage_1d`)

## Working flow (end-to-end)

1. **Hub**: `AnalyticsReconciler` watches the single `MultiClusterObservability` CR and the right-sizing ConfigMaps (`rs-workload-config`, etc.).
2. **Enable**: when `workloadPodRightSizingRecommendation.enabled=true`, MCO ensures `rs-workload-config` exists (with defaults if missing).
3. **Render rules**: MCO reads the ConfigMap YAML, builds `<NS_FILTER>` and optional `<LABEL_JOIN>`, then generates the `PrometheusRule` object.
4. **Deploy rules**
   - **Managed clusters**: MCO creates/updates an ACM `Policy` that embeds a `ConfigurationPolicy` enforcing the `PrometheusRule` in `openshift-monitoring`. An OCM `Placement` selects target clusters, and a `PlacementBinding` binds the Policy to the Placement.
   - **Hub local-cluster**: MCO also applies the same `PrometheusRule` directly on the hub in `openshift-monitoring` (to keep hub/local-cluster right-sizing working even if policy propagation to `local-cluster` is disabled).
5. **Evaluate**: on each selected cluster, `prometheus-k8s` evaluates the recording rules and emits `acm_rs:*` series.
6. **Federate (MCOA deployments)**: `platform-metrics-collector` uses a `ScrapeConfig` (`manifests/base/grafana/analytics/scrape-config.yaml`) to federate selected `acm_rs:*` series to the hub for global dashboards.
7. **Visualize**: Grafana dashboards (for example `dash-acm-right-sizing-workloads-pods.yaml`) query the `acm_rs:*` series.

## Source of truth (implementation)

- Rule generator: `operators/multiclusterobservability/controllers/analytics/rightsizing/rs-workload/prometheusrule.go`
- Deployment wiring (Policy/Placement/ConfigMap): `operators/multiclusterobservability/controllers/analytics/rightsizing/rs-workload/*.go`
- Shared utilities: `operators/multiclusterobservability/controllers/analytics/rightsizing/rs-utility/*.go`

