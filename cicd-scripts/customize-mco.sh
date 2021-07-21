#!/bin/bash
# Copyright (c) 2021 Red Hat, Inc.

set -e

ROOTDIR="$(cd "$(dirname "$0")/.." ; pwd -P)"

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
IMAGE=""

update_mco_cr() {
    # discard unstaged changes
    cd ${ROOTDIR} && git checkout -- .

    # Add mco-imageTagSuffix annotation
    ${SED_COMMAND} "/annotations.*/a \ \ \ \ mco-imageTagSuffix: ${LATEST_SNAPSHOT}" ${ROOTDIR}/examples/mco/e2e/v1beta1/observability.yaml
    ${SED_COMMAND} "/annotations.*/a \ \ \ \ mco-imageTagSuffix: ${LATEST_SNAPSHOT}" ${ROOTDIR}/examples/mco/e2e/v1beta2/observability.yaml

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
    changed_files=`cd $ROOTDIR; git --no-pager diff --name-only main...HEAD > /dev/null 2>&1`
    echo $changed_files
    for file in ${changed_files}; do
        echo $file
        if [[ $file =~ ^proxy ]]; then
            COMPONENTS+=" rbac-query-proxy"
            continue
        fi
        if [[ $file =~ ^operators/endpointmetrics ]]; then
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

# start executing the ACTION
get_components
update_mco_cr "${COMPONENTS}"
