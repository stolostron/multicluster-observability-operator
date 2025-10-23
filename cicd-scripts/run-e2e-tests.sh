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

# TODO checking if kube context can be switched here to remove need for cm-cli
echo "Kube Contexts: $(kubectl config get-contexts)"

# Only run clusteradm commands if NOT in a kind environment
if [[ -z ${IS_KIND_ENV} ]] && [[ ! -z "${SHARED_DIR}" ]] && [[ -f "${SHARED_DIR}/managed-1.kc" ]]; then
    # Extract managed cluster info from JSON
  set +x  
  MANAGED_CLUSTER_API_URL=$(jq -r '.api_url' "${SHARED_DIR}/managed-1.json")
  MANAGED_CLUSTER_USER=$(jq -r '.username' "${SHARED_DIR}/managed-1.json")
  MANAGED_CLUSTER_PASS=$(jq -r '.password' "${SHARED_DIR}/managed-1.json")

  # Extract hub cluster info from JSON
  HUB_API_URL=$(jq -r '.api_url' "${SHARED_DIR}/hub-1.json")
  HUB_USER=$(jq -r '.username' "${SHARED_DIR}/hub-1.json")
  HUB_PASS=$(jq -r '.password' "${SHARED_DIR}/hub-1.json")
  set -x 

  # join clusters hub and managed cluster
  clusteradm init || true
  set +x
  HUB_TOKEN=$(clusteradm get token) || true
  # Switch context to managed cluster
  oc login --insecure-skip-tls-verify -u "$MANAGED_CLUSTER_USER" -p "$MANAGED_CLUSTER_PASS" "$MANAGED_CLUSTER_API_URL"  || true
  set -x
  clusteradm join --hub-token ${HUB_TOKEN} --hub-api-server ${HUB_API_SERVER} --cluster-name ${MANAGED_CLUSTER_NAME} || true
  set +x
  oc login --insecure-skip-tls-verify -u "$HUB_USER" -p "$HUB_PASS" "$HUB_API_URL" || true
  set -x
  # Set kubeconfig back to hub
  export KUBECONFIG="${SHARED_DIR}/hub-1.kc"  
  clusteradm accept --clusters ${MANAGED_CLUSTER_NAME} || true
fi

# After login to managed cluster 
echo "Kube Contexts: $(kubectl config get-contexts)"

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
printf "\n    - name: ${cluster_name}" >>${OPTIONSFILE}
if [[ -n ${IS_KIND_ENV} ]]; then
  printf "\n      clusterServerURL: ${clusterServerURL}" >>${OPTIONSFILE}
fi
printf "\n      baseDomain: ${base_domain}" >>${OPTIONSFILE}
printf "\n      kubeconfig: ${kubeconfig_hub_path}" >>${OPTIONSFILE}
printf "\n      kubecontext: ${kubecontext}" >>${OPTIONSFILE}

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
