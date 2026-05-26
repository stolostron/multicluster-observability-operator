## ManagedCluster Monitoring API

### API Design:

The requirement doc is located in [here](https://docs.google.com/document/d/1qawBUo8VcdBXuXzZl8sypIug1nLsUEm_5Yy0qENZ-aU)

ObservabilityAddon CR is namespace scoped and located in each cluster namespace in hub side if monitoring feature is enabled for that managed cluster. Hub operator will generate the default one in the cluster namespace and users can customize it later. One CR includes two sections: one for spec and the other for status.

Group of this CR is observability.open-cluster-management.io, version is v1beta1, kind is ObservabilityAddon

**ObservabilityAddon** Spec: describe the specification and status for the metrics collector in one managed cluster

name | description | required | default | schema
---- | ----------- | -------- | ------- | ------
enableMetrics | Push metrics or not | yes | true | bool
metricsConfigs| Metrics collection configurations | yes | n/a | MetricsConfigs


**MetricsConfigs Spec**: describe the specification for metrics collected  from local prometheus and pushed to hub server

name | description | required | default | schema
---- | ----------- | -------- | ------- | ------
metricsSource | The server configuration to get metrics from | no | n/a | MetricsSource
interval | Interval to collect&push metrics | yes | 1m | string
allowlistConfigMaps | List  of configmap name. For each configmap it contains the allowlist for metrics pushed to hub. It only includes the metrics customized by users. The default metrics will also be pushed even if this value is empty. | no | n/a | []string
scrapeTargets | Additional scrape targets added to local prometheus to scrape additional metrics. The metrics scraped from the new added scrape targets will be included in the allowlist of metrics.(filter the metrics using {job=”SERVICE_MONITOR_NAME”}) | no | n/a | [][ServiceMonitorSpec](https://github.com/coreos/prometheus-operator/blob/master/Documentation/api.md#servicemonitorspec)
rules | List for alert rules and recording rules. The metrics defined in the new-added recording rules will be included in the allowlist of metrics. | no | n/a | [][Rule](https://github.com/coreos/prometheus-operator/blob/master/Documentation/api.md#rule 

**MetricsSource Spec**: describe the information to get the metrics

name | description | required | default | schema
---- | ----------- | -------- | ------- | ------
serverURL | The server url is to get metrics from | yes | https://prometheus-k8s.openshift-monitoring.svc:9091 | string
tlsConfig | A file containing the CA certificate to use to verify the Prometheus server | no | n/a | *[TLSConfig](https://github.com/coreos/prometheus-operator/blob/master/Documentation/api.md#tlsconfig)

**ObservabilityAddon Status**: describe the status for current CR. It's updated by the metrics collector

name | description | required | default | schema
---- | ----------- | -------- | ------- | ------
conditions | Conditions contains the different condition statuses for this managed cluster | no | [] | []Condtions

**Condition**: describe the condition status for current CR.

name | description | required | default | schema
---- | ----------- | -------- | ------- | ------
lastTransitionTime | Last time the condition transit from one status to another | yes | n/a | Time
status | Status of the condition, one of True, False, Unknown | yes | n/a | string
reason | (brief) reason for the condition's last transition | yes | n/a | string
message | Human readable message indicating details about last transition | yes | n/a | string
type | Type of node condition | yes | n/a | string



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
    metricsSource:
      serverUrl: https://*****
      tlsConfig:
        ca: local-ca-secret
        cert: local-cert-secret
    allowlistConfigMaps:
    - sample-allowlist
    scrapeTargets:
    - endpoints:
      - interval: 30s
        port: web
        scheme: https
      namespaceSelector: {}
      selector:
        matchLabels:
          alertmanager: test
    rules:
    - alert: HighRequestLatency
      expr: job:request_latency_seconds:mean5m{job="myjob"} > 0.5
      for: 10m
    - record: job:http_inprogress_requests:sum
      expr: sum by (job) (http_inprogress_requests)
status:
  conditions:
    - type: Available
      status: 'True'
      lastTransitionTime: '2020-07-23T16:18:46Z'
      reason: ClientCreated
      message: The metrics collector client deployment created
```
