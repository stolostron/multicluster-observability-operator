# Observability Overview

This document attempts to explain how the different components in Open Cluster Management Observabilty come together to deliver multicluster fleet observability. We do leverage several open source projects: [Grafana](https://github.com/grafana/grafana), [Alertmanager](https://github.com/prometheus/alertmanager), [Thanos](https://github.com/thanos-io/thanos/), [Observatorium Operator and API Gateway](https://github.com/observatorium), [Prometheus](https://github.com/prometheus/prometheus); We also leverage a few [Open Cluster Mangement projects](https://open-cluster-management.io/) namely - [Cluster Manager or Registration Operator](https://github.com/open-cluster-management/registration-operator), [Klusterlet](https://github.com/open-cluster-management/registration-operator), [multicloud operators placementrule](https://github.com/open-cluster-management/multicloud-operators-placementrule)

## Conceptual Diagram

![Conceptual Diagram of the Components](images/observability_overview_in_ocm.png)

## Associated Github repository

Component |Git Repo	| Description	
---  | ------ | ----  
MCO Operator | [multicluster-observability-operator](https://github.com/open-cluster-management/multicluster-observability-operator) | Operator for monitoring. This is the root repo. If we follow the Readme instructions here to install, the code from all other repos mentioned below are used/referenced.
Endpoint Operator | [endpoint-metrics-operator](https://github.com/open-cluster-management/endpoint-metrics-operator) | Operator that manages  setting up observability and data collection at the managed clusters.
Observatorium Operator | [observatorium-operator](https://github.com/open-cluster-management/observatorium-operator) | Operator to deploy the Observatorium project. Inside the open cluster management, at this time, it means metrics using Thanos. 
Metrics collector | [metrics-collector](https://github.com/open-cluster-management/metrics-collector) | Scrapes metrics from Prometheus at managed clusters, the metric collection being shaped by configuring allow-list. 
RBAC Proxy | [rbac_query_proxy](https://github.com/open-cluster-management/rbac-query-proxy) | Helper service that acts a multicluster metrics RBAC proxy.
Grafana | [grafana](https://github.com/open-cluster-management/grafana) | Grafana repo -  for  dashboarding and metric analytics.
Dashboard Loader | [grafana-dashboard-loader](https://github.com/open-cluster-management/grafana-dashboard-loader) | Sidecar proxy to load grafana dashboards from configmaps. 
Management Ingress | [management-ingress](https://github.com/open-cluster-management/management-ingress) | NGINX based ingress controller to serve Open Cluster Management services. 
Observatorium API | [observatorium](https://github.com/open-cluster-management/observatorium) | API Gateway which controls reading, writing of the Observability data to the backend infrastructure. 
Thanos Ecosystem | [kube-thanos](https://github.com/open-cluster-management/kube-thanos) | Kubernetes specific configuration for deploying Thanos. Contains Thanos receivers, store, compactor, querier, rules etc. 