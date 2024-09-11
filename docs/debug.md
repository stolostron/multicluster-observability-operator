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