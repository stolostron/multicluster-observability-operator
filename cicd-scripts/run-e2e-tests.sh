#!/usr/bin/env bash

# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

set -exo pipefail

ROOTDIR="$(
  cd "$(dirname "$0")/.."
  pwd -P
)"

SED_COMMAND=${SED}' -i-e -e'

# customize the images for testing
export MULTICLUSTER_OBSERVABILITY_ADDON_IMAGE_REF="quay.io/rhobs/multicluster-observability-addon:latest"
export OBO_PROMETHEUS_OPERATOR_IMAGE_REF="quay.io/rhobs/obo-prometheus-operator:v0.82.1-rhobs1"
${ROOTDIR}/cicd-scripts/customize-mco.sh
GINKGO_FOCUS="$(cat /tmp/ginkgo_focus)"

# need to modify sc for KinD
if [[ -n ${IS_KIND_ENV} ]]; then
  ${SED_COMMAND} "s~gp3-csi$~standard~g" ${ROOTDIR}/examples/minio/minio-pvc.yaml
  ${SED_COMMAND} "s~gp3-csi$~standard~g" ${ROOTDIR}/examples/minio-tls/minio-pvc.yaml
fi

kubeconfig_hub_path=""
if [ ! -z "${SHARED_DIR}" ]; then
  export KUBECONFIG="${SHARED_DIR}/hub-1.kc"
  kubeconfig_hub_path="${SHARED_DIR}/hub-1.kc"
else
  # for local testing
  if [ -z "${KUBECONFIG}" ]; then
    echo "Error: environment variable KUBECONFIG must be specified!"
    exit 1
  fi
  kubeconfig_hub_path="${HOME}/.kube/kubeconfig-hub"
  kubectl config view --raw --minify >${kubeconfig_hub_path}
fi

kubecontext=$(kubectl config current-context)
cluster_name="local-cluster"

if [[ -n ${IS_KIND_ENV} ]]; then
  clusterServerURL="https://127.0.0.1:32806"
  base_domain="placeholder"
else
  clusterServerURL=$(kubectl config view -o jsonpath="{.clusters[0].cluster.server}")
  app_domain=$(kubectl -n openshift-ingress-operator get ingresscontrollers default -ojsonpath='{.status.domain}')
  base_domain="${app_domain#apps.}"
  kubectl apply -f ${ROOTDIR}/operators/multiclusterobservability/config/crd/bases --server-side=true --force-conflicts
fi

OPTIONSFILE=${ROOTDIR}/tests/resources/options.yaml
# remove the options file if it exists
rm -f ${OPTIONSFILE}

printf "options:" >>${OPTIONSFILE}
printf "\n  kubeconfig: ${kubeconfig_hub_path}" >>${OPTIONSFILE}
printf "\n  hub:" >>${OPTIONSFILE}
printf "\n    clusterServerURL: ${clusterServerURL}" >>${OPTIONSFILE}
printf "\n    kubeconfig: ${kubeconfig_hub_path}" >>${OPTIONSFILE}
printf "\n    kubecontext: ${kubecontext}" >>${OPTIONSFILE}
printf "\n    baseDomain: ${base_domain}" >>${OPTIONSFILE}
if [[ -n ${IS_KIND_ENV} ]]; then
  printf "\n    grafanaURL: http://127.0.0.1:31001" >>${OPTIONSFILE}
  printf "\n    grafanaHost: grafana-test" >>${OPTIONSFILE}
fi
printf "\n  clusters:" >>${OPTIONSFILE}

# Check if there's a separate managed cluster (not local-cluster)
# Priority order:
# 1. CI clusterpool: ${SHARED_DIR}/managed-1.kc
# 2. Explicit env vars: MANAGED_CLUSTER_KUBECONFIG or IMPORT_KUBECONFIG
# 3. Fallback to local-cluster only

managed_cluster_name=""
managed_cluster_kubeconfig=""

if [[ -n ${SHARED_DIR} ]] && [[ -f ${SHARED_DIR}/managed-1.kc ]]; then
  # CI environment with clusterpool
  managed_cluster_name="managed"
  managed_cluster_kubeconfig="${SHARED_DIR}/managed-1.kc"
  echo "Detected CI clusterpool environment: ${SHARED_DIR}/managed-1.kc"
elif [[ -n ${MANAGED_CLUSTER_NAME} ]] && [[ ${MANAGED_CLUSTER_NAME} != "local-cluster" ]]; then
  # Explicit environment variables
  managed_cluster_name=${MANAGED_CLUSTER_NAME}
  managed_cluster_kubeconfig=${MANAGED_CLUSTER_KUBECONFIG:-${IMPORT_KUBECONFIG}}
