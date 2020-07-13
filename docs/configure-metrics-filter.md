# Metrics Filter

when installed `multicluster-monitoring-operator` successfully, you can find a configmap `grafana-dashboards-metrics` in `open-cluster-management-monitoring` namespaces.

```bash
$ kubectl get cm grafana-dashboards-metrics -n open-cluser-management-monitoring -o yaml
apiVersion: v1
data:
  metrics.yaml: |2

     # additionalMetrics:
     #   - additional_metrics_1
     #   - additional_metrics_2
     #   - regex_for_match_metrics_1
     #   - regex_for_match_metrics_2
kind: ConfigMap
metadata:
  creationTimestamp: "2020-07-10T16:43:27Z"
  labels:
    app: monitoring
  name: grafana-dashboards-metrics
  namespace: open-cluster-management-monitoring
  ownerReferences:
  - apiVersion: monitoring.open-cluster-management.io/v1alpha1
    blockOwnerDeletion: true
    controller: true
    kind: MultiClusterMonitoring
    name: monitoring
    uid: 982b6ad5-6078-474b-b110-d9b141bd06f3
  resourceVersion: "5666826"
  selfLink: /api/v1/namespaces/open-cluster-management-monitoring/configmaps/grafana-dashboards-metrics
  uid: 98f12d62-84e6-4e4c-8496-4237f1823fd1
```

You can configure this configmap with respect to which metrics to collect, this configmap only supports 2 configuration sections: `defalutMetrics` and `additionalMetrics`:

- defalutMetrics: these metrics used for the built-in dashboard to show cluster status, you do not need to configure this section. By default, we do not show this section.

- additionalMetrics: these metrics used for user customization, if you configure this section with some metrics, `multicluster-monitoring-operator` will collect these metrics and store them to your object storage.

when you configured this configmap with some metrics, you can check `cluster-monitoring-config` configmap in `openshift-monitoring` namespaces, and  then you can find these metrics in `cluster-monitoring-config`.
