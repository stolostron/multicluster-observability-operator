# HCP Monitoring Support Tool

This tool provides a support exception for monitoring Hosted Control Planes (HCP) on managed clusters. It creates specific `ServiceMonitors` with the correct cluster identification labels required by ACM.

## Important: Support Scope

- **Architecture Compatibility**: This tool is **only supported** when using the legacy **metrics-collector** architecture. It is NOT compatible with the new **MCOA** (Multi-Cluster Observability Addon) architecture.
- **Native Support Versions**: 
  - **metrics-collector**: This feature (automatic monitoring of Hosted Clusters on managed clusters) is natively supported starting from versions **2.17.1** and **2.16.3**.
  - **MCOA**: Native support for this feature is planned for version **5.0**.

## Prerequisites

To build and run this tool, the following must be installed on your system:
- **Go**: Version 1.23 or higher (as specified in the project's `go.mod`).
- **Git**: To clone the repository and manage dependencies.
- **Access**: A user with `cluster-admin` permissions on the managed cluster.

## Authentication

The tool uses the standard Kubernetes client-go authentication logic. It will look for cluster credentials in the following order:
1. The `KUBECONFIG` environment variable pointing to a valid kubeconfig file.
2. The default `~/.kube/config` file.

**Important**: Ensure your current context is set to the managed cluster where the Hosted Control Planes are running.

## Usage

### Run directly
You can run the tool directly using the Go toolchain:
```bash
export KUBECONFIG=/path/to/managed-cluster-kubeconfig
go run -mod=mod operators/endpointmetrics/cmd/hcp-monitoring-support/main.go --setup
```

### Build and run binary
Alternatively, you can build a standalone binary:
```bash
go build -mod=mod -o hcp-monitoring-support operators/endpointmetrics/cmd/hcp-monitoring-support/main.go
./hcp-monitoring-support --setup
```

### Commands
- `--setup`: Create or update the required `ServiceMonitors` for all `HostedClusters` found on the cluster.
- `--cleanup`: Delete the ACM-specific `ServiceMonitors`.

### Options
- `--auto-approve`: Automatically approve write actions (skips the confirmation prompt).
- `--dry-run`: Display what actions would be taken without executing any changes on the cluster.
