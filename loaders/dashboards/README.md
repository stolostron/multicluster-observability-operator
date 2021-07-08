# grafana-dashboard-loader

Sidecar proxy to load grafana dashboards from configmaps.
## Prerequisites

- You must install [Open Cluster Management Observabilty](https://github.com/open-cluster-management/multicluster-observability-operator)

## How to build image

```
$ docker build -f Dockerfile.prow -t grafana-dashboard-loader:latest .
```

Now, you can use this image to replace the grafana-dashboard-loader component and verify your PRs.
