#!/bin/bash
# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

# Copyright Contributors to the Open Cluster Management project

set -eo pipefail

WORKDIR="$(
  cd "$(dirname "$0")"
  pwd -P
)"

# Create bin directory and add it to PATH
mkdir -p ${WORKDIR}/bin
export PATH=${PATH}:${WORKDIR}/bin

# tmp output directory for metrics list
TMP_OUT=$(mktemp -d /tmp/metrics.XXXXXXXXXX)
METRICS_JSON_OUT=${TMP_OUT}/metrics.json
RECORDINGRULES_JSON_OUT=${TMP_OUT}/recordingrules.json
TIME_SERIES_OUT=${WORKDIR}/timeseries.txt

METRICS_ALLOW_LIST_URL=${METRICS_ALLOW_LIST_URL:-https://raw.githubusercontent.com/stolostron/multicluster-observability-operator/main/operators/multiclusterobservability/manifests/base/config/metrics_allowlist.yaml}
METRICS_IMAGE=${METRICS_IMAGE-quay.io/ocm-observability/metrics-data:2.4.0}

if [[ -z ${IS_TIMESERIES_ONLY} ]]; then
  # check docker
  if ! command -v docker &>/dev/null; then
    echo "docker must be installed to run this script."
    exit 1
  fi
fi

# install kubectl
KUBECTL="kubectl"
if ! command -v kubectl &>/dev/null; then
  if command -v oc &>/dev/null; then
    KUBECTL="oc"
  else
    if [[ "$(uname)" == "Linux" ]]; then
      curl -LO https://storage.googleapis.com/kubernetes-release/release/v1.21.0/bin/linux/amd64/kubectl
    elif [[ "$(uname)" == "Darwin" ]]; then
      if [[ "$(uname -m)" == "x64_64" ]]; then
        curl -LO https://storage.googleapis.com/kubernetes-release/release/v1.21.0/bin/darwin/amd64/kubectl
      else
        curl -LO https://storage.googleapis.com/kubernetes-release/release/v1.21.0/bin/darwin/arm64/kubectl
      fi
    fi
    chmod +x ./kubectl && mv ./kubectl ${WORKDIR}/bin/kubectl
  fi
fi

# install jq
if ! command -v jq &>/dev/null; then
  if [[ "$(uname)" == "Linux" ]]; then
    curl -o jq -L https://github.com/stedolan/jq/releases/download/jq-1.7.1/jq-linux64
  elif [[ "$(uname)" == "Darwin" ]]; then
    if [[ "$(uname -m)" == "x64_64" ]]; then
      curl -o jq -L https://github.com/stedolan/jq/releases/download/jq-1.7.1/jq-macos-amd64
    else
      curl -o jq -L https://github.com/stedolan/jq/releases/download/jq-1.7.1/jq-macos-arm64
    fi
  fi
  chmod +x ./jq && mv ./jq ${WORKDIR}/bin/jq
fi

# install gojsontoyaml
if ! command -v gojsontoyaml &>/dev/null; then
  if [[ "$(uname)" == "Linux" ]]; then
    curl -L https://github.com/brancz/gojsontoyaml/releases/download/v0.1.0/gojsontoyaml_0.1.0_linux_amd64.tar.gz | tar -xz -C ${WORKDIR}/bin/
  elif [[ "$(uname)" == "Darwin" ]]; then
    if [[ "$(uname -m)" == "x64_64" ]]; then
      curl -L https://github.com/brancz/gojsontoyaml/releases/download/v0.1.0/gojsontoyaml_0.1.0_darwin_amd64.tar.gz | tar -xz -C ${WORKDIR}/bin/
    else
      curl -L https://github.com/brancz/gojsontoyaml/releases/download/v0.1.0/gojsontoyaml_0.1.0_darwin_arm64.tar.gz | tar -xz -C ${WORKDIR}/bin/
    fi
  fi
fi

function get_metrics_list() {
  echo "getting metrics list..."
  if [[ -z ${IS_GENERATING_OCP311_METRICS} ]]; then
    matches=$(curl -L ${METRICS_ALLOW_LIST_URL} | gojsontoyaml --yamltojson | jq -r '.data."metrics_list.yaml"' | gojsontoyaml --yamltojson | jq -r '.matches' | jq '"{" + .[] + "}"')
    names=$(curl -L ${METRICS_ALLOW_LIST_URL} | gojsontoyaml --yamltojson | jq -r '.data."metrics_list.yaml"' | gojsontoyaml --yamltojson | jq -r '.names' | jq '"{__name__=\"" + .[] + "\"}"')
    echo $matches $names | jq -s . >${METRICS_JSON_OUT}
  else
    matches=$(curl -L ${METRICS_ALLOW_LIST_URL} | gojsontoyaml --yamltojson | jq -r '.data."ocp311_metrics_list.yaml"' | gojsontoyaml --yamltojson | jq -r '.matches' | jq '"{" + .[] + "}"')
    names=$(curl -L ${METRICS_ALLOW_LIST_URL} | gojsontoyaml --yamltojson | jq -r '.data."ocp311_metrics_list.yaml"' | gojsontoyaml --yamltojson | jq -r '.names' | jq '"{__name__=\"" + .[] + "\"}"')
    echo $matches $names | jq -s . >${METRICS_JSON_OUT}
  fi
}

function get_recordingrules_list() {
  echo "getting recordingrules list..."
  if [[ -z ${IS_GENERATING_OCP311_METRICS} ]]; then
    recordingrules=$(curl -L ${METRICS_ALLOW_LIST_URL} | gojsontoyaml --yamltojson | jq -r '.data."metrics_list.yaml"' | gojsontoyaml --yamltojson | jq '.recording_rules[]')
    echo "$recordingrules" | jq -s . >${RECORDINGRULES_JSON_OUT}
  else
    recordingrules=$(curl -L ${METRICS_ALLOW_LIST_URL} | gojsontoyaml --yamltojson | jq -r '.data."ocp311_metrics_list.yaml"' | gojsontoyaml --yamltojson | jq '.recording_rules[]')
    echo "$recordingrules" | jq -s . >${RECORDINGRULES_JSON_OUT}
  fi
}

function generate_metrics() {
  echo "generating metrics..."
  federate="curl --fail --silent -G http://localhost:9090/federate"
  for rule in $(cat ${METRICS_JSON_OUT} | jq -r '.[]'); do
    federate="${federate} $(printf -- "--data-urlencode match[]=%s" ${rule})"
  done
  echo '# Beginning for metrics' >${TIME_SERIES_OUT}
  ${federate} >>${TIME_SERIES_OUT}
}

function generate_recordingrules() {
  echo "generating recordingrules..."
  query="curl --fail --silent -G http://localhost:9090/api/v1/query"
  cat ${RECORDINGRULES_JSON_OUT} | jq -cr '.[]' | while read item; do
    record=$(jq -r '.record' <<<"$item")
    expr=$(jq -r '.expr' <<<"$item")
    urlencode=$(printf %s "${expr}" | jq -s -R -r @uri)
    querycmd="${query} -d query=${urlencode}"
    echo -e "\n# TYPE ${record} untyped" >>${TIME_SERIES_OUT}
    ${querycmd} | jq -r '.data.result' | jq -cr '.[]' | while read result; do
      vec="${record}"
      metric=$(jq -r '.metric | to_entries | map("\(.key)=\"\(.value | tostring)\"") | .[]' <<<"$result")
      metric=$(echo "${metric}" | sed ':a;N;$!ba;s/\n/,/g')
      vec="${vec}{${metric}}"
      timestamp=$(jq -r '.value[0]' <<<"$result")
      value=$(jq -r '.value[1]' <<<"$result")
      timestamp=$(echo "${timestamp} * 1000" | bc)
      timestamp=${timestamp%.*}
      echo "${vec} ${value} ${timestamp}" >>${TIME_SERIES_OUT}
    done
  done
}

function generate_timeseries() {
  ${KUBECTL} port-forward -n openshift-monitoring prometheus-k8s-0 9090 >/dev/null &
  sleep 10
  generate_metrics
  generate_recordingrules
  jobs -p | xargs -r kill
}

function build_metrics_data_image() {
  docker build -t ${METRICS_IMAGE} .
}

function push_metrics_data_image() {
  docker push ${METRICS_IMAGE}
}

get_metrics_list
get_recordingrules_list
generate_timeseries
if [[ -z ${IS_TIMESERIES_ONLY} ]]; then
  build_metrics_data_image
  push_metrics_data_image
fi
