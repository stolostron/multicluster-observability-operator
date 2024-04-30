#!/usr/bin/env bash

# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

#set -exo pipefail

set -x

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

kubecontext=$(kubectl config current-context)
hub_cluster_name="local-cluster"

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
printf "\n    - name: ${hub_cluster_name}" >>${OPTIONSFILE}
if [[ -n ${IS_KIND_ENV} ]]; then
  printf "\n      clusterServerURL: ${clusterServerURL}" >>${OPTIONSFILE}
fi
printf "\n      baseDomain: ${base_domain}" >>${OPTIONSFILE}
printf "\n      kubeconfig: ${kubeconfig_hub_path}" >>${OPTIONSFILE}
printf "\n      kubecontext: ${kubecontext}" >>${OPTIONSFILE}

kubeconfig_managed_path="${SHARED_DIR}/managed-1.kc"
if [[ -z ${IS_KIND_ENV} && -f "${kubeconfig_managed_path}" ]]; then
  managed_cluster_name="managed-cluster-1"
  kubecontext_managed=$(kubectl --kubeconfig="${kubeconfig_managed_path}" config current-context)
  app_domain_managed=$(kubectl -n openshift-ingress-operator --kubeconfig="${kubeconfig_managed_path}" get ingresscontrollers default -ojsonpath='{.status.domain}')
  base_domain_managed="${app_domain_managed#apps.}"
  printf "\n    - name: ${managed_cluster_name}" >>${OPTIONSFILE}
  printf "\n      baseDomain: ${base_domain_managed}" >>${OPTIONSFILE}
  printf "\n      kubeconfig: ${kubeconfig_managed_path}" >>${OPTIONSFILE}
  printf "\n      kubecontext: ${kubecontext_managed}" >>${OPTIONSFILE}
fi

if command -v ginkgo &>/dev/null; then
  GINKGO_CMD=ginkgo
else
  # just for Prow KinD vm
  # uninstall old go version(1.16) and install new version
  wget -nv https://go.dev/dl/go1.20.4.linux-amd64.tar.gz
  if command -v sudo >/dev/null 2>&1; then
    sudo rm -fr /usr/local/go
    sudo tar -C /usr/local -xzf go1.20.4.linux-amd64.tar.gz
  # else
  #     rm -fr /usr/local/go
  #     tar -C /usr/local -xzf go1.20.4.linux-amd64.tar.gz
  fi
  go install github.com/onsi/ginkgo/ginkgo@latest
  GINKGO_CMD="$(go env GOPATH)/bin/ginkgo"
fi

go mod vendor
${GINKGO_CMD} -debug -trace ${GINKGO_FOCUS} -v ${ROOTDIR}/tests/pkg/tests -- -options=${OPTIONSFILE} -v=5

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
