## Grafana Issues

### Grafana is loading but dashbords show no data

Assuming that thanos is working correctly (it is able to ingest and query data). You can proceed with the following steps to debug the issue:

* Check if the Grafana exlorer is working by querying a random metric like `up`.
  * If it is not working, check [communication](#the-read-path) between the components on the read path using the user token as Grafana does. 
  * Otherwise, Grafana is able to fetch data, and there is an issue with the dashboards specifically. Continue to the next step.
* Check if the `Label` and `Value` fields are correctly populated in the top left corner. If they contain data, well, it should be working... Otherwise continue to the next step.
* These fields are populated using the `acm_managed_cluster_labels` metric and the `observability-managed-cluster-label-allowlist` configmap. This configmap is read by the `rbac-query-proxy`. Check if you see error logs in the `rbac-query-proxy` pod that points to an issue with those. You can restart it to make sure they have latest configmap data.
* Check if the `acm_managed_cluster_labels` is collected by ACM from the grafana explorer. If not, make sure it is collected by the metrics collector on the hub.
  * Chech the list of metrics in the `metrics-collector` pod args.
  * Make sure that the `acm_managed_cluster_labels` metric is in the `observability-metrics-allowlist` configmap in the `metrics_list.yaml` file.
* Check if the `acm_managed_cluster_labels` is collected on the hub by the in-cluster prometheus. You can use the console for this.
  * If it is not collected, move to the __Prometheus scraping issues__ section. Bear in mind that the `acm_managed_cluster_labels` metric is managed by the MCE team. It is scraped from the `clusterlifecycle-state-metrics` pod in the `multicluster-engine` namespace.

## Prometheus scraping issues

You expect some metrics in Prometheus but they are not there, and prometheus is up and working. You can do some of the following checks from the Prometheus UI, the Console -> Observe section. We focus here mainly on debugging using shells. You can run the commands from the console, getting a terminal from the target pod, or from your local machine using `oc exec -n openshift-monitoring prometheus-k8s-0 -c prometheus -- `. Here are some steps to debug the issue:

* Check if the target exists in Prometheus:
  * Go in the Console -> Targets section. Look for the serviceMonitor in your namespace and check if the target is there.
  * Or check the currently loaded configuration of the prometheus pod to see if you find the scraping job named after it, and if it is correctly configured:
```bash
curl http://localhost:9090/api/v1/status/config | sed 's/\\n/\n/g' | grep "job_name: serviceMonitor/NAMESPACE/SERVICE_MONITOR_NAME" -A100 
# or from your local machine, pipe in less and search for the job name
curl -s http://localhost:9090/api/v1/status/config | jq -r '.data.yaml' | yq '.scrape_configs' | less
```
* If the target was not discovered, check if prometheus can reach the target by fetching the `metrics` path of the target from the prometheus pod in the `openshift-monitoring` namespace:
```bash
curl http://<TARGET_SERVICE>:<TARGET_PORT>/metrics
```
  * Make sure that you find the metrics you are looking for.
  * If it fails check the network policies.
* If the target exists, make sure that relabelling rules are not impacting your metrics.
* Check the state of the target:
  * Go in the Console -> Targets section. Look for the target you are interested in and click on it. You'll see the last scrape time and the last error.
  * Or check the metric `up{job="<JOB_NAME>"}`. If it is `0`, the scrape failed. If it is `1`, the target is reachable.
  * Or look for target scraping errors using the prometheus API. This command needs jq to parse the output, so from your local machine. Check the `lastError` field for the active targets:
```bash
curl 'http://localhost:9090/api/v1/targets?scrapePool=serviceMonitor%2F<NAMESPACE>%2F<SERVICE_MONITOR_NAME>%2F0' | jq 'del(.data.droppedTargets, .data.droppedTargetCounts)'
```
* Sometimes, the error is not very detailed. If the target is reached, check the validity of the exposed metrics format with promtool from a `prometheus-k8s` pod in the `openshift-monitoring` namespace:
```bash
curl http://<TARGET_SERVICE>:<TARGET_PORT>/metrics | promtool check metrics
```

## Metrics collection issues

### Federation issues

You can test communication with the federation endpoint using the following query from the `metrics-collector` pod:

```bash
curl -k -H "Authorization: Bearer $(cat /var/run/secrets/kubernetes.io/serviceaccount/token)" 'https://prometheus-k8s.openshift-monitoring.svc:9092/federate?match[]=%7B__name__%3D~"up"%7D'
```

If the communication is working, and you receive a `502` response code, it probably means that the query is too big, and takes too much time to complete. There is a limit of `30s` set on the [oauth proxy](https://github.com/openshift/oauth-proxy/blob/master/options.go#L25) of the prometheus pod. There is a flag to customize this value, but it is not exposed by CMO. It is considered that a query that takes more than `30s` to complete is an abusive use of the federation endpoint.

## Communication checks on the Hub 

### The read path

Here is a list of curl requests that can be used to check the communication between the different components on the Hub. The requests are all fetching the `machine_cpu_cores` metric from the Thanos stores.

The read path is as follows: Grafana -> rbac-query-proxy -> Observatorium API -> Thanos-query-frontend -> Thanos-query -> Thanos stores.

From Grafana:

```bash
# Using the Service Account token
curl 'http://rbac-query-proxy.open-cluster-management-observability.svc.cluster.local:8080/api/v1/query?query=machine_cpu_cores' -H "Authorization: Bearer $(cat /var/run/secrets/kubernetes.io/serviceaccount/token)"

# Using a user token 
curl 'http://rbac-query-proxy.open-cluster-management-observability.svc.cluster.local:8080/api/v1/query?query=machine_cpu_cores' -H "Authorization: Bearer sha256~USER_TOKEN"
```

From the rbac-query-proxy pod:

```bash
curl --cert /var/rbac_proxy/certs/tls.crt --key /var/rbac_proxy/certs/tls.key --cacert /var/rbac_proxy/certs/ca.crt 'https://observability-observatorium-api.open-cluster-management-observability.svc.cluster.local:8080/api/metrics/v1/default/api/v1/query?query=machine_cpu_cores' 
```

From Thanos-query pod:

```bash
curl -s 'http://localhost:9090/api/v1/query?query=machine_cpu_cores'
```