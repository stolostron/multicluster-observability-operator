#!/usr/bin/env bash

# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

set -exo pipefail

ROOTDIR="$(
  cd "$(dirname "$0")/.."
  pwd -P
)"

SED_COMMAND=${SED}' -i-e -e'

# Set the latest snapshot if it is not set
source ./scripts/test-utils.sh
LATEST_SNAPSHOT=${LATEST_SNAPSHOT:-$(get_latest_snapshot)}

# list all components need to do test.
CHANGED_COMPONENTS=""
GINKGO_FOCUS=""
IMAGE=""

update_mco_cr() {
  if [ "${OPENSHIFT_CI}" == "true" ]; then
    # discard unstaged changes
    cd ${ROOTDIR} && git checkout -- .
    for component_name in ${CHANGED_COMPONENTS}; do
      component_anno_name=$(echo ${component_name} | sed 's/-/_/g')
      get_image ${component_name}
      ${SED_COMMAND} "/annotations.*/a \ \ \ \ mco-${component_anno_name}-image: ${IMAGE}" ${ROOTDIR}/examples/mco/e2e/v1beta1/observability.yaml
      ${SED_COMMAND} "/annotations.*/a \ \ \ \ mco-${component_anno_name}-image: ${IMAGE}" ${ROOTDIR}/examples/mco/e2e/v1beta2/observability.yaml
    done
  else
    if [[ -n ${RBAC_QUERY_PROXY_IMAGE_REF} ]]; then
      ${SED_COMMAND} "/annotations.*/a \ \ \ \ mco-rbac_query_proxy-image: ${RBAC_QUERY_PROXY_IMAGE_REF}" ${ROOTDIR}/examples/mco/e2e/v1beta1/observability.yaml
      ${SED_COMMAND} "/annotations.*/a \ \ \ \ mco-rbac_query_proxy-image: ${RBAC_QUERY_PROXY_IMAGE_REF}" ${ROOTDIR}/examples/mco/e2e/v1beta2/observability.yaml
    fi
    if [[ -n ${ENDPOINT_MONITORING_OPERATOR_IMAGE_REF} ]]; then
      ${SED_COMMAND} "/annotations.*/a \ \ \ \ mco-endpoint_monitoring_operator-image: ${ENDPOINT_MONITORING_OPERATOR_IMAGE_REF}" ${ROOTDIR}/examples/mco/e2e/v1beta1/observability.yaml
      ${SED_COMMAND} "/annotations.*/a \ \ \ \ mco-endpoint_monitoring_operator-image: ${ENDPOINT_MONITORING_OPERATOR_IMAGE_REF}" ${ROOTDIR}/examples/mco/e2e/v1beta2/observability.yaml
    fi
    if [[ -n ${GRAFANA_DASHBOARD_LOADER_IMAGE_REF} ]]; then
      ${SED_COMMAND} "/annotations.*/a \ \ \ \ mco-grafana_dashboard_loader-image: ${GRAFANA_DASHBOARD_LOADER_IMAGE_REF}" ${ROOTDIR}/examples/mco/e2e/v1beta1/observability.yaml
      ${SED_COMMAND} "/annotations.*/a \ \ \ \ mco-grafana_dashboard_loader-image: ${GRAFANA_DASHBOARD_LOADER_IMAGE_REF}" ${ROOTDIR}/examples/mco/e2e/v1beta2/observability.yaml
    fi
    if [[ -n ${METRICS_COLLECTOR_IMAGE_REF} ]]; then
      ${SED_COMMAND} "/annotations.*/a \ \ \ \ mco-metrics_collector-image: ${METRICS_COLLECTOR_IMAGE_REF}" ${ROOTDIR}/examples/mco/e2e/v1beta1/observability.yaml
      ${SED_COMMAND} "/annotations.*/a \ \ \ \ mco-metrics_collector-image: ${METRICS_COLLECTOR_IMAGE_REF}" ${ROOTDIR}/examples/mco/e2e/v1beta2/observability.yaml
    fi
    if [[ -n ${OBSERVATORIUM_OPERATOR_IMAGE_REF} ]]; then
      ${SED_COMMAND} "/annotations.*/a \ \ \ \ mco-observatorium_operator-image: ${OBSERVATORIUM_OPERATOR_IMAGE_REF}" ${ROOTDIR}/examples/mco/e2e/v1beta1/observability.yaml
      ${SED_COMMAND} "/annotations.*/a \ \ \ \ mco-metrics_collector-image: ${OBSERVATORIUM_OPERATOR_IMAGE_REF}" ${ROOTDIR}/examples/mco/e2e/v1beta2/observability.yaml
    fi
  fi

  # Add mco-imageTagSuffix annotation
  ${SED_COMMAND} "/annotations.*/a \ \ \ \ mco-imageTagSuffix: ${LATEST_SNAPSHOT}" ${ROOTDIR}/examples/mco/e2e/v1beta1/observability.yaml
  ${SED_COMMAND} "/annotations.*/a \ \ \ \ mco-imageTagSuffix: ${LATEST_SNAPSHOT}" ${ROOTDIR}/examples/mco/e2e/v1beta2/observability.yaml

  # need to add this annotation due to KinD cluster resources are insufficient
  if [[ -n ${IS_KIND_ENV} ]]; then
    ${SED_COMMAND} "/annotations.*/a \ \ \ \ mco-thanos-without-resources-requests: true" ${ROOTDIR}/examples/mco/e2e/v1beta1/observability.yaml
    ${SED_COMMAND} "/annotations.*/a \ \ \ \ mco-thanos-without-resources-requests: true" ${ROOTDIR}/examples/mco/e2e/v1beta2/observability.yaml
    # annotate MCO in kind env to be able to install prometheus
    ${SED_COMMAND} "/annotations.*/a \ \ \ \ test-env: kind-test" ${ROOTDIR}/examples/mco/e2e/v1beta2/observability.yaml
  fi
}

