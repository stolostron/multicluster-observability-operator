## Dynamic Metric Collection (Custom Metrics Collection)

Dynamic metrics collection refers to the ability to trigger collecting metrics on managed clusters based on specific conditions. Collecting metrics consumes resources on hub cluster. This is especially important when considering collecting metrics across a large fleet of clusters. It makes sense to start collect certain metrics only when they are likely going to be needed for optmially using resources. When problems occur on a managed cluster, it may be necessary to collect metrics at a higher rate to help analyze the problems. Dynamic metrics collection enables both these use cases. Metrics collection stops automatically 15 minutes after the underlying condition no longer exists.

Dynamic metric collection is supported with a group of collection rules (`collection_rules`). A collection rule is a named entity that specifies

* `expr` - A collection rule condition in the form of a PromQL expression
* `dynamic_metrics` - A list of metrics or recording rules that must be collected when the rule persists
* `for` - The interval of time the rule must be active before the metrics collection could begin

The collection rules are grouped together as a parameter section named `collect_rules`, where it can be enabled or disabled as a group. A collection rule group also can specify a cluster selector match expression `selector.matchExpressions` and it allows for a collect rule group to be applied only on managed clusters that match the criteria.

By default, collection rules are evaluated continuously on managed clusters every 30 seconds, or at the time interval that is specified in MCO custom resource, whichever is less. Once the collection rule condition persists for the duration specified by the `for` attribute, the collection rule starts and the metrics specified by the rule are automatically collected on the managed cluster. Metrics collection stops automatically after the collection rule condition no longer exists on the managed cluster, at least 15 minutes after it starts.

You can specify collection rules via custom_metrics ConfigMap. The process is similar to how custom metrics are added to an existing deployment. Below are the collection rules for single-node OpenShift clusters shipped with the release that you can use as a reference to design your own custom rules.

```
----
collect_rules:
  - group: SNOResourceUsage
    annotations:
      description: >
        By default, a SNO cluster does not collect pod and container resource metrics. Once a SNO cluster 
        reaches a level of resource consumption, these granular metrics are collected dynamically. 
        When the cluster resource consumption is consistently less than the threshold for a period of time, 
        collection of the granular metrics stops.
    selector:
      matchExpressions:
        - key: clusterType
          operator: In
          values: ["SNO"]
    rules:
    - collect: SNOHighCPUUsage
      annotations:
        description: >
          Collects the dynamic metrics specified if the cluster cpu usage is constantly more than 70% for 2 minutes
      expr: (1 - avg(rate(node_cpu_seconds_total{mode=\"idle\"}[5m]))) * 100 > 70
      for: 2m
      dynamic_metrics:
        names:
          - container_cpu_cfs_periods_total
          - container_cpu_cfs_throttled_periods_total
          - kube_pod_container_resource_limits 
          - kube_pod_container_resource_requests   
          - namespace_workload_pod:kube_pod_owner:relabel 
          - node_namespace_pod_container:container_cpu_usage_seconds_total:sum_irate 
          - node_namespace_pod_container:container_cpu_usage_seconds_total:sum_rate 
    - collect: SNOHighMemoryUsage
      annotations:
        description: >
          Collects the dynamic metrics specified if the cluster memory usage is constantly more than 70% for 2 minutes
      expr: (1 - sum(:node_memory_MemAvailable_bytes:sum) / sum(kube_node_status_allocatable{resource=\"memory\"})) * 100 > 70
      for: 2m
      dynamic_metrics:
        names:
          - kube_pod_container_resource_limits 
          - kube_pod_container_resource_requests 
          - namespace_workload_pod:kube_pod_owner:relabel
        matches:
          - __name__="container_memory_cache",container!=""
          - __name__="container_memory_rss",container!=""
          - __name__="container_memory_swap",container!=""
          - __name__="container_memory_working_set_bytes",container!=""
----

### Installation

No special installation is necessary to use this feature
