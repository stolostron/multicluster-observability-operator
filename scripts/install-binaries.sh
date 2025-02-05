#!/bin/bash
# Copyright (c) 2024 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

# The functions in this script are used to install the various binaries that
# are required for the CI/CD pipeline to function, as well as execution of local e2e tests.
# If the binaries are already executable on the ${PATH} of the host, the script will skip the installation.
# Each function takes a path as the first argument, which is the directory where the binary will be installed.
# If no path is provided, fallback to ${BIN_DIR} or default path to /usr/local/bin.
KUBECTL_VERSION="${KUBECTL_VERSION:=v1.28.2}"
JQ_VERSION="${JQ_VERSION:=1.7.1}"
YQ_VERSION="${YQ_VERSION:=4.45.1}"
MIMIRTOOL_VERSION="${MIMIRTOOL_VERSION:=2.14.3}"
PROMTOOL_VERSION="${PROMTOOL_VERSION:=3.1.0}"

BIN_DIR="${BIN_DIR:=/usr/local/bin}"

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

install_jq() {
  bin_dir=${1:-${BIN_DIR}}
  if ! command -v jq &>/dev/null; then
    echo "This script will install jq on your machine"
    if [[ "$(uname)" == "Linux" ]]; then
      curl -o jq -L "https://github.com/stedolan/jq/releases/download/jq-${JQ_VERSION}/jq-linux64"
    elif [[ "$(uname)" == "Darwin" ]]; then
      curl -o jq -L "https://github.com/stedolan/jq/releases/download/jq-${JQ_VERSION}/jq-macos-$(uname -m)"
    fi
    chmod +x ./jq && mv ./jq ${bin_dir}/jq
  fi
}

install_yq() {
  bin_dir=${1:-${BIN_DIR}}
  if ! command -v yq &>/dev/null; then
    echo "This script will install yq on your machine"
    if [[ "$(uname)" == "Linux" ]]; then
      curl -o yq -L "https://github.com/mikefarah/yq/releases/download/${YQ_VERSION}/yq_linux_amd64"
    elif [[ "$(uname)" == "Darwin" ]]; then
      curl -o yq -L "https://github.com/mikefarah/yq/releases/download/${YQ_VERSION}/yq_darwin_$(uname -m)"
    fi
    chmod +x ./yq && mv ./yq ${bin_dir}/yq
  fi
}

install_mimirtool() {
  bin_dir=${1:-${BIN_DIR}}
  if ! command -v mimirtool &>/dev/null; then
    echo "This script will install mimirtool on your machine"
    if [[ "$(uname)" == "Linux" ]]; then
      curl -o mimirtool -L "https://github.com/grafana/mimir/releases/download/mimir-${MIMIRTOOL_VERSION}/mimirtool-linux-amd64"
    elif [[ "$(uname)" == "Darwin" ]]; then
      curl -o mimirtool -L "https://github.com/grafana/mimir/releases/download/mimir-${MIMIRTOOL_VERSION}/mimirtool-darwin-$(uname -m)"
    fi
    chmod +x ./mimirtool && mv ./mimirtool ${bin_dir}/mimirtool
  fi
}

install_promtool() {
  bin_dir=${1:-${BIN_DIR}}
  if ! command -v promtool &>/dev/null; then
    echo "This script will install promtool on your machine"
    if [[ "$(uname)" == "Linux" ]]; then
      curl -o prometheus.tar.gz -L "https://github.com/prometheus/prometheus/releases/download/v${PROMTOOL_VERSION}/prometheus-${PROMTOOL_VERSION}.linux-amd64.tar.gz"
    elif [[ "$(uname)" == "Darwin" ]]; then
      curl -o prometheus.tar.gz -L "https://github.com/prometheus/prometheus/releases/download/v${PROMTOOL_VERSION}/prometheus-${PROMTOOL_VERSION}.darwin-$(uname -m).tar.gz"
    fi
    mkdir prometheus
    tar -xzf prometheus.tar.gz -C prometheus --strip-components=1
    chmod +x ./prometheus/promtool && mv ./prometheus/promtool ${bin_dir}/promtool
    rm -rf prometheus
    rm prometheus.tar.gz
  fi
}

install_envtest_deps() {
  go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
  bin_dir=${1:-${BIN_DIR}}
  setup-envtest --bin-dir ${bin_dir} -p env use 1.30.x
}

install_binaries() {
  bin_dir=${1:-${BIN_DIR}}
  install_kubectl ${bin_dir}
  install_jq ${bin_dir}
}

# check if script is called directly, or sourced
(return 0 2>/dev/null) && sourced=1 || sourced=0
# This allows functions within this file to be called individually from Makefile(s).
if [[ $sourced == 0 ]]; then
  $*
fi
