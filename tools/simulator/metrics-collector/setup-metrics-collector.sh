#!/bin/bash
# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

WORK_DIR="$(
  cd "$(dirname "$0")"
  pwd -P
)"
# Create bin directory and add it to PATH
mkdir -p ${WORK_DIR}/bin
export PATH=${PATH}:${WORK_DIR}/bin

if ! command -v jq &>/dev/null; then
  if [[ "$(uname)" == "Linux" ]]; then
    curl -o jq -L https://github.com/stedolan/jq/releases/download/jq-1.6/jq-linux64
  elif [[ "$(uname)" == "Darwin" ]]; then
    curl -o jq -L https://github.com/stedolan/jq/releases/download/jq-1.6/jq-osx-amd64
  fi
  chmod +x ./jq
  chmod +x ./jq && mv ./jq ${WORK_DIR}/bin/jq
fi

KUBECTL="kubectl"
if ! command -v kubectl &>/dev/null; then
  if command -v oc &>/dev/null; then
    KUBECTL="oc"
  else
    echo "This script will install kubectl (https://kubernetes.io/docs/tasks/tools/install-kubectl/) on your machine"
    if [[ "$(uname)" == "Linux" ]]; then
      curl -LO https://storage.googleapis.com/kubernetes-release/release/v1.21.0/bin/linux/amd64/kubectl
    elif [[ "$(uname)" == "Darwin" ]]; then
      curl -LO https://storage.googleapis.com/kubernetes-release/release/v1.21.0/bin/darwin/amd64/kubectl
    fi
    chmod +x ./kubectl && mv ./kubectl ${WORK_DIR}/bin/kubectl
  fi
fi

SED_COMMAND='sed -i'
if [[ "$(uname)" == "Darwin" ]]; then
  SED_COMMAND='sed -i -e'
fi

function usage() {
  echo "${0} -n NUMBERS [-t METRICS_DATA_TYPE] [-w WORKERS] [-m MANAGED_CLUSTER_PREFIX]"
  echo ''
  # shellcheck disable=SC2016
  echo '  -n: Specifies the total number of simulated metrics collectors, required'
  # shellcheck disable=SC2016
  echo '  -t: Specifies the data type of metrics data source, the default value is "NON_SNO", it also can be "SNO".'
  # shellcheck disable=SC2016
  echo '  -w: Specifies the worker threads for each simulated metrics collector, optional, the default value is "1".'
  # shellcheck disable=SC2016
  echo '  -m: Specifies the prefix for the simulated managedcluster name, optional, the default value is "simulated-managed-cluster".'
  echo ''
}

WORKERS=1                                          # default worker threads for each simulated metrics collector
METRICS_DATA_TYPE="NON_SNO"                        # default metrics data source type
MANAGED_CLUSTER_PREFIX="simulated-managed-cluster" # default managedccluster name prefix

# Allow command-line args to override the defaults.
while getopts ":n:t:w:m:h" opt; do
  case ${opt} in
    n)
      NUMBERS=${OPTARG}
      ;;
    t)
      METRICS_DATA_TYPE=${OPTARG}
      ;;
    w)
      WORKERS=${OPTARG}
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

if [[ ${METRICS_DATA_TYPE} != "SNO" && ${METRICS_DATA_TYPE} != "NON_SNO" ]]; then
  echo "error: arguments <${METRICS_DATA_TYPE}> is not valid, it must be 'SNO' of 'NON_SNO'" >&2
  exit 1
fi

if ! [[ ${WORKERS} =~ ${re} ]]; then
  echo "error: arguments <${WORKERS}> is not a number" >&2
  exit 1
fi

OBSERVABILITY_NS="open-cluster-management-observability"

# metrics data source image
DEFAULT_METRICS_IMAGE="quay.io/ocm-observability/metrics-data:2.4.0"
if [[ ${METRICS_DATA_TYPE} == "SNO" ]]; then
  DEFAULT_METRICS_IMAGE="quay.io/ocm-observability/metrics-data:2.4.0-sno"
fi
METRICS_IMAGE="${METRICS_IMAGE:-$DEFAULT_METRICS_IMAGE}"

for i in $(seq 1 ${NUMBERS}); do
  cluster_name=${MANAGED_CLUSTER_PREFIX}-${i}
  ${KUBECTL} create ns ${cluster_name}

  # create ca/sa/rolebinding for metrics collector
  ${KUBECTL} get configmap metrics-collector-serving-certs-ca-bundle -n ${OBSERVABILITY_NS} -o json | jq 'del(.metadata.namespace,.metadata.resourceVersion,.metadata.uid) | .metadata.creationTimestamp=null' | ${KUBECTL} apply -n ${cluster_name} -f -
  ${KUBECTL} get secret observability-controller-open-cluster-management.io-observability-signer-client-cert -n ${OBSERVABILITY_NS} -o json | jq 'del(.metadata.namespace,.metadata.resourceVersion,.metadata.uid) | .metadata.creationTimestamp=null' | ${KUBECTL} apply -n ${cluster_name} -f -
  ${KUBECTL} get secret observability-managed-cluster-certs -n ${OBSERVABILITY_NS} -o json | jq 'del(.metadata.namespace,.metadata.resourceVersion,.metadata.uid) | .metadata.creationTimestamp=null' | ${KUBECTL} apply -n ${cluster_name} -f -
  ${KUBECTL} get sa endpoint-observability-operator-sa -n ${OBSERVABILITY_NS} -o json | jq 'del(.metadata.namespace,.metadata.resourceVersion,.metadata.uid) | .metadata.creationTimestamp=null' | ${KUBECTL} apply -n ${cluster_name} -f -
  ${KUBECTL} -n ${cluster_name} patch secret observability-managed-cluster-certs --type='json' -p='[{"op": "replace", "path": "/metadata/ownerReferences", "value": []}]'
  ${KUBECTL} -n ${cluster_name} patch sa endpoint-observability-operator-sa --type='json' -p='[{"op": "replace", "path": "/metadata/ownerReferences", "value": []}]'

  # deploy metrics collector deployment to cluster ns
  deploy_yaml_file=${cluster_name}-metrics-collector-deployment.json
  ${KUBECTL} get deploy metrics-collector-deployment -n ${OBSERVABILITY_NS} -o json >${deploy_yaml_file}

  # replace namespace, cluster and clusterID. Insert --simulated-timeseries-file
  uuid=$(cat /proc/sys/kernel/random/uuid)
  jq \
    --arg cluster_name ${cluster_name} \
    --arg cluster "--label=\"cluster=${cluster_name}\"" \
    --arg clusterID "--label=\"clusterID=${uuid}\"" \
    --arg workerNum "--worker-number=${WORKERS}" \
    --arg file "--simulated-timeseries-file=/metrics-volume/timeseries.txt" \
    '.metadata.namespace=$cluster_name | .spec.template.spec.containers[0].command[.spec.template.spec.containers[0].command|length] |= . + $cluster |.spec.template.spec.containers[0].command[.spec.template.spec.containers[0].command|length] |= . + $clusterID | .spec.template.spec.containers[0].command[.spec.template.spec.containers[0].command|length] |= . + $file | .spec.template.spec.containers[0].command[.spec.template.spec.containers[0].command|length] |= . + $workerNum' ${deploy_yaml_file} >${deploy_yaml_file}.tmp && mv ${deploy_yaml_file}.tmp ${deploy_yaml_file}

  # insert metrics initContainer
  jq \
    --argjson init '{"initContainers": [{"command":["sh","-c","cp /tmp/timeseries.txt /metrics-volume"],"image":"'${METRICS_IMAGE}'","imagePullPolicy":"IfNotPresent","name":"init-metrics","volumeMounts":[{"mountPath":"/metrics-volume","name":"metrics-volume"}]}]}' \
    --argjson emptydir '{"emptyDir": {}, "name": "metrics-volume"}' \
    --argjson metricsdir '{"mountPath": "/metrics-volume","name": "metrics-volume"}' \
    '.spec.template.spec += $init | .spec.template.spec.volumes += [$emptydir] | .spec.template.spec.containers[0].volumeMounts += [$metricsdir]' ${deploy_yaml_file} >${deploy_yaml_file}.tmp && mv ${deploy_yaml_file}.tmp ${deploy_yaml_file}

  cat "${deploy_yaml_file}" | ${KUBECTL} -n ${cluster_name} apply -f -
  rm -f "${deploy_yaml_file}" "${deploy_yaml_file}".tmp
  ${KUBECTL} -n ${cluster_name} patch deploy metrics-collector-deployment --type='json' -p='[{"op": "replace", "path": "/metadata/ownerReferences", "value": []}]'
  ${KUBECTL} -n ${cluster_name} patch deploy metrics-collector-deployment --type='json' -p='[{"op": "remove", "path": "/spec/template/spec/containers/0/resources"}]'

  # deploy role
  cat "role-endpoint-observability-operator-crd-hostedclusters-read.yaml" | ${KUBECTL} -n ${cluster_name} apply -f -

  # deploy ClusterRoleBinding for read metrics from OCP prometheus
  rolebinding_yaml_file=${cluster_name}-metrics-collector-view.yaml
  cp -rf metrics-collector-view.yaml "$rolebinding_yaml_file"
  ${SED_COMMAND} "s~__CLUSTER_NAME__~${cluster_name}~g" "${rolebinding_yaml_file}"
  cat "${rolebinding_yaml_file}" | ${KUBECTL} -n ${cluster_name} apply -f -
  rm -f "${rolebinding_yaml_file}"

  # deploy ClusterRoleBinding for reading CRDs and HosterClusters
  rolebinding_yaml_file=${cluster_name}-rb-endpoint-operator-role-crd-hostedclusters-read.yaml
  cp -rf rb-endpoint-operator-role-crd-hostedclusters-read.yaml "$rolebinding_yaml_file"
  ${SED_COMMAND} "s~__CLUSTER_NAME__~${cluster_name}~g" "${rolebinding_yaml_file}"
  cat "${rolebinding_yaml_file}" | ${KUBECTL} -n ${cluster_name} apply -f -
  rm -f "${rolebinding_yaml_file}"
done
