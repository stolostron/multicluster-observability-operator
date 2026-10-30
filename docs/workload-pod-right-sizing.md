# Workload and pod right-sizing

Workload and pod right-sizing take their default from namespace right-sizing.
When a switch is omitted, the operator fills it in once with the namespace
switch's value at that time: enabled when namespace right-sizing is enabled (its
own default), disabled when namespace right-sizing is disabled. The value is then
saved in the resource, so turning namespace right-sizing off later does not turn
workload or pod right-sizing off. Users can still override any of the four
switches independently.
Configure them in the `MultiClusterObservability` resource:

```yaml
spec:
  capabilities:
    platform:
      analytics:
        workloadRightSizingRecommendation:
          enabled: true
        podRightSizingRecommendation:
          enabled: true
```

Set either flag to `false` to disable only that feature. Its ConfigMap,
PrometheusRule, and overview/detail Perses dashboards are removed. Pod
right-sizing includes pods without a workload and pods whose owner is not a
recognized workload (for example, static pods owned by a Node). Their workload
fields remain absent/blank. New workload/pod ConfigMaps include all namespaces,
`openshift-*` included; existing user namespace filters are preserved.
When pod right-sizing is disabled,
the workload detail dashboard omits the pod tables and pod drill-down links.

The pod detail dashboard contains CPU/memory summary panels and time-series
charts, with no tables. Pod tables remain on the pod overview and workload detail
dashboards.

| Feature | AddOnDeploymentConfig variable | ConfigMap | PrometheusRule |
| --- | --- | --- | --- |
| Workload | `platformWorkloadRightSizing` | `rs-workload-config` | `acm-rs-workload-prometheus-rules` |
| Pod | `platformPodRightSizing` | `rs-pod-config` | `acm-rs-pod-prometheus-rules` |

The operator manages the ADC variables as `enabled` or `disabled`; configure the
MCO resource rather than editing the managed ADC. When either switch is omitted,
it inherits its one-time default from namespace right-sizing (enabled by
default). A hub that previously disabled namespace right-sizing will not get
workload or pod right-sizing enabled unexpectedly.

Workload and pod right-sizing support the same configurable evaluation intervals
as namespace and virtualization right-sizing. Edit `prometheusRuleConfig` in
`rs-workload-config` or `rs-pod-config` in the hub's
`open-cluster-management-observability` namespace, preserving the other settings:

```yaml
evaluationInterval: 10m
evaluationInterval1d: 30m
```

Each ConfigMap controls its feature independently. `evaluationInterval` sets both
the rolling rule group's evaluation cadence and its lookback window. It defaults
to `5m` and accepts single-unit durations from `5m` through `1h`.
`evaluationInterval1d` sets how often daily aggregates are refreshed, defaults to
`15m`, and accepts single-unit durations from `15m` through `6h`; the aggregation
window remains one day. Recorded metric names, including their `:5m` suffixes,
remain unchanged when the intervals change.

MCO delegates rule generation and ConfigMap validation to MCOA, including when
using legacy metrics collection. MCOA applies interval edits during reconciliation.
Missing keys use the defaults without rewriting existing ConfigMaps. Invalid
values are restored to their last valid value, or to the default if no previous
value is cached. As with other right-sizing features, changing the rolling
interval changes the samples used by percentile profiles; allow a day for the
daily window to contain only samples recorded at the new interval.

Legacy metrics collection forwards the 16 profiled workload/pod request, limit,
usage, and recommendation metrics plus `acm_rs:pod:info` through the default metrics allowlist. Pod
inventory preserves table rows and selectors even without resource measurements;
missing fields stay blank, and the inventory value is hidden. The
intermediate `:5m` series remain on the source cluster. MCOA federation includes
the same profiled metrics when its platform collector is enabled.

When enabled, each pod produces approximately 25 time series on the hub (8 profiled
request/limit/usage/recommendation metrics × 3 default profiles, plus `acm_rs:pod:info`),
and each workload produces approximately 24. Every pod is recorded, including each
Job run, and its daily series remain for about 24 hours after the pod is gone. Use the
`placementConfiguration` in `rs-workload-config` / `rs-pod-config` to limit which
clusters run workload or pod right-sizing.
