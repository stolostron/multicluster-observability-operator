# RBAC Query Proxy

The `rbac-query-proxy` is an HTTP reverse proxy that sits between Grafana and the Observatorium API. Its primary purpose is to enforce Role-Based Access Control (RBAC) for multicluster metrics queries and to dynamically generate a synthetic metric that enables powerful filtering capabilities in Grafana.

## Core Functionality

### Synthetic Metric for Grafana Filtering

The proxy introduces a synthetic metric named `acm_label_names`. This metric is not stored in Prometheus but is generated on-the-fly by the proxy. Its labels are dynamically populated from two sources:

1.  The `observability-managed-cluster-label-allowlist` ConfigMap in the `open-cluster-management-addon-observability` namespace.
2.  The labels present on the `ManagedCluster` custom resources.

This mechanism allows Grafana dashboards to use these labels as variables for filtering. Users can select managed clusters in a dashboard based on specific label names and their corresponding values (e.g., `cloud=Amazon`, `region=us-east-1`). This provides a flexible way to group and visualize metrics for different sets of clusters.

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