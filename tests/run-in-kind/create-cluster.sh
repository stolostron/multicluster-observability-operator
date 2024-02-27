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
  if ! command -v kind >/dev/null 2>&1; then
    echo "This script will install kind (https://kind.sigs.k8s.io/) on your machine."
    curl -Lo "./kind-$(uname -p)64" "https://kind.sigs.k8s.io/dl/v0.10.0/kind-$(uname)-$(uname -p)64"
    chmod +x "./kind-$(uname -p)64"
    sudo mv "./kind-$(uname -p)64" /usr/local/bin/kind
  fi
  echo "Delete the KinD cluster if exists"
  kind delete cluster --name $1 || true
  rm -rf $HOME/.kube/kind-config-$1

  echo "Start KinD cluster with the default cluster name - $1"
  kind create cluster --kubeconfig $HOME/.kube/kind-config-$1 --name $1 --config ${WORKDIR}/kind/kind-$1.config.yaml
  export KUBECONFIG=$HOME/.kube/kind-config-$1
}

$*
