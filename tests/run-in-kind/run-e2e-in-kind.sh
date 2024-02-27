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
source ${WORKDIR}/install-dependencies.sh
source ${WORKDIR}/create-cluster.sh

KUBECTL_VERSION=v1.29.2

setup_kubectl_command() {
  if ! command -v kubectl >/dev/null 2>&1; then
    echo "This script will install kubectl (https://kubernetes.io/docs/tasks/tools/install-kubectl/) on your machine"
    if [[ "$(uname)" == "Linux" ]]; then
      curl -LO "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/amd64/kubectl"
    elif [[ "$(uname)" == "Darwin" ]]; then
      curl -LO "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/darwin/$(uname -p)/kubectl"
    fi

    chmod +x ./kubectl
    sudo mv ./kubectl /usr/local/bin/kubectl
  fi
}

setup_e2e_test_env() {
  ${ROOTDIR}/cicd-scripts/setup-e2e-tests.sh
}

run_e2e_test() {
  ${ROOTDIR}/cicd-scripts/run-e2e-tests.sh
}

run() {
  setup_kubectl_command
  create_kind_cluster hub
  deploy_all
  setup_e2e_test_env
  run_e2e_test
}

run
