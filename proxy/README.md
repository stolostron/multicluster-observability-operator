# RBAC Query Proxy

The `rbac-query-proxy` is an HTTP reverse proxy that sits between Grafana and the Observatorium API. Its primary purpose is to enforce Role-Based Access Control (RBAC) for multicluster metrics queries and to dynamically generate a synthetic metric that enables powerful, multi-level filtering of clusters in Grafana.

## Core Functionality

### Dynamic Grafana Filtering via Label Synchronization

A key feature of the proxy is its ability to enable dynamic, multi-level filtering of managed clusters within Grafana dashboards. This is achieved through a label synchronization process that works as follows:

1.  **Label Discovery:** An informer within the proxy continuously watches all `ManagedCluster` resources across the fleet.
2.  **Allowlist Persistence:** When a new label is discovered on a `ManagedCluster`, its key is added to the `observability-managed-cluster-label-allowlist` ConfigMap. Crucially, labels are never removed from this ConfigMap, even if they are no longer present on any cluster. This ensures that labels used for historical metrics remain available for querying.
3.  **Synthetic Metric Generation:** The proxy reads the list of keys from this ConfigMap and uses it to generate a synthetic (on-the-fly) metric named `acm_label_names`.

This mechanism powers a cascading variable filtering experience in Grafana:

-   **Select a Label Key:** A dashboard variable allows users to select a label key (e.g., `cloud`, `region`) from a dropdown populated by the `acm_label_names` metric.
-   **Select a Label Value:** A second variable shows the possible values for the selected label key (e.g., `Amazon`, `Azure`). This is populated using the `acm_managed_cluster_labels` metric, which is a real metric provided by ACM that reflects the current labels on clusters.
-   **Select Clusters:** A third variable shows the specific clusters that match the selected label key and value.
-   **Filter Panels:** Dashboard queries are then filtered using the selected clusters.

This workflow enables users to intuitively drill down from high-level labels to specific sets of clusters when visualizing metrics.

### RBAC Enforcement

In addition to generating the synthetic metric, the proxy enforces access control by inspecting user requests and injecting appropriate label matchers into the PromQL queries. This ensures that users can only see metrics from the clusters and namespaces they are authorized to access.

## Configuration

The `rbac-query-proxy` is configured via command-line flags.

| Flag               | Default                  | Description                                                                    |
| ------------------ | ------------------------ | ------------------------------------------------------------------------------ |
| `--listen-address` | `0.0.0.0:3002`           | The address for the HTTP server to listen on.                                  |
| `--metrics-server` |                          | The upstream URL of the Observatorium API. (Required)                            |
| `--kubeconfig`     |                          | Path to a kubeconfig file. If unset, in-cluster configuration will be used.    |
| `--v`              | `0`                      | Sets the log verbosity level. Higher values produce more detailed log output.  |

## How to Build

You can build the container image using the provided Dockerfile:

```bash
docker build -f Dockerfile -t rbac-query-proxy:latest .
```