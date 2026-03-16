# Metrics tools

Collection of simple CI and developer tools and scripts to validate the list of metrics collected for a list of dashboards.
This helps maintain the list of metrics and rules in sync with the dashboards, and ensure that we don't collect
more metrics than needed.

More precisely, it helps verifying that:
* Metrics needed in dashboards are collected, and not more
* Rules needed in dashboards are defined, and not more
* Query rules are valid
* Metrics needed in dashboards are effectively present across all managed clusters (`clustercheck`)

## Tools

* `clustercheck`: Developer debug tool that provides a simple CLI to ensure that a list of metrics used in dashboards is effectively collected and available for each managed cluster in the Prometheus endpoint.
  * Set up a local port-forward to your Prometheus or Thanos instance: `oc port-forward -n open-cluster-management-observability svc/observability-thanos-query 9090:9090`
  * Run locally from the repo root: `go run ./cicd-scripts/metrics/cmd/clustercheck/main.go --scrape-configs <path> --prometheus-url http://localhost:9090`
  * You can filter the managed clusters to check by providing a regex: `go run ./cicd-scripts/metrics/cmd/clustercheck/main.go --scrape-configs <path> --prometheus-url http://localhost:9090 --cluster-regex "my-cluster-.*"`
  * Or build and execute: `go build -o clustercheck ./cicd-scripts/metrics/cmd/clustercheck/main.go && ./clustercheck ...`

## Usage

When adding a new list of dashboards in a new directory, make sure that you define the corresponding scrapeConfig and rules.
Then add a target in the Makefile to run the metrics check for the new dashboards, following the existing examples.
