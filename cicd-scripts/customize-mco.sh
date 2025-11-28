#!/usr/bin/env bash

# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

set -exo pipefail

ROOTDIR="$(
  cd "$(dirname "$0")/.."
  pwd -P
)"

SED_COMMAND=(sed -i -e)

# Set the latest snapshot if it is not set
source ./scripts/test-utils.sh
LATEST_SNAPSHOT=${LATEST_SNAPSHOT:-$(get_latest_acm_snapshot)}

if [[ -n ${IS_KIND_ENV} ]]; then
  source ./tests/run-in-kind/env.sh
fi

# list all components need to do test.
CHANGED_COMPONENTS=""
IMAGE=""

update_mco_cr() {
  if [ "${OPENSHIFT_CI}" == "true" ]; then
    # discard unstaged changes
    cd ${ROOTDIR} && git checkout -- .
  fi
  if [[ -n ${RBAC_QUERY_PROXY_IMAGE_REF} ]]; then
    "${SED_COMMAND[@]}" "/annotations.*/a \ \ \ \ mco-rbac_query_proxy-image: ${RBAC_QUERY_PROXY_IMAGE_REF}" ${ROOTDIR}/examples/mco/e2e/v1beta2/observability.yaml
    "${SED_COMMAND[@]}" "/annotations.*/a \ \ \ \ mco-rbac_query_proxy-image: ${RBAC_QUERY_PROXY_IMAGE_REF}" ${ROOTDIR}/examples/mco/e2e/v1beta2/*/observability.yaml
  fi
  if [[ -n ${ENDPOINT_MONITORING_OPERATOR_IMAGE_REF} ]]; then
    "${SED_COMMAND[@]}" "/annotations.*/a \ \ \ \ mco-endpoint_monitoring_operator-image: ${ENDPOINT_MONITORING_OPERATOR_IMAGE_REF}" ${ROOTDIR}/examples/mco/e2e/v1beta2/observability.yaml
    "${SED_COMMAND[@]}" "/annotations.*/a \ \ \ \ mco-endpoint_monitoring_operator-image: ${ENDPOINT_MONITORING_OPERATOR_IMAGE_REF}" ${ROOTDIR}/examples/mco/e2e/v1beta2/*/observability.yaml
  fi
  if [[ -n ${GRAFANA_DASHBOARD_LOADER_IMAGE_REF} ]]; then
    "${SED_COMMAND[@]}" "/annotations.*/a \ \ \ \ mco-grafana_dashboard_loader-image: ${GRAFANA_DASHBOARD_LOADER_IMAGE_REF}" ${ROOTDIR}/examples/mco/e2e/v1beta2/observability.yaml
    "${SED_COMMAND[@]}" "/annotations.*/a \ \ \ \ mco-grafana_dashboard_loader-image: ${GRAFANA_DASHBOARD_LOADER_IMAGE_REF}" ${ROOTDIR}/examples/mco/e2e/v1beta2/*/observability.yaml
  fi
  if [[ -n ${METRICS_COLLECTOR_IMAGE_REF} ]]; then
    "${SED_COMMAND[@]}" "/annotations.*/a \ \ \ \ mco-metrics_collector-image: ${METRICS_COLLECTOR_IMAGE_REF}" ${ROOTDIR}/examples/mco/e2e/v1beta2/observability.yaml
    "${SED_COMMAND[@]}" "/annotations.*/a \ \ \ \ mco-metrics_collector-image: ${METRICS_COLLECTOR_IMAGE_REF}" ${ROOTDIR}/examples/mco/e2e/v1beta2/*/observability.yaml
  fi
  if [[ -n ${OBSERVATORIUM_OPERATOR_IMAGE_REF} ]]; then
    "${SED_COMMAND[@]}" "/annotations.*/a \ \ \ \ mco-metrics_collector-image: ${OBSERVATORIUM_OPERATOR_IMAGE_REF}" ${ROOTDIR}/examples/mco/e2e/v1beta2/observability.yaml
    "${SED_COMMAND[@]}" "/annotations.*/a \ \ \ \ mco-metrics_collector-image: ${OBSERVATORIUM_OPERATOR_IMAGE_REF}" ${ROOTDIR}/examples/mco/e2e/v1beta2/*/observability.yaml
  fi
  if [[ -n ${MULTICLUSTER_OBSERVABILITY_ADDON_IMAGE_REF} ]]; then
    "${SED_COMMAND[@]}" "/annotations.*/a \ \ \ \ mco-multicluster_observability_addon-image: ${MULTICLUSTER_OBSERVABILITY_ADDON_IMAGE_REF}" ${ROOTDIR}/examples/mco/e2e/v1beta2/observability.yaml
    "${SED_COMMAND[@]}" "/annotations.*/a \ \ \ \ mco-multicluster_observability_addon-image: ${MULTICLUSTER_OBSERVABILITY_ADDON_IMAGE_REF}" ${ROOTDIR}/examples/mco/e2e/v1beta2/*/observability.yaml
  fi
  if [[ -n ${OBO_PROMETHEUS_OPERATOR_IMAGE_REF} ]]; then
    "${SED_COMMAND[@]}" "/annotations.*/a \ \ \ \ mco-obo_prometheus_rhel9_operator-image: ${OBO_PROMETHEUS_OPERATOR_IMAGE_REF}" ${ROOTDIR}/examples/mco/e2e/v1beta2/observability.yaml
    "${SED_COMMAND[@]}" "/annotations.*/a \ \ \ \ mco-obo_prometheus_rhel9_operator-image: ${OBO_PROMETHEUS_OPERATOR_IMAGE_REF}" ${ROOTDIR}/examples/mco/e2e/v1beta2/*/observability.yaml
  fi

  # Add mco-imageTagSuffix annotation
  # "${SED_COMMAND[@]}" "/annotations.*/a \ \ \ \ mco-imageTagSuffix: ${LATEST_SNAPSHOT}" ${ROOTDIR}/examples/mco/e2e/v1beta2/observability.yaml
  # "${SED_COMMAND[@]}" "/annotations.*/a \ \ \ \ mco-imageTagSuffix: ${LATEST_SNAPSHOT}" ${ROOTDIR}/examples/mco/e2e/v1beta2/custom-certs/observability.yaml
  # "${SED_COMMAND[@]}" "/annotations.*/a \ \ \ \ mco-imageTagSuffix: ${LATEST_SNAPSHOT}" ${ROOTDIR}/examples/mco/e2e/v1beta2/custom-certs-kind/observability.yaml

  # need to add this annotation due to KinD cluster resources are insufficient
  if [[ -n ${IS_KIND_ENV} ]]; then
    "${SED_COMMAND[@]}" "/annotations.*/a \ \ \ \ mco-thanos-without-resources-requests: true" ${ROOTDIR}/examples/mco/e2e/v1beta2/observability.yaml
    # annotate MCO in kind env to be able to install prometheus
    "${SED_COMMAND[@]}" "/annotations.*/a \ \ \ \ test-env: kind-test" ${ROOTDIR}/examples/mco/e2e/v1beta2/observability.yaml
    "${SED_COMMAND[@]}" "/annotations.*/a \ \ \ \ test-env: kind-test" ${ROOTDIR}/examples/mco/e2e/v1beta2/custom-certs-kind/observability.yaml

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
  if [[ -n "${GINKGO_FOCUS}" ]]; then
    echo "Using GINKGO_FOCUS from environment: ${GINKGO_FOCUS}"
    return
  fi
  if [[ -n ${IS_KIND_ENV} ]]; then
    # For KinD cluster, do not need to run all test cases
    # and we skip those that explictly requires OCP
    GINKGO_FOCUS=" --focus manifestwork/g --focus endpoint_preserve/g --focus grafana/g --focus metrics/g --focus addon/g --focus alert/g --focus dashboard/g --skip requires-ocp/g0"
  else
    GINKGO_FOCUS=""
  fi
  echo "Test focuses are ${GINKGO_FOCUS}"
}

# start executing
get_changed_components
update_mco_cr
get_ginkgo_focus
echo "${GINKGO_FOCUS}" >/tmp/ginkgo_focus
