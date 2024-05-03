#!/bin/bash
# Copyright (c) 2024 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

# The functions in this script are used to install the various binaries that
# are required for the CI/CD pipeline to function, as well as execution of local e2e tests.
# If the binaries are already executable on the ${PATH} of the host, the script will skip the installation.
# Each function takes a path as the first argument, which is the directory where the binary will be installed.
# If no path is provided, fallback to ${BIN_DIR} or default path to /usr/local/bin.

OPERATOR_SDK_VERSION="${KUBECTL_VERSION:=v1.4.2}"
KUBECTL_VERSION="${KUBECTL_VERSION:=v1.28.2}"
KUSTOMIZE_VERSION="${KUSTOMIZE_VERSION:=v5.3.0}"
JQ_VERSION="${JQ_VERSION:=1.6}"
KIND_VERSION="${KIND_VERSION:=v0.22.0}"

BIN_DIR="${BIN_DIR:=/usr/local/bin}"

install_operator_sdk() {
  bin_dir=${1:-${BIN_DIR}}
  if ! command -v operator-sdk &>/dev/null; then
    echo "operator-sdk not found on path, installing operator-sdk version ${OPERATOR_SDK_VERSION}"
    if [[ "$(uname)" == "Linux" ]]; then
      curl -L "https://github.com/operator-framework/operator-sdk/releases/download/${OPERATOR_SDK_VERSION}/operator-sdk_linux_amd64" -o operator-sdk
    elif [[ "$(uname)" == "Darwin" ]]; then
      curl -L "https://github.com/operator-framework/operator-sdk/releases/download/${OPERATOR_SDK_VERSION}/operator-sdk_darwin_$(uname -m)" -o operator-sdk
    fi
    chmod +x operator-sdk
    sudo mv operator-sdk ${bin_dir}/operator-sdk
  fi
}

install_kubectl() {
  bin_dir=${1:-${BIN_DIR}}
  if ! command -v kubectl &>/dev/null; then
    echo "This script will install kubectl (https://kubernetes.io/docs/tasks/tools/install-kubectl/) on your machine"
    if [[ "$(uname)" == "Linux" ]]; then
      curl -LO "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/amd64/kubectl"
    elif [[ "$(uname)" == "Darwin" ]]; then
      curl -LO "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/darwin/$(uname -m)/kubectl"
    fi
    chmod +x ./kubectl && mv ./kubectl ${bin_dir}/kubectl
  fi
}

install_kustomize() {
  bin_dir=${1:-${BIN_DIR}}
  if ! command -v kustomize &>/dev/null; then
    echo "This script will install kustomize (sigs.k8s.io/kustomize/kustomize) on your machine"
    if [[ "$(uname)" == "Linux" ]]; then
      curl -o kustomize_${KUSTOMIZE_VERSION}.tar.gz -L "https://github.com/kubernetes-sigs/kustomize/releases/download/kustomize%2F${KUSTOMIZE_VERSION}/kustomize_${KUSTOMIZE_VERSION}_linux_amd64.tar.gz"
    elif [[ "$(uname)" == "Darwin" ]]; then
      curl -o kustomize_${KUSTOMIZE_VERSION}.tar.gz -L "https://github.com/kubernetes-sigs/kustomize/releases/download/kustomize%2F${KUSTOMIZE_VERSION}/kustomize_${KUSTOMIZE_VERSION}_darwin_$(uname -m).tar.gz"
    fi
    tar xzvf kustomize_${KUSTOMIZE_VERSION}.tar.gz
    chmod +x ./kustomize && mv ./kustomize ${bin_dir}/kustomize
  fi
}

install_jq() {
  bin_dir=${1:-${BIN_DIR}}
  if ! command -v jq &>/dev/null; then
    echo "This script will install jq on your machine"
    if [[ "$(uname)" == "Linux" ]]; then
      curl -o jq -L "https://github.com/stedolan/jq/releases/download/jq-${JQ_VERSION}/jq-linux64"
    elif [[ "$(uname)" == "Darwin" ]]; then
      curl -o jq -L "https://github.com/stedolan/jq/releases/download/jq-${JQ_VERSION}/jq-osx-$(uname -m)"
    fi
    chmod +x ./jq && mv ./jq ${bin_dir}/jq
  fi
}

install_kind() {
  bin_dir=${1:-${BIN_DIR}}
  if ! command -v kind &>/dev/null; then
    echo "This script will install KinD on your machine"
    if [[ "$(uname)" == "Linux" ]]; then
      curl -o kind -L "https://kind.sigs.k8s.io/dl/${KIND_VERSION}/kind-linux-amd64"
    elif [[ "$(uname)" == "Darwin" ]]; then
      curl -o kind -L "https://kind.sigs.k8s.io/dl/${KIND_VERSION}/kind-darwin-$(uname -m)"
    fi
    chmod +x ./kind && mv ./kind ${bin_dir}/kind
  fi
}

install_gojsontoyaml() {
  bin_dir=${1:-${BIN_DIR}}
  if ! command -v gojsontoyaml &>/dev/null; then
    if [[ "$(uname)" == "Linux" ]]; then
      curl -L https://github.com/brancz/gojsontoyaml/releases/download/v0.1.0/gojsontoyaml_0.1.0_linux_amd64.tar.gz | tar -xz -C ${bin_dir} gojsontoyaml
    elif [[ "$(uname)" == "Darwin" ]]; then
      curl -L https://github.com/brancz/gojsontoyaml/releases/download/v0.1.0/gojsontoyaml_0.1.0_darwin_$(uname -m).tar.gz | tar -xz -C ${bin_dir} gojsontoyaml
    fi
  fi
}

install_build_deps() {
  bin_dir=${1:-${BIN_DIR}}
  install_operator_sdk ${bin_dir}
  # kustomize is required to build the bundle
  install_kustomize ${bin_dir}
}

install_integration_tests_deps() {
  bin_dir=${1:-${BIN_DIR}}
  install_kubectl ${bin_dir}
  install_kind ${bin_dir}
}

install_e2e_tests_deps() {
  bin_dir=${1:-${BIN_DIR}}
  install_kubectl ${bin_dir}
  install_jq ${bin_dir}
  install_kind ${bin_dir}
  install_kustomize ${bin_dir}
}

# This allows functions within this file to be called individually from Makefile(s).
$*
