#!/bin/bash
# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

KUBECTL="kubectl"
if ! command -v kubectl &>/dev/null; then
  if command -v oc &>/dev/null; then
    KUBECTL="oc"
  else
    echo "kubectl or oc must be installed!"
    exit 1
  fi
fi

function usage() {
  echo "${0} -n NUMBERS [-m MANAGED_CLUSTER_PREFIX]"
  echo ''
  # shellcheck disable=SC2016
  echo '  -n: Specifies the total number of simulated metrics collectors, required'
  # shellcheck disable=SC2016
  echo '  -m: Specifies the prefix for the simulated managedcluster name, optional, the default value is "simulated-managed-cluster".'
  echo ''
}

MANAGED_CLUSTER_PREFIX="simulated-managed-cluster" # default managedccluster name prefix

# Allow command-line args to override the defaults.
while getopts ":n:m:h" opt; do
  case ${opt} in
    n)
      NUMBERS=${OPTARG}
      ;;
    m)
      MANAGED_CLUSTER_PREFIX=${OPTARG}
      ;;
    h)
      usage
      exit 0
      ;;
    \?)
      echo "Invalid option: -${OPTARG}" >&2
      usage
      exit 1
      ;;
  esac
done

if [[ -z ${NUMBERS} ]]; then
  echo "Error: NUMBERS (-n) must be specified!"
  usage
  exit 1
fi

re='^[0-9]+$'
if ! [[ ${NUMBERS} =~ ${re} ]]; then
  echo "error: arguments <${NUMBERS}> is not a number" >&2
  exit 1

fi

for i in $(seq 1 ${NUMBERS}); do
  cluster_name=${MANAGED_CLUSTER_PREFIX}-${i}
  ${KUBECTL} delete deploy -n ${cluster_name} metrics-collector-deployment
  ${KUBECTL} delete clusterrolebinding ${cluster_name}-clusters-metrics-collector-view
  ${KUBECTL} delete clusterrolebinding ${cluster_name}-endpoint-operator-role-crd-hostedclusters-read
  ${KUBECTL} delete -n ${cluster_name} secret/observability-managed-cluster-certs
  ${KUBECTL} delete ns ${cluster_name}
done

${KUBECTL} delete clusterrole endpoint-observability-operator-crd-hostedclusters-read
