#!/bin/bash
# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

WORK_DIR="$(
  cd "$(dirname "$0")"
  pwd -P
)"

KUBECTL="kubectl"
if ! command -v kubectl &>/dev/null; then
  if command -v oc &>/dev/null; then
    KUBECTL="oc"
  else
    echo "kubectl or oc must be installed!"
    exit 1
  fi
fi

SED_COMMAND='sed -i'
if [[ "$(uname)" == "Darwin" ]]; then
  SED_COMMAND='sed -i -e'
fi

function usage() {
  echo "${0} [-i INTERVAL] [-w WORKERS]"
  echo ''
  # shellcheck disable=SC2016
  echo '  -i: Specifies the alert forward INTERVAL, optional, the default value is "30s".'
  # shellcheck disable=SC2016
  echo '  -w: Specifies the number of concurrent workers that forward the alerts, optional, the default value is "1000".'
  echo ''
}

INTERVAL="30s" # default alert forward interval
WORKERS=1000   # default alert forward workers

# Allow command-line args to override the defaults.
while getopts ":i:w:h" opt; do
  case ${opt} in
    i)
      INTERVAL=${OPTARG}
      ;;
    w)
      WORKERS=${OPTARG}
      ;;
    h)
      usage
      exit 0
      ;;
    \?)
      echo "Invalid option: -$OPTARG" >&2
      usage
      exit 1
      ;;
  esac
done

OBSERVABILITY_NS="open-cluster-management-observability"
AM_ACCESS_SA="observability-alertmanager-accessor"
AM_ROUTE="alertmanager"
AM_ACCESS_TOKEN=$(${KUBECTL} -n ${OBSERVABILITY_NS} get secret $(${KUBECTL} -n ${OBSERVABILITY_NS} get sa ${AM_ACCESS_SA} -o yaml | grep ${AM_ACCESS_SA}-token | cut -d' ' -f3) -o jsonpath="{.data.token}" | base64 -d)
AM_ACCESS_TOKEN_SECRET="am-access-token"
AM_HOST=$(${KUBECTL} -n ${OBSERVABILITY_NS} get route ${AM_ROUTE} -o jsonpath="{.spec.host}")
ALERT_FORWARDER_NS="alert-forwarder"

${SED_COMMAND} "s~__AM_HOST__~${AM_HOST}~g" ${WORK_DIR}/deployment.yaml
${SED_COMMAND} "s~--interval=30s~--interval=${INTERVAL}~g" ${WORK_DIR}/deployment.yaml
${SED_COMMAND} "s~--workers=1000~--workers=${WORKERS}~g" ${WORK_DIR}/deployment.yaml
${KUBECTL} create ns ${ALERT_FORWARDER_NS}
${KUBECTL} -n ${ALERT_FORWARDER_NS} create secret generic ${AM_ACCESS_TOKEN_SECRET} --from-literal=token=${AM_ACCESS_TOKEN}
${KUBECTL} -n ${ALERT_FORWARDER_NS} apply -f ${WORK_DIR}/deployment.yaml