get_image() {
  if [[ $1 == "rbac-query-proxy" ]]; then
    IMAGE="${RBAC_QUERY_PROXY_IMAGE_REF}"
  fi
  if [[ $1 == "endpoint-monitoring-operator" ]]; then
    IMAGE="${ENDPOINT_MONITORING_OPERATOR_IMAGE_REF}"
  fi
  if [[ $1 == "grafana-dashboard-loader" ]]; then
    IMAGE="${GRAFANA_DASHBOARD_LOADER_IMAGE_REF}"
  fi
  if [[ $1 == "metrics-collector" ]]; then
    IMAGE="${METRICS_COLLECTOR_IMAGE_REF}"
  fi
}

# function get_changed_components is used to get the component used to test
# get_changed_components is to get the component name based on the changes in your PR
get_changed_components() {
  if [ "${OPENSHIFT_CI}" == "true" ]; then
    changed_files=$(
      cd ${ROOTDIR}
      git diff --name-only HEAD~1
    )
    for file in ${changed_files}; do
      if [[ ${file} =~ ^proxy ]]; then
        CHANGED_COMPONENTS+=" rbac-query-proxy"
        continue
      fi
      if [[ ${file} =~ ^operators/endpointmetrics || ${file} =~ ^operators/pkg ]]; then
        CHANGED_COMPONENTS+=" endpoint-monitoring-operator"
        continue
      fi
      if [[ ${file} =~ ^loaders/dashboards ]]; then
        CHANGED_COMPONENTS+=" grafana-dashboard-loader"
        continue
      fi
      if [[ ${file} =~ ^collectors/metrics ]]; then
        CHANGED_COMPONENTS+=" metrics-collector"
        continue
      fi
      if [[ ${file} =~ ^pkg ]]; then
        CHANGED_COMPONENTS="rbac-query-proxy metrics-collector endpoint-monitoring-operator grafana-dashboard-loader"
        break
      fi
    done
  fi
  # remove duplicates
  CHANGED_COMPONENTS=$(echo "${CHANGED_COMPONENTS}" | xargs -n1 | sort -u | xargs)
  echo "Tested components are ${CHANGED_COMPONENTS}"
}

