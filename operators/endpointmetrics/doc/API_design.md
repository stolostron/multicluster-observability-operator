## ObservabilityAddon API

### API Design:

The requirement doc is located in [here](https://docs.google.com/document/d/1qawBUo8VcdBXuXzZl8sypIug1nLsUEm_5Yy0qENZ-aU)

ObservabilityAddon CR is namespace scoped and located in each managed cluster namespace in hub side if observability addon is enabled for that managed cluster. The initial instance will be created by ACM in the managed cluster namespace as part of managed cluster import/create process and users can customize it later. One CR includes two sections: one for spec and the other for status.

Group of this CR is observability.open-cluster-management.io

version is v1beta1

kind is ObservabilityAddon

**ObservabilityAddon Spec**: the specification and status for the metrics collector in one managed cluster

name | description | required | default | schema
---- | ----------- | -------- | ------- | ------
enableMetrics | Push metrics or not | no | true | bool
metricsConfigs| Metrics collection configurations | no | n/a | MetricsConfigs


**MetricsConfigs Spec**: the specification for metrics collected  from local prometheus and pushed to hub server

name | description | required | default | schema
---- | ----------- | -------- | ------- | ------
interval | Interval for the metrics collector push metrics to  hub server| no | 1m | string


**ObservabilityAddon Status**: the status for current CR. It's updated by the metrics collector

name | description | required | default | schema
---- | ----------- | -------- | ------- | ------
status | Status contains the different condition statuses for this managed cluster | n/a | [] | []Condtions

**Conditions**
type | reason | message
---- | ------ | -------
Ready | Deployed | Metrics collector deployed and functional
Disabled | Disabled | enableMetrics is set to False
NotSupported | NotSupported | Observability is not supported in this cluster

### Samples

Here is a sample ObservabilityAddon CR

```
apiVersion: observability.open-cluster-management.io/v1beta1
kind: ObservabilityAddon
metadata:
  name: sample-endpointmonitoring
spec:
  enableMetrics: true
  metricsConfigs:
    interval: 1m
status:
  conditions:
    - type: Ready
      status: 'True'
      lastTransitionTime: '2020-07-23T16:18:46Z'
      reason: Deployed
      message: Metrics collector deployed and functional
    - type: Disabled
      status: 'True'
      lastTransitionTime: '2020-07-23T15:18:46Z'
      reason: Disabled
      message: enableMetrics is set to False
```
