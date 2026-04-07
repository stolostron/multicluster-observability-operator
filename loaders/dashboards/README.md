# Grafana Dashboard Loader

The Grafana Dashboard Loader is a high-performance, stateless sidecar that synchronizes Kubernetes ConfigMaps containing Grafana JSON definitions into a local Grafana instance. It implements an **Exclusive Ownership** model where Kubernetes is the absolute source of truth.

## Core Features

- **Kubernetes-Native Architecture**: Uses a Kubernetes workqueue with exponential backoff for robust, resilient synchronization.
- **Exclusive Ownership**: The loader assumes total control over the Grafana instance's dashboard space.
- **Dynamic Folder Management**: Supports moving dashboards between folders using the `observability.open-cluster-management.io/dashboard-folder` annotation.
- **Hybrid Deletion Model**: Combines immediate in-memory tracking for responsive deletions with a 10-minute "Total Sweep" safety net for correctness.
- **Home Dashboard Sync**: Automatically sets a specific dashboard as the Grafana "Home" page based on ConfigMap annotations.

## How it works (Source of Truth)

Unlike traditional loaders that rely on tagging or external databases, this loader treats the **Kubernetes API** as the absolute source of truth.

1.  **Deterministic UIDs**: Every dashboard is assigned a stable UID derived from its source ConfigMap (`namespace/name/key`). This ensures that dashboards maintain their identity and URL even if the loader restarts.
2.  **Total Control**: The loader periodically lists *all* dashboards in the Grafana instance. Any dashboard that does not have a corresponding ConfigMap in Kubernetes is considered an "orphan" and is automatically deleted.
3.  **Always Overwrite**: To ensure consistency, the loader always overwrites existing dashboards in Grafana with the content from Kubernetes. Manual changes made in the Grafana UI are intentionally disregarded and will be reverted on the next sync cycle.

### ⚠️ Operational Warning
Because this loader assumes **Exclusive Ownership**, any dashboard created manually in the Grafana UI (without a corresponding ConfigMap) will be deleted during the periodic "Total Sweep" cycle. Always manage your dashboards via Kubernetes manifests.

## Configuration

### Desired ConfigMaps
The loader watches for ConfigMaps that match **either** of the following criteria:
1. Label `grafana-custom-dashboard: "true"` is present.
2. The ConfigMap is owned by a `MultiClusterObservability` resource and its name contains `grafana-dashboard`.

### Supported Annotations & Labels
- `observability.open-cluster-management.io/dashboard-folder`: (Annotation) The title of the Grafana folder where the dashboard should be placed. Defaults to `Custom`.
- `general-folder: "true"`: (Label) If set to true, the dashboard is placed in the "General" folder (ID 0).
- `set-home-dashboard: "true"`: (Annotation) If set on the ConfigMap, the loader will attempt to set the primary dashboard in this CM as the Grafana Home page.

## Development

### How to build image
```bash
make docker-build-local  # Targeted at the loader component
```

### Running Tests
- **Unit Tests**: `go test ./loaders/dashboards/pkg/controller/...`
- **Integration Tests**: `make integration-test-loaders` (Uses `envtest` to run against a real control plane).