# function get_ginkgo_focus is to get the required cases
get_ginkgo_focus() {
  if [ "${OPENSHIFT_CI}" == "true" ]; then
    changed_files=$(
      cd $ROOTDIR
      git diff --name-only HEAD~1
    )
    for file in ${changed_files}; do
      if [[ ${file} =~ ^proxy ]]; then
        GINKGO_FOCUS+=" --focus grafana/g0 --focus metrics/g0"
        continue
      fi
      if [[ ${file} =~ ^collectors/metrics ]]; then
        GINKGO_FOCUS+=" --focus grafana/g0 --focus metrics/g0 --focus addon/g0"
        continue
      fi
      if [[ ${file} =~ ^operators/endpointmetrics ]]; then
        GINKGO_FOCUS+=" --focus grafana/g0 --focus metrics/g0 --focus addon/g0 --focus endpoint_preserve/g0"
        continue
      fi
      if [[ ${file} =~ ^loaders/dashboards ]]; then
        GINKGO_FOCUS+=" --focus grafana/g0 --focus metrics/g0 --focus addon/g0"
        continue
      fi
      if [[ $file =~ ^operators/multiclusterobservability ]]; then
        GINKGO_FOCUS+=" --focus addon/g0 --focus config/g0 --focus alert/g0 --focus alertforward/g0 --focus certrenew/g0 --focus grafana/g0 --focus grafana_dev/g0 --focus dashboard/g0 --focus manifestwork/g0 --focus metrics/g0 --focus observatorium_preserve/g0 --focus reconcile/g0 --focus retention/g0 --focus export/g0"
        continue
      fi
      if [[ $file =~ ^operators/pkg ]]; then
        GINKGO_FOCUS+=" --focus addon/g0 --focus config/g0 --focus alert/g0 --focus alertforward/g0  --focus certrenew/g0 --focus grafana/g0 --focus grafana_dev/g0 --focus dashboard/g0 --focus manifestwork/g0 --focus metrics/g0 --focus observatorium_preserve/g0 --focus reconcile/g0 --focus retention/g0 --focus endpoint_preserve/g0 --focus export/g0"
        continue
      fi
      if [[ ${file} =~ ^pkg ]]; then
        # test all cases
        GINKGO_FOCUS=""
        break
      fi
      if [[ $file =~ ^examples/alerts ]]; then
        GINKGO_FOCUS+=" --focus alert/g0 --focus alertforward/g0"
        continue
      fi
      if [[ ${file} =~ ^examples/dashboards ]]; then
        GINKGO_FOCUS+=" --focus dashboard/g0"
        continue
      fi
      if [[ ${file} =~ ^examples/metrics ]]; then
        GINKGO_FOCUS+=" --focus metrics/g0"
        continue
      fi
      if [[ ${file} =~ ^tests ]]; then
        GINKGO_FOCUS+=" --focus $(echo ${file} | cut -d '/' -f4 | sed -En 's/observability_(.*)_test.go/\1/p')/g0"
        continue
      fi
      if [[ ${file} =~ ^tools ]]; then
        GINKGO_FOCUS+=" --focus grafana_dev/g0"
        continue
      fi
    done
  fi

  if [[ -n ${IS_KIND_ENV} ]]; then
    # For KinD cluster, do not need to run all test cases
    GINKGO_FOCUS=" --focus manifestwork/g0 --focus endpoint_preserve/g0 --focus grafana/g0 --focus metrics/g0 --focus addon/g0 --focus alert/g0 --focus dashboard/g0"
  else
    GINKGO_FOCUS=$(echo "${GINKGO_FOCUS}" | xargs -n2 | sort -u | xargs)
  fi
  echo "Test focuses are ${GINKGO_FOCUS}"
}

# start executing
get_changed_components
update_mco_cr
get_ginkgo_focus
echo "${GINKGO_FOCUS}" >/tmp/ginkgo_focus
