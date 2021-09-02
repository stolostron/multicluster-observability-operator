#!/usr/bin/env bash

# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

set -exo pipefail

ROOTDIR="$(cd "$(dirname "$0")/.." ; pwd -P)"
export PATH=${PATH}:${ROOTDIR}/bin

if [[ "$(uname)" == "Linux" ]]; then
    SED_COMMAND='sed -i-e -e'
elif [[ "$(uname)" == "Darwin" ]]; then
    SED_COMMAND='sed -i '-e' -e'
fi

# Use snapshot for target release. Use latest one if no branch info detected, or not a release branch
BRANCH=""
LATEST_SNAPSHOT=""
if [[ "${PULL_BASE_REF}" == "release-"* ]]; then
    BRANCH=${PULL_BASE_REF#"release-"}
    BRANCH=${BRANCH}".0"
    LATEST_SNAPSHOT=$(curl https://quay.io/api/v1/repository/open-cluster-management/multicluster-observability-operator | jq '.tags|with_entries(select(.key|contains("'${BRANCH}'-SNAPSHOT")))|keys[length-1]')
fi
if [[ "${LATEST_SNAPSHOT}" == "null" ]] || [[ "${LATEST_SNAPSHOT}" == "" ]]; then
    LATEST_SNAPSHOT=$(curl https://quay.io/api/v1/repository/open-cluster-management/multicluster-observability-operator | jq '.tags|with_entries(select(.key|contains("SNAPSHOT")))|keys[length-1]')
fi

# trim the leading and tailing quotes
LATEST_SNAPSHOT="${LATEST_SNAPSHOT#\"}"
LATEST_SNAPSHOT="${LATEST_SNAPSHOT%\"}"

# list all components need to do test.
COMPONENTS=""
GINKGO_FOCUS=""
IMAGE=""

update_mco_cr() {
    # discard unstaged changes
    cd ${ROOTDIR} && git checkout -- .

    # Add mco-imageTagSuffix annotation
    ${SED_COMMAND} "/annotations.*/a \ \ \ \ mco-imageTagSuffix: ${LATEST_SNAPSHOT}" ${ROOTDIR}/examples/mco/e2e/v1beta1/observability.yaml
    ${SED_COMMAND} "/annotations.*/a \ \ \ \ mco-imageTagSuffix: ${LATEST_SNAPSHOT}" ${ROOTDIR}/examples/mco/e2e/v1beta2/observability.yaml

    # need to add this annotation due to KinD cluster resources are insufficient
    if [[ -n "${IS_KIND_ENV}" ]]; then
        ${SED_COMMAND} "/annotations.*/a \ \ \ \ mco-thanos-without-resources-requests: true" ${ROOTDIR}/examples/mco/e2e/v1beta1/observability.yaml
        ${SED_COMMAND} "/annotations.*/a \ \ \ \ mco-thanos-without-resources-requests: true" ${ROOTDIR}/examples/mco/e2e/v1beta2/observability.yaml
    fi

    for component_name in ${@}; do
        component_anno_name=$(echo ${component_name} | sed 's/-/_/g')
        get_image ${component_name}
        ${SED_COMMAND} "/annotations.*/a \ \ \ \ mco-${component_anno_name}-image: ${IMAGE}" ${ROOTDIR}/examples/mco/e2e/v1beta1/observability.yaml
        ${SED_COMMAND} "/annotations.*/a \ \ \ \ mco-${component_anno_name}-image: ${IMAGE}" ${ROOTDIR}/examples/mco/e2e/v1beta2/observability.yaml
    done
}

get_image() {
    if [[ $1 = "rbac-query-proxy" ]]; then
        IMAGE="${RBAC_QUERY_PROXY_IMAGE_REF}"
    fi
    if [[ $1 = "endpoint-monitoring-operator" ]]; then
        IMAGE="${ENDPOINT_MONITORING_OPERATOR_IMAGE_REF}"
    fi
    if [[ $1 = "grafana-dashboard-loader" ]]; then
        IMAGE="${GRAFANA_DASHBOARD_LOADER_IMAGE_REF}"
    fi
    if [[ $1 = "metrics-collector" ]]; then
        IMAGE="${METRICS_COLLECTOR_IMAGE_REF}"
    fi
}

# function get_components is to get the component used to test
# get_components is to get the component name based on the changes in your PR
get_components() {
    changed_files=`cd $ROOTDIR; git diff --name-only HEAD~1`
    for file in ${changed_files}; do
        if [[ $file =~ ^proxy ]]; then
            COMPONENTS+=" rbac-query-proxy"
            continue
        fi
        if [[ $file =~ ^operators/endpointmetrics || $file =~ ^operators/pkg ]]; then
            COMPONENTS+=" endpoint-monitoring-operator"
            continue
        fi
        if [[ $file =~ ^loaders/dashboards ]]; then
            COMPONENTS+=" grafana-dashboard-loader"
            continue
        fi
        if [[ $file =~ ^collectors/metrics ]]; then
            COMPONENTS+=" metrics-collector"
            continue
        fi
        if [[ $file =~ ^pkg ]]; then
            COMPONENTS="rbac-query-proxy metrics-collector endpoint-monitoring-operator grafana-dashboard-loader"
            break
        fi
    done
    # remove duplicates
    COMPONENTS=`echo "${COMPONENTS}" | xargs -n1 | sort -u | xargs`
    echo "Tested components are ${COMPONENTS}"
}

# function get_ginkgo_focus is to get the required cases
get_ginkgo_focus() {
    changed_files=`cd $ROOTDIR; git diff --name-only HEAD~1`
    for file in ${changed_files}; do
        if [[ $file =~ ^proxy ]]; then
            GINKGO_FOCUS+=" --focus grafana/g0 --focus metrics/g0"
            continue
        fi
        if [[ $file =~ ^collectors/metrics ]]; then
            GINKGO_FOCUS+=" --focus grafana/g0 --focus metrics/g0 --focus addon/g0"
            continue
        fi
        if [[ $file =~ ^operators/endpointmetrics ]]; then
            GINKGO_FOCUS+=" --focus grafana/g0 --focus metrics/g0 --focus addon/g0 --focus endpoint_preserve/g0"
            continue
        fi
        if [[ $file =~ ^loaders/dashboards ]]; then
            GINKGO_FOCUS+=" --focus grafana/g0 --focus metrics/g0 --focus addon/g0"
            continue
        fi
        if [[ $file =~ ^operators/multiclusterobservability ]]; then
            GINKGO_FOCUS+=" --focus addon/g0 --focus config/g0 --focus alert/g0 --focus certrenew/g0 --focus grafana/g0 --focus grafana_dev/g0 --focus dashboard/g0 --focus manifestwork/g0 --focus metrics/g0 --focus observatorium_preserve/g0 --focus reconcile/g0 --focus retention/g0"
            continue
        fi
        if [[ $file =~ ^operators/pkg ]]; then
            GINKGO_FOCUS+=" --focus addon/g0 --focus config/g0 --focus alert/g0 --focus certrenew/g0 --focus grafana/g0 --focus grafana_dev/g0 --focus dashboard/g0 --focus manifestwork/g0 --focus metrics/g0 --focus observatorium_preserve/g0 --focus reconcile/g0 --focus retention/g0 --focus endpoint_preserve/g0"
            continue
        fi
        if [[ $file =~ ^pkg ]]; then
            # test all cases
            GINKGO_FOCUS=""
            break
        fi
        if [[ $file =~ ^examples/alerts ]]; then
            GINKGO_FOCUS+=" --focus alert/g0"
            continue
        fi
        if [[ $file =~ ^examples/dashboards ]]; then
            GINKGO_FOCUS+=" --focus dashboard/g0"
            continue
        fi
        if [[ $file =~ ^examples/metrics ]]; then
            GINKGO_FOCUS+=" --focus metrics/g0"
            continue
        fi
        if [[ $file =~ ^tests ]]; then
            GINKGO_FOCUS+=" --focus $(echo $file | cut -d '/' -f4 | sed -En 's/observability_(.*)_test.go/\1/p')/g0"
            continue
        fi
        if [[ $file =~ ^tools ]]; then
           GINKGO_FOCUS+=" --focus grafana_dev/g0"
           continue
        fi
    done
    # For KinD cluster, do not need to run all test cases
    if [[ -n "${IS_KIND_ENV}" ]]; then
        GINKGO_FOCUS=" --focus manifestwork/g0 --focus endpoint_preserve/g0 --focus grafana/g0 --focus metrics/g0 --focus addon/g0 --focus alert/g0 --focus dashboard/g0"
    else
        GINKGO_FOCUS=`echo "${GINKGO_FOCUS}" | xargs -n2 | sort -u | xargs`
    fi
    echo "Test focuses are ${GINKGO_FOCUS}"
}

# start executing the ACTION
get_components
update_mco_cr "${COMPONENTS}"
get_ginkgo_focus
echo "${GINKGO_FOCUS}" > /tmp/ginkgo_focus
