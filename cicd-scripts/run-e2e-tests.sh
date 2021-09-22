#!/usr/bin/env bash

# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

set -exo pipefail

TARGET_OS="$(uname)"
XARGS_FLAGS="-r"
SED_COMMAND='sed -i -e'
if [[ "$(uname)" == "Linux" ]]; then
    TARGET_OS=linux
elif [[ "$(uname)" == "Darwin" ]]; then
    TARGET_OS=darwin
    XARGS_FLAGS=
    SED_COMMAND='sed -i '-e' -e'
else
    echo "This system's OS $(TARGET_OS) isn't recognized/supported" && exit 1
fi

ROOTDIR="$(cd "$(dirname "$0")/.." ; pwd -P)"
${ROOTDIR}/cicd-scripts/customize-mco.sh
GINKGO_FOCUS="$(cat /tmp/ginkgo_focus)"

# need to modify sc for KinD
if [[ -n "${IS_KIND_ENV}" ]]; then
    $SED_COMMAND "s~gp2$~standard~g"  ${ROOTDIR}/examples/minio/minio-pvc.yaml
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
    kubectl config view --raw --minify > ${kubeconfig_hub_path}
fi

kubecontext=$(kubectl config current-context)
cluster_name="local-cluster"

if [[ -n "${IS_KIND_ENV}" ]]; then
    clusterServerURL="https://127.0.0.1:32806"
    base_domain="placeholder"
else
    clusterServerURL=$(kubectl config view -o jsonpath="{.clusters[0].cluster.server}")
    app_domain=$(kubectl -n openshift-ingress-operator get ingresscontrollers default -ojsonpath='{.status.domain}')
    base_domain="${app_domain#apps.}"
    kubectl apply -f ${ROOTDIR}/operators/multiclusterobservability/config/crd/bases
fi

OPTIONSFILE=${ROOTDIR}/tests/resources/options.yaml
# remove the options file if it exists
rm -f ${OPTIONSFILE}

printf "options:" >> ${OPTIONSFILE}
printf "\n  kubeconfig: ${kubeconfig_hub_path}" >> ${OPTIONSFILE}
printf "\n  hub:" >> ${OPTIONSFILE}
printf "\n    clusterServerURL: ${clusterServerURL}" >> ${OPTIONSFILE}
printf "\n    kubeconfig: ${kubeconfig_hub_path}" >> ${OPTIONSFILE}
printf "\n    kubecontext: ${kubecontext}" >> ${OPTIONSFILE}
printf "\n    baseDomain: ${base_domain}" >> ${OPTIONSFILE}
if [[ -n "${IS_KIND_ENV}" ]]; then
    printf "\n    grafanaURL: http://127.0.0.1:31001" >> ${OPTIONSFILE}
    printf "\n    grafanaHost: grafana-test" >> ${OPTIONSFILE}
fi
printf "\n  clusters:" >> ${OPTIONSFILE}
printf "\n    - name: ${cluster_name}" >> ${OPTIONSFILE}
if [[ -n "${IS_KIND_ENV}" ]]; then
    printf "\n      clusterServerURL: ${clusterServerURL}" >> ${OPTIONSFILE}
fi
printf "\n      baseDomain: ${base_domain}" >> ${OPTIONSFILE}
printf "\n      kubeconfig: ${kubeconfig_hub_path}" >> ${OPTIONSFILE}
printf "\n      kubecontext: ${kubecontext}" >> ${OPTIONSFILE}

go get -u github.com/onsi/ginkgo/ginkgo
go mod vendor
if command -v ginkgo &> /dev/null; then
    GINKGO_CMD=ginkgo
else
    # just for Prow KinD vm
    GINKGO_CMD="/home/ec2-user/go/bin/ginkgo"
fi
$GINKGO_CMD -debug -trace ${GINKGO_FOCUS} -v ${ROOTDIR}/tests/pkg/tests -- -options=${OPTIONSFILE} -v=3

cat ${ROOTDIR}/tests/pkg/tests/results.xml | grep failures=\"0\" | grep errors=\"0\"
if [ $? -ne 0 ]; then
    echo "Cannot pass all test cases."
    cat ${ROOTDIR}/tests/pkg/tests/results.xml
    exit 1
fi
