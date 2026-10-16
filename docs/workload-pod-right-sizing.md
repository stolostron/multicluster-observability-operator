# Workload and pod right-sizing

Workload and pod right-sizing are independently enabled by default. The analytics
controller persists omitted `enabled` flags as `true` and preserves explicit `false`.
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
fields remain absent/blank. New workload/pod ConfigMaps include all namespaces;
existing user namespace filters are preserved. Remove `openshift.*` from an older
ConfigMap's exclusion criteria to include system namespaces as well.
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
MCO resource rather than editing the managed ADC. The deprecated
`workloadPodRightSizingRecommendation.enabled` field is still read during
upgrade for migration into any omitted `workloadRightSizingRecommendation` /
`podRightSizingRecommendation` switches. Explicit new switches take precedence.
The retired ADC key `platformWorkloadPodRightSizing` is no longer written and is
removed from existing AddOnDeploymentConfigs on reconcile. For a fresh
configuration with no old or new switches, both features are enabled.

When deploying custom images, also install the updated CRD at
`operators/multiclusterobservability/config/crd/bases/observability.open-cluster-management.io_multiclusterobservabilities.yaml`.
Updating the operator image alone does not update the API-server schema; an older
CRD can prune the new fields. Both the base CRD and OLM bundle CRD include the new fields.

Legacy metrics collection forwards the 16 profiled workload/pod request, limit,
usage, and recommendation metrics plus `acm_rs:pod:info` through the default metrics allowlist. Pod
inventory preserves table rows and selectors even without resource measurements;
missing fields stay blank, and the inventory value is hidden. The
intermediate `:5m` series remain on the source cluster. MCOA federation includes
the same profiled metrics when its platform collector is enabled.

When using this operator with an older ACM installation, also use the matching
RBAC query proxy image: the operator configures TLS flags that older proxy images
do not accept. Preserve the image overrides through the MultiClusterHub's
`installer.open-cluster-management.io/image-overrides-configmap` configuration.
The proxy image override key is `rbac_query_proxy`.
