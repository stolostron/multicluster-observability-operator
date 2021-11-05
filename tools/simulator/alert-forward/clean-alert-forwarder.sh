#!/bin/bash
# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

KUBECTL="kubectl"
if ! command -v kubectl &> /dev/null; then
    if command -v oc &> /dev/null; then
        SED_COMMAND="oc"
    else
        echo "kubectl or oc must be installed!"
        exit 1
    fi
fi

ALERT_FORWARDER_NS="alert-forwarder"
ALERT_FORWARDER_DEPLOY="alert-forwarder"
AM_ACCESS_TOKEN_SECRET="am-access-token"

${KUBECTL} -n ${ALERT_FORWARDER_NS} delete deployment ${ALERT_FORWARDER_DEPLOY}
${KUBECTL} -n ${ALERT_FORWARDER_NS} delete secret ${AM_ACCESS_TOKEN_SECRET}
${KUBECTL} delete ns ${ALERT_FORWARDER_NS}
