#!/usr/bin/env bash

set -exo pipefail

ROOTDIR="$(
  cd "$(dirname "$0")/../.."
  pwd -P
)"
WORKDIR=${ROOTDIR}/tests/run-in-kind

export IS_KIND_ENV=true

# shellcheck disable=SC1091
source ${WORKDIR}/env.sh

create_kind_cluster() {
  echo "Delete the KinD cluster if exists coleen"
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

setup_e2e_test_env() {
  ${ROOTDIR}/cicd-scripts/setup-e2e-tests.sh
}

run_e2e_test() {
  ${ROOTDIR}/cicd-scripts/run-e2e-tests.sh
}

run() {
  create_kind_cluster hub
  create_kind_cluster_managed managed
  deploy_crds
  deploy_templates
  deploy_service_ca_operator
  deploy_openshift_router
  setup_e2e_test_env
  run_e2e_test
}

run