fi

if [[ -n ${managed_cluster_name} ]] && [[ -f ${managed_cluster_kubeconfig} ]]; then
  # Add managed cluster as the first entry
  managed_base_domain=${MANAGED_CLUSTER_BASE_DOMAIN:-""}
  managed_api_url=${MANAGED_CLUSTER_API_URL:-""}

  # Auto-extract API URL from kubeconfig if not provided
  if [[ -z ${managed_api_url} ]]; then
    managed_api_url=$(kubectl --kubeconfig=${managed_cluster_kubeconfig} config view -o jsonpath="{.clusters[0].cluster.server}" 2>/dev/null || echo "")
    if [[ -n ${managed_api_url} ]]; then
      echo "Extracted API URL from managed cluster kubeconfig: ${managed_api_url}"
    fi
  fi

  # Auto-extract base domain from API URL
  if [[ -z ${managed_base_domain} ]] && [[ -n ${managed_api_url} ]]; then
    managed_base_domain=$(echo ${managed_api_url} | sed -E 's|https://api\.([^:]+).*|\1|')
    if [[ -n ${managed_base_domain} ]]; then
      echo "Extracted base domain from API URL: ${managed_base_domain}"
    fi
  fi

  # construct API URL from base domain if we have it
  if [[ -z ${managed_api_url} ]] && [[ -n ${managed_base_domain} ]]; then
    managed_api_url="https://api.${managed_base_domain}:6443"
  fi

  printf "\n    - name: ${managed_cluster_name}" >>${OPTIONSFILE}
  if [[ -n ${managed_api_url} ]]; then
    printf "\n      clusterServerURL: ${managed_api_url}" >>${OPTIONSFILE}
  fi
  if [[ -n ${managed_base_domain} ]]; then
    printf "\n      baseDomain: ${managed_base_domain}" >>${OPTIONSFILE}
  fi
  printf "\n      kubeconfig: ${managed_cluster_kubeconfig}" >>${OPTIONSFILE}

  echo "Added managed cluster to options.yaml: ${managed_cluster_name} (kubeconfig: ${managed_cluster_kubeconfig})"
else
  # No separate managed cluster - add local-cluster only (for KinD or hub-only testing)
  printf "\n    - name: ${cluster_name}" >>${OPTIONSFILE}
  if [[ -n ${IS_KIND_ENV} ]]; then
    printf "\n      clusterServerURL: ${clusterServerURL}" >>${OPTIONSFILE}
  fi
  printf "\n      baseDomain: ${base_domain}" >>${OPTIONSFILE}
  printf "\n      kubeconfig: ${kubeconfig_hub_path}" >>${OPTIONSFILE}
  printf "\n      kubecontext: ${kubecontext}" >>${OPTIONSFILE}

  echo "No separate managed cluster found - addon tests will be skipped"
fi

if command -v ginkgo &>/dev/null; then
  GINKGO_CMD=ginkgo
else
  # just for Prow KinD vm
  # uninstall old go version(1.16) and install new version
  wget -nv https://go.dev/dl/go1.24.9.linux-amd64.tar.gz
  if command -v sudo >/dev/null 2>&1; then
    sudo rm -fr /usr/local/go
    sudo tar -C /usr/local -xzf go1.24.9.linux-amd64.tar.gz
  # else
  #     rm -fr /usr/local/go
  #     tar -C /usr/local -xzf go1.21.10.linux-amd64.tar.gz
  fi
  go install github.com/onsi/ginkgo/v2/ginkgo@v2.23.4
  GINKGO_CMD="$(go env GOPATH)/bin/ginkgo"
fi

go mod vendor
${GINKGO_CMD} --no-color --junit-report=${ROOTDIR}/tests/pkg/tests/results.xml -debug -trace ${GINKGO_FOCUS} -v ${ROOTDIR}/tests/pkg/tests -- -options=${OPTIONSFILE} -v=6

cat ${ROOTDIR}/tests/pkg/tests/results.xml | grep failures=\"0\" | grep errors=\"0\"
if [ $? -ne 0 ]; then
  echo "Cannot pass all test cases."
  cat ${ROOTDIR}/tests/pkg/tests/results.xml
  # The underlying cluster is still deleted. Setting large timeout won't help
  # echo "sleeping for 60 min"
  # sleep 3600
  # echo "waking up from sleep"
  exit 1
fi
