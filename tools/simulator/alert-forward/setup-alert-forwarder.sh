#!/bin/bash
# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

WORK_DIR="$(cd "$(dirname "$0")" ; pwd -P)"

KUBECTL="kubectl"
if ! command -v kubectl &> /dev/null; then
    if command -v oc &> /dev/null; then
        SED_COMMAND="oc"
    else
        echo "kubectl or oc must be installed!"
        exit 1
    fi
fi

SED_COMMAND='sed -i'
if [[ "$(uname)" == "Darwin" ]]; then
    sed_command='sed -i -e'
fi

OBSERVABILITY_NS="open-cluster-management-observability"
AM_ACCESS_SA="observability-alertmanager-accessor"
AM_ROUTE="alertmanager"
AM_ACCESS_TOKEN=$(${KUBECTL} -n ${OBSERVABILITY_NS} get secret $(${KUBECTL} -n ${OBSERVABILITY_NS} get sa ${AM_ACCESS_SA} -o yaml | grep ${AM_ACCESS_SA}-token | cut -d' ' -f3) -o jsonpath="{.data.token}" | base64 -d)
AM_ACCESS_TOKEN_SECRET="am-access-token"
AM_HOST=$(${KUBECTL} -n ${OBSERVABILITY_NS} get route ${AM_ROUTE} -o jsonpath="{.spec.host}")
ALERT_FORWARDER_NS="alert-forwarder"

${SED_COMMAND} "s~__AM_HOST__~${AM_HOST}~g" ${WORK_DIR}/deployment.yaml
${KUBECTL} create ns ${ALERT_FORWARDER_NS}
${KUBECTL} -n ${ALERT_FORWARDER_NS} create secret generic ${AM_ACCESS_TOKEN_SECRET} --from-literal=token=${AM_ACCESS_TOKEN}
${KUBECTL} -n ${ALERT_FORWARDER_NS} apply -f ${WORK_DIR}/deployment.yaml

