#!/bin/bash
# Copyright (c) 2024 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

# This script bootstraps a KinD environment with the required
# resources to run MulticlusterObservability components.

set -exo pipefail

ROOTDIR="$(pwd -P)"

WORKDIR=${ROOTDIR}/tests/run-in-kind

create_kind_cluster() {
  echo "Delete the KinD cluster if exists"
  kind delete cluster --name $1 || true
  rm -rf $HOME/.kube/kind-config-$1

  echo "Start KinD cluster with the default cluster name - $1"
  kind create cluster --kubeconfig $HOME/.kube/kind-config-$1 --name $1 --config ${WORKDIR}/kind/kind-$1.config.yaml
  export KUBECONFIG=$HOME/.kube/kind-config-$1
}

deploy_service_ca_operator() {
  kubectl create ns openshift-config-managed
  kubectl apply -f ${WORKDIR}/service-ca/
}

deploy_crds() {
  kubectl apply -f ${WORKDIR}/req_crds/
}

deploy_templates() {
  kubectl apply -f ${WORKDIR}/templates/
}

deploy_openshift_router() {
  kubectl create ns openshift-ingress
  kubectl apply -f ${WORKDIR}/router/
}

create_kind_cluster_managed() {
  echo "Coleen Delete the KinD cluster if exists"
  kind delete cluster --name $1 || true
  rm -rf $HOME/.kube/kind-config-$1

  echo "Start KinD cluster with the default cluster name - $1"
  kind create cluster --kubeconfig $HOME/.kube/kind-config-$1 --name $1 --config ${WORKDIR}/kind/kind-$1.config.yaml
}

run() {
  create_kind_cluster hub
  deploy_crds
  deploy_templates
  deploy_service_ca_operator
  deploy_openshift_router
  create_kind_cluster_managed managed
}

run
