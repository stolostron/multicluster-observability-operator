#!/usr/bin/env bash

set -exo pipefail

ROOTDIR="$(
  cd "$(dirname "$0")/../.."
  pwd -P
)"
WORKDIR=${ROOTDIR}/tests/run-in-kind

export IS_KIND_ENV=true
KIND_VERSION=v0.22.0

# shellcheck disable=SC1091
source ${WORKDIR}/env.sh

create_kind_cluster() {
  if ! command -v kind >/dev/null 2>&1; then

    echo "This script will install KinD (https://kind.sigs.k8s.io/docs/user/quick-start/) on your machine"
    if [[ "$(uname)" == "Linux" ]]; then
      curl -Lo "./kind" "https://kind.sigs.k8s.io/dl/${KIND_VERSION}/kind-linux-amd64"
    elif [[ "$(uname)" == "Darwin" ]]; then
      curl -Lo "./kind" "https://kind.sigs.k8s.io/dl/${KIND_VERSION}/kind-darwin-$(uname -m)"
    fi

    chmod +x "./kind" && mv "./kind" /usr/local/bin/kind
  fi
  echo "Delete the KinD cluster if exists"
  kind delete cluster --name $1 || true
  rm -rf $HOME/.kube/kind-config-$1

  echo "Start KinD cluster with the default cluster name - $1"
  kind create cluster --kubeconfig $HOME/.kube/kind-config-$1 --name $1 --config ${WORKDIR}/kind/kind-$1.config.yaml
  export KUBECONFIG=$HOME/.kube/kind-config-$1
}

$*
