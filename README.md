# Observability Overview

[![Build](https://img.shields.io/badge/build-Prow-informational)](https://prow.ci.openshift.org/?repo=stolostron%2F${multicluster-observability-operator})
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)

This document explains how the different components in Open Cluster Management Observability come together to deliver multicluster fleet observability. We leverage several open source projects: [Grafana](https://github.com/grafana/grafana), [Alertmanager](https://github.com/prometheus/alertmanager), [Thanos](https://github.com/thanos-io/thanos/), [Observatorium Operator and API Gateway](https://github.com/observatorium), [Prometheus](https://github.com/prometheus/prometheus). We also leverage [Open Cluster Management projects](https://open-cluster-management.io/) namely - [Cluster Manager or Registration Operator](https://github.com/stolostron/registration-operator), [Klusterlet](https://github.com/stolostron/registration-operator). The multicluster-observability operator is the root operator which pulls in all things needed.

## Architecture

### Component Orchestration

The following hierarchy describes how components are deployed and managed:

*   **MCO (Multicluster Observability Operator)**: The root operator on the hub cluster. It orchestrates the deployment of:
    *   **Observatorium Operator**
    *   **Grafana** and the **Dashboard Loader**
    *   **AlertManager**
    *   **MCOA Addon Manager** (when enabled)
    *   **Endpoint Operator (on the Hub)**: Deployed directly by MCO's controller client (bypassing ManifestWorks) to satisfy Global Hub self-management constraints.
*   **Observatorium Operator**: Manages the life cycle of the data plane components:
    *   **Observatorium API**
    *   **Thanos ecosystem** (Query, Store, Compact, Receive, etc.)
*   **Endpoint Operator**: Manages observability on managed clusters.
    *   **Deployment**: Distributed to managed clusters via `ManifestWorks` and applied by the `work-agent` (except on the Hub).
    *   **Responsibility**: Deploys and configures the **Metrics Collector**. It can deploy separate collector instances for platform monitoring and **user workload monitoring**. It also configures the **Cluster Monitoring Operator (CMO)** to ensure both platform and user workload Prometheus instances forward alerts to the Hub's Alertmanager.

### Collection Architectures

The project currently supports two architectures for metrics collection:

1.  **MCOA (Multi-Cluster Observability Addon) - New Standard**: Leverages the upstream `addon-framework` and `monitoring.rhobs` APIs (`PrometheusAgent`, `ScrapeConfig`) for a more standard and scalable approach.
2.  **Legacy Architecture**: Uses the `observability-endpoint-operator` and custom `metrics-collector` deployed via `ManifestWorks`.

### Data Flow

```mermaid
---
config:
  theme: 'base'
  themeVariables:
    primaryColor: '#2D70B5'
    background: 'white'
    primaryTextColor: '#fff'
    lineColor: '#343628'
    secondaryColor: 'transparent'
    tertiaryColor: '#fdfdfd'
---
block
    columns 7
    
    %% -- COLUMN 1: MANAGED CLUSTER --
    block:MC
        columns 1
        ManagedCluster(["Managed Cluster"])
        EndpointOp("Endpoint Operator")
        space
        MetCol("Metrics Collector")
        space
        PlatProm("Platform Prometheus")
    end

    space

    %% -- COLUMN 2: HUB SERVER --
    block:HUB:3
        columns 5
        space:2
        HubCluster(["Hub Cluster"])
        space:2
        MCO("MCO Operator")
        space
        RBAC["RBAC Query Proxy"]
        space
        block:Grafana
            columns 1
            Loader["Dashboard Loader"]
            Graf["Grafana"]
        end
        
        ObsOp["Observatorium Operator"]
        space:4

        space:2
        ObsAPI["Observatorium API"]
        space:2

        space:5

        AlertMgr["Alert manager"]
        space:1
        Thanos["Thanos ecosystem"]
        space:1
    end

    space

    %% -- COLUMN 3: External systems --
    block:Right
        columns 1
        ExtSystems(["External Systems"])
        WebBrowser("Web Browser")
        space
        ExtEndpoint("External Endpoint")
        space
        ObjStore[("Object Storage")]
    end

    %% -- STYLING --
    classDef mainBoxLabel fill:#B5362D,color:white,stroke:#333,stroke-width:0px;
    class ManagedCluster mainBoxLabel
    class HubCluster mainBoxLabel
    class ExtSystems mainBoxLabel
    classDef controlPlane fill:#ccc,color:#666,stroke:#bbb,stroke-width:2px,stroke-dasharray: 5 5;
    class MCO,EndpointOp,ObsOp,Loader controlPlane    

    %% -- CONNECTIONS --
    MetCol -- "federates metrics" --> PlatProm
    MetCol -- "remote writes" --> ObsAPI
    PlatProm -- "alerts" --> AlertMgr
    WebBrowser -- "accesses" --> Grafana
    Grafana -- "queries via" --> RBAC
    RBAC -- "proxies to" --> ObsAPI
    ObsAPI -- "forwards" --> Thanos
    ObsAPI -- "remote writes (optional)" --> ExtEndpoint
    Thanos -- "sends alerts" --> AlertMgr
    Thanos -- "stores" --> ObjStore
```

## Associated Github Repositories

| Component | Git Repo | Description | Status |
| :--- | :--- | :--- | :--- |
| **MCO Operator** | [multicluster-observability-operator](https://github.com/stolostron/multicluster-observability-operator) | **Root repo**. Operator for monitoring and orchestration. | Active |
| **MCOA** | [multicluster-observability-addon](https://github.com/stolostron/multicluster-observability-addon) | New metrics collection addon using standard upstream APIs. | Active |
| **Endpoint Operator** | [endpoint-metrics-operator](https://github.com/stolostron/multicluster-observability-operator/tree/main/operators/endpointmetrics) | Manages observability setup on managed clusters. | **Legacy** |
| **Observatorium Operator** | [observatorium-operator](https://github.com/stolostron/observatorium-operator) | Deploys Observatorium (Thanos) components. Forked from main repo. | Active |
| **Metrics Collector** | [metrics-collector](https://github.com/stolostron/multicluster-observability-operator/tree/main/collectors/metrics) | Scrapes/filters metrics from managed clusters. | **Legacy** |
| **RBAC Proxy** | [rbac_query_proxy](https://github.com/stolostron/multicluster-observability-operator/tree/main/proxy) | Enforces ACM permissions on metric queries. | Active |
| **Grafana** | [grafana](https://github.com/stolostron/grafana) | Dashboarding and metric analytics. Forked from main repo. | Active |
| **Dashboard Loader** | [grafana-dashboard-loader](https://github.com/stolostron/multicluster-observability-operator/tree/main/loaders/dashboards) | Sidecar to load dashboards from configmaps. | Active |
| **Observatorium API** | [observatorium](https://github.com/stolostron/observatorium) | API Gateway for reading/writing observability data. Forked from main repo. | Active |
| **Thanos Ecosystem** | [kube-thanos](https://github.com/stolostron/kube-thanos) | Kubernetes configuration for deploying Thanos. | Active |

## Integration Guide for External Projects

External projects can integrate with ACM Observability to provide custom dashboards and collect additional metrics.

### 1. Dashboard Integration
*   Add your Grafana dashboards to the `operators/multiclusterobservability/manifests/base/grafana` directory. You can use existing folders or create new ones.
*   **UID:** Each new dashboard must define a **unique `uid`** value in its JSON definition.
*   **Folder:** The folder name visible in the Grafana UI is configured via the `observability.open-cluster-management.io/dashboard-folder` annotation in the dashboard ConfigMap/JSON.

> **Note:** The short to mid-term goal is to migrate all dashboards to [Perses](https://github.com/perses/perses) with MCOA. This will allow users to view dashboards directly within the integrated OpenShift console.

### 2. Metrics Integration
To ensure your dashboards display data, you must whitelist the required metrics for collection.

*   **Cardinality:** Users must ensure that added metrics have an **optimized cardinality**. Use aggregation rules (recording rules) when possible to keep the system efficient and scalable.
*   **Legacy (Metrics Collector):** Add metrics to `operators/multiclusterobservability/manifests/base/config/metrics_allowlist.yaml`.
*   **MCOA (New):** Add metrics to the `scrape-config.yaml` file located within the corresponding dashboard directory in `operators/multiclusterobservability/manifests/base/grafana`.

### 3. CI Validation
We enforce strict metric collection to minimize cardinality/cost.
*   Run `make check-metrics` to verify that only the metrics required by your dashboards are being collected.
*   **New Directories:** If you create a new directory for your dashboards, you must update `cicd-scripts/metrics/Makefile` to include this new path in the CI checks.

## Quick Start Guide

### Prerequisites

* Ensure [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl) and [kustomize](https://kubectl.docs.kubernetes.io/installation/kustomize/) are installed.
* Prepare a OpenShift cluster to function as the hub cluster.
* Ensure [docker 17.03+](https://docs.docker.com/get-started) is installed.
* Ensure [golang](https://golang.org/doc/install) is installed (See `go.mod` for exact version).
* Ensure the open-cluster-management cluster manager is installed. See [Cluster Manager](https://open-cluster-management.io/getting-started/core/cluster-manager/) for more information.
* Ensure the `open-cluster-management` _klusterlet_ is installed. See [Klusterlet](https://open-cluster-management.io/getting-started/core/register-cluster/) for more information.

> Note: By default, the API conversion webhook use on the OpenShift service serving certificate feature to manage the certificate, you can replace it with cert-manager if you want to run the multicluster-observability-operator in a kubernetes cluster.

Use the following quick start commands for building and testing the multicluster-observability-operator:

### Clone the Repository

Check out the multicluster-observability-operator repository.

```bash
git clone git@github.com:stolostron/multicluster-observability-operator.git
cd multicluster-observability-operator
```

### Build the Operator

Build the multicluster-observability-operator image and push it to a public registry, such as quay.io:

```bash
make docker-build docker-push IMG=quay.io/<YOUR_USERNAME_IN_QUAY>/multicluster-observability-operator:latest
```

### Run the Operator in the Cluster

1. Create the `open-cluster-management-observability` namespace if it doesn't exist:

```bash
kubectl create ns open-cluster-management-observability
```

2. Deploy the minio service which acts as storage service of the multicluster observability:

```bash
kubectl -n open-cluster-management-observability apply -k examples/minio
```

3. Replace the operator image and deploy the multicluster-observability-operator:

```bash
make deploy IMG=quay.io/<YOUR_USERNAME_IN_QUAY>/multicluster-observability-operator:latest
```

4. Deploy the multicluster-observability-operator CR:

```bash
kubectl apply -f operators/multiclusterobservability/config/samples/observability_v1beta2_multiclusterobservability.yaml
```

5. Verify all the components for the Multicluster Observability are starting up and running:

```bash
kubectl -n open-cluster-management-observability get pod
```

### What is next

After a successful deployment, you can run the following command to check if you have OCP cluster as a managed cluster.

```bash
kubectl get managedcluster --show-labels
```

If there is no `vendor=OpenShift` label exists in your managed cluster, you can manually add this label with this command `kubectl label managedcluster <managed cluster name> vendor=OpenShift`

Then you should be able to have `metrics-collector` pod is running (Legacy Architecture):

```bash
kubectl -n open-cluster-management-addon-observability get pod
endpoint-observability-operator-5c95cb9df9-4cphg   1/1     Running   0          97m
metrics-collector-deployment-6c7c8f9447-brpjj      1/1     Running   0          96m
```

Expose the thanos query frontend via route by running this command:

```bash
cat << EOF | kubectl -n open-cluster-management-observability apply -f -
kind: Route
apiVersion: route.openshift.io/v1
metadata:
  name: query-frontend
spec:
  port:
    targetPort: http
  wildcardPolicy: None
  to:
    kind: Service
    name: observability-thanos-query-frontend
EOF
```

You can access the thanos query UI via browser by inputting the host from `oc get route -n open-cluster-management-observability query-frontend`. There should have metrics available when you search the metrics `:node_memory_MemAvailable_bytes:sum`. The available metrics are listed [here](https://github.com/stolostron/multicluster-observability-operator/blob/main/operators/multiclusterobservability/manifests/base/config/metrics_allowlist.yaml)

### Uninstall the Operator in the Cluster

1. Delete the multicluster-observability-operator CR:

```bash
kubectl -n open-cluster-management-observability delete -f operators/multiclusterobservability/config/samples/observability_v1beta2_multiclusterobservability.yaml
```

2. Delete the multicluster-observability-operator:

```bash
make undeploy
```

3. Delete the minio service:

```bash
kubectl -n open-cluster-management-observability delete -k examples/minio
```

4. Delete the `open-cluster-management-observability` namespace:

```bash
kubectl delete ns open-cluster-management-observability
```
