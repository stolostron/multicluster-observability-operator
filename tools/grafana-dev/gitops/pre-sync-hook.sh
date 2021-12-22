#!/usr/bin/env bash
# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

obs_namespace='open-cluster-management-observability'

# argocd need these label to monitor grafan-dev resource
kubectl -n "$obs_namespace" label secret/grafana-dev-config app.kubernetes.io/instance=grafana-dev
kubectl -n "$obs_namespace" label deployment.apps/grafana-dev app.kubernetes.io/instance=grafana-dev
kubectl -n "$obs_namespace" label service/grafana-dev app.kubernetes.io/instance=grafana-dev
kubectl -n "$obs_namespace" label ingress.networking.k8s.io/grafana-dev app.kubernetes.io/instance=grafana-dev
kubectl -n "$obs_namespace" label persistentvolumeclaim/grafana-dev app.kubernetes.io/instance=grafana-dev

# argocd need these annotation to ignore grafan-dev resource when sync application status
kubectl -n "$obs_namespace" annotate secret/grafana-dev-config argocd.argoproj.io/compare-options=IgnoreExtraneous
kubectl -n "$obs_namespace" annotate deployment.apps/grafana-dev argocd.argoproj.io/compare-options=IgnoreExtraneous
kubectl -n "$obs_namespace" annotate service/grafana-dev argocd.argoproj.io/compare-options=IgnoreExtraneous
kubectl -n "$obs_namespace" annotate ingress.networking.k8s.io/grafana-dev argocd.argoproj.io/compare-options=IgnoreExtraneous
kubectl -n "$obs_namespace" annotate persistentvolumeclaim/grafana-dev argocd.argoproj.io/compare-options=IgnoreExtraneous

# argocd need these annotation to prevent prune grafan-dev resource when sync application status
kubectl -n "$obs_namespace" annotate secret/grafana-dev-config argocd.argoproj.io/sync-options='Prune=false'
kubectl -n "$obs_namespace" annotate deployment.apps/grafana-dev argocd.argoproj.io/sync-options='Prune=false'
kubectl -n "$obs_namespace" annotate service/grafana-dev argocd.argoproj.io/sync-options='Prune=false'
kubectl -n "$obs_namespace" annotate ingress.networking.k8s.io/grafana-dev argocd.argoproj.io/sync-options='Prune=false'
kubectl -n "$obs_namespace" annotate persistentvolumeclaim/grafana-dev argocd.argoproj.io/sync-options='Prune=false'
