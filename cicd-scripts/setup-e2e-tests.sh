#!/usr/bin/env bash

# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

# Required KUBECONFIG environment variable to run this script:

set -exo pipefail

OBSERVABILITY_NS="open-cluster-management-observability"
OCM_DEFAULT_NS="open-cluster-management"
AGENT_NS="open-cluster-management-agent"
HUB_NS="open-cluster-management-hub"
export MANAGED_CLUSTER="local-cluster"
COMPONENT_REPO="quay.io/open-cluster-management"

ROOTDIR="$(cd "$(dirname "$0")/.." ; pwd -P)"

# Create bin directory and add it to PATH
mkdir -p ${ROOTDIR}/bin
export PATH=${PATH}:${ROOTDIR}/bin

if ! command -v jq &> /dev/null; then
    if [[ "$(uname)" == "Linux" ]]; then
        curl -o jq -L https://github.com/stedolan/jq/releases/download/jq-1.6/jq-linux64
    elif [[ "$(uname)" == "Darwin" ]]; then
        curl -o jq -L https://github.com/stedolan/jq/releases/download/jq-1.6/jq-osx-amd64
    fi
    chmod +x ./jq && mv ./jq ${ROOTDIR}/bin/jq
fi

function usage() {
  echo "${0} -a ACTION [-i IMAGES]"
  echo ''
  # shellcheck disable=SC2016
  echo '  -a: Specifies the ACTION name, required, the value could be "install" or "uninstall".'
  # shellcheck disable=SC2016
  echo '  -i: Specifies the desired IMAGES, optional, the support images includes:
        quay.io/open-cluster-management/multicluster-observability-operator:<tag>
        quay.io/open-cluster-management/rbac-query-proxy:<tag>
        quay.io/open-cluster-management/grafana-dashboard-loader:<tag>
        quay.io/open-cluster-management/metrics-collector:<tag>
        quay.io/open-cluster-management/endpoint-monitoring-operator:<tag>'
  # shellcheck disable=SC2016
  echo '  -p: Specifies the pipeline for the images'
  echo ''
}

# Allow command-line args to override the defaults.
while getopts ":a:i:p:h" opt; do
  case ${opt} in
    a)
      ACTION=${OPTARG}
      ;;
    i)
      IMAGES=${OPTARG}
      ;;
    p)
      PIPELINE=${OPTARG}
      ;;
    h)
      usage
      exit 0
      ;;
    \?)
      echo "Invalid option: -$OPTARG" >&2
      usage
      exit 1
      ;;
  esac
done

if [[ -z "${ACTION}" ]]; then
  echo "Error: ACTION (-a) must be specified!"
  usage
  exit 1
fi

if [[ -z "${KUBECONFIG}" ]]; then
  echo "Error: environment variable KUBECONFIG must be specified!"
  exit 1
fi

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

# Use snapshot for target release. Use latest one if no branch info detected, or not a release branch
BRANCH=""
LATEST_SNAPSHOT=""
if [[ "${PULL_BASE_REF}" == "release-"* ]]; then
    BRANCH=${PULL_BASE_REF#"release-"}
    BRANCH=`curl https://quay.io//api/v1/repository/open-cluster-management/multicluster-observability-operator | jq '.tags|with_entries(select(.key|contains("'${BRANCH}'")))|keys[length-1]' | awk -F '-' '{print $1}'`
    BRANCH="${BRANCH#\"}"
    LATEST_SNAPSHOT=`curl https://quay.io//api/v1/repository/open-cluster-management/multicluster-observability-operator | jq '.tags|with_entries(select(.key|contains("'${BRANCH}'-SNAPSHOT")))|keys[length-1]'`
fi
if [[ "${LATEST_SNAPSHOT}" == "null" ]] || [[ "${LATEST_SNAPSHOT}" == "" ]]; then
    LATEST_SNAPSHOT=$(curl https://quay.io/api/v1/repository/open-cluster-management/multicluster-observability-operator | jq '.tags|with_entries(select(.key|contains("SNAPSHOT")))|keys[length-1]')
fi

# trim the leading and tailing quotes
LATEST_SNAPSHOT="${LATEST_SNAPSHOT#\"}"
LATEST_SNAPSHOT="${LATEST_SNAPSHOT%\"}"

setup_kubectl() {
    if ! command -v kubectl &> /dev/null; then
        echo "This script will install kubectl (https://kubernetes.io/docs/tasks/tools/install-kubectl/) on your machine"
        if [[ "$(uname)" == "Linux" ]]; then
            curl -LO https://storage.googleapis.com/kubernetes-release/release/v1.21.0/bin/linux/amd64/kubectl
        elif [[ "$(uname)" == "Darwin" ]]; then
            curl -LO https://storage.googleapis.com/kubernetes-release/release/v1.21.0/bin/darwin/amd64/kubectl
        fi
        chmod +x ./kubectl && mv ./kubectl ${ROOTDIR}/bin/kubectl
    fi
}

setup_kustomize() {
    if ! command -v kustomize &> /dev/null; then
        echo "This script will install kustomize (sigs.k8s.io/kustomize/kustomize) on your machine"
        if [[ "$(uname)" == "Linux" ]]; then
            curl -o kustomize_v3.8.7.tar.gz -L https://github.com/kubernetes-sigs/kustomize/releases/download/kustomize%2Fv3.8.7/kustomize_v3.8.7_linux_amd64.tar.gz
        elif [[ "$(uname)" == "Darwin" ]]; then
            curl -o kustomize_v3.8.7.tar.gz -L  https://github.com/kubernetes-sigs/kustomize/releases/download/kustomize%2Fv3.8.7/kustomize_v3.8.7_darwin_amd64.tar.gz
        fi
        tar xzvf kustomize_v3.8.7.tar.gz
        chmod +x ./kustomize && mv ./kustomize ${ROOTDIR}/bin/kustomize
    fi
}

deploy_hub_spoke_core() {
    cd ${ROOTDIR}
    if [ -d "registration-operator" ]; then
        rm -rf registration-operator
    fi
    git clone --depth 1 -b release-2.3 https://github.com/open-cluster-management/registration-operator.git && cd registration-operator
    $SED_COMMAND "s~clusterName: cluster1$~clusterName: $MANAGED_CLUSTER~g" deploy/klusterlet/config/samples/operator_open-cluster-management_klusterlets.cr.yaml
    # deploy hub and spoke via OLM
    make cluster-ip
    make deploy

    # wait until hub and spoke are ready
    wait_for_deployment_ready 10 60s ${HUB_NS} cluster-manager-registration-controller cluster-manager-registration-webhook cluster-manager-work-webhook
    wait_for_deployment_ready 10 60s ${AGENT_NS} klusterlet-registration-agent klusterlet-work-agent
}

delete_hub_spoke_core() {
    cd ${ROOTDIR}/registration-operator
    # uninstall hub and spoke via OLM
    make clean-deploy

    rm -rf ${ROOTDIR}/registration-operator
    oc delete ns ${OCM_DEFAULT_NS} --ignore-not-found
}

approve_csr_joinrequest() {
    echo "wait for CSR for cluster join reqest is created..."
    for i in {1..60}; do
        # TODO(morvencao): remove the hard-coded cluster label
        csrs=$(kubectl get csr -lopen-cluster-management.io/cluster-name=${MANAGED_CLUSTER})
        if [[ ! -z ${csrs} ]]; then
            csrnames=$(kubectl get csr -lopen-cluster-management.io/cluster-name=${MANAGED_CLUSTER} -o jsonpath={.items..metadata.name})
            for csrname in ${csrnames}; do
                echo "approve CSR: $csrname"
                kubectl certificate approve $csrname
            done
            break
        fi
        if [[ ${i} -eq 60 ]]; then
            echo "timeout wait for CSR is created."
            exit 1
        fi
        echo "retrying in 10s..."
        sleep 10
    done

    for i in {1..20}; do
        clusters=$(kubectl get managedcluster)
        if [[ ! -z ${clusters} ]]; then
            clusternames=$(kubectl get managedcluster -o jsonpath={.items..metadata.name})
            for clustername in ${clusternames}; do
                echo "approve joinrequest for ${clustername}"
                kubectl patch managedcluster ${clustername} --patch '{"spec":{"hubAcceptsClient":true}}' --type=merge
                kubectl label managedcluster ${clustername} vendor=GKE --overwrite
            done
            break
        fi
        if [[ ${i} -eq 20 ]]; then
            echo "timeout wait for managedcluster is created."
            exit 1
        fi
        echo "retrying in 10s..."
        sleep 10
    done
}

delete_csr() {
    kubectl delete csr -lopen-cluster-management.io/cluster-name=${MANAGED_CLUSTER} --ignore-not-found
}

# deploy the new grafana to check the dashboards from browsers
deploy_grafana_test() {
    cd ${ROOTDIR}/operators/multiclusterobservability/
    $SED_COMMAND "s~name: grafana$~name: grafana-test~g; s~app: multicluster-observability-grafana$~app: multicluster-observability-grafana-test~g; s~secretName: grafana-config$~secretName: grafana-config-test~g; s~secretName: grafana-datasources$~secretName: grafana-datasources-test~g; /MULTICLUSTEROBSERVABILITY_CR_NAME/d" manifests/base/grafana/deployment.yaml
    $SED_COMMAND "s~image: quay.io/open-cluster-management/grafana-dashboard-loader.*$~image: $COMPONENT_REPO/grafana-dashboard-loader:$LATEST_SNAPSHOT~g" manifests/base/grafana/deployment.yaml
    $SED_COMMAND "s~replicas: 2$~replicas: 1~g" manifests/base/grafana/deployment.yaml
    $SED_COMMAND "s~name: grafana$~name: grafana-test~g; s~app: multicluster-observability-grafana$~app: multicluster-observability-grafana-test~g" manifests/base/grafana/service.yaml
    kubectl apply -f manifests/base/grafana/deployment.yaml
    kubectl apply -f ${ROOTDIR}/tests/run-in-kind/grafana
}

deploy_mco_operator() {
    # we need to change the mco operator img in Prow KinD cluster
    if [[ -n "${IS_KIND_ENV}" ]]; then
        cd ${ROOTDIR}/operators/multiclusterobservability/config/manager && kustomize edit set image quay.io/open-cluster-management/multicluster-observability-operator=${MULTICLUSTER_OBSERVABILITY_OPERATOR_IMAGE_REF}
    fi
    kustomize build ${ROOTDIR}/operators/multiclusterobservability/config/default | kubectl apply -n ${OCM_DEFAULT_NS} -f -
    echo "mco operator is deployed successfully."

    # wait until mco is ready
    wait_for_deployment_ready 10 60s ${OCM_DEFAULT_NS} multicluster-observability-operator

    kubectl create ns ${OBSERVABILITY_NS} || true

    if [[ -z "${IS_KIND_ENV}" ]]; then
        # TODO(morvencao): remove the following two extra routes after after accessing metrics from grafana url with bearer token is supported
        temp_route=$(mktemp -d /tmp/grafana.XXXXXXXXXX)
        # install grafana route
        cat << EOF > ${temp_route}/grafana-route.yaml
kind: Route
apiVersion: route.openshift.io/v1
metadata:
  name: grafana
spec:
  host: grafana
  wildcardPolicy: None
  to:
    kind: Service
    name: grafana
EOF
        # install observability-thanos-query-frontend route
        cat << EOF > ${temp_route}/observability-thanos-query-frontend-route.yaml
kind: Route
apiVersion: route.openshift.io/v1
metadata:
  name: observability-thanos-query-frontend
spec:
  host: observability-thanos-query-frontend
  port:
    targetPort: http
  to:
    kind: Service
    name: observability-thanos-query-frontend
  wildcardPolicy: None
EOF
        app_domain=$(kubectl -n openshift-ingress-operator get ingresscontrollers default -o jsonpath='{.status.domain}')
        ${SED_COMMAND} "s~host: grafana$~host: grafana.$app_domain~g" ${temp_route}/grafana-route.yaml
        kubectl -n ${OBSERVABILITY_NS} apply -f ${temp_route}/grafana-route.yaml
        ${SED_COMMAND} "s~host: observability-thanos-query-frontend$~host: observability-thanos-query-frontend.$app_domain~g" ${temp_route}/observability-thanos-query-frontend-route.yaml
        kubectl -n ${OBSERVABILITY_NS} apply -f ${temp_route}/observability-thanos-query-frontend-route.yaml
    fi
}

delete_mco_operator() {
    # delete mco CR if it exists
    kubectl delete multiclusterobservabilities --all

    # delete extra routes if they exist
    kubectl -n ${OBSERVABILITY_NS} delete route --all

    kubectl -n ${OBSERVABILITY_NS} delete -k ${ROOTDIR}/examples/minio --ignore-not-found

    # wait until all resources are deleted before delete the mco
    for i in {1..20}; do
        if [[ -z $(kubectl -n ${OBSERVABILITY_NS} get all) ]]; then
            echo "all the resources in ${OBSERVABILITY_NS} namespace are removed."
            break
        fi
        if [[ ${i} -eq 20 ]]; then
            echo "timeout wait for the resources in ${OBSERVABILITY_NS} namespace are removed."
            exit 1
        fi
        echo "retrying in 10s..."
        sleep 10
    done

    # delete the mco
    # don't delete the ${OCM_DEFAULT_NS} namespace at this step, since ACM is there
    ${SED_COMMAND} '0,/^---$/d' ${ROOTDIR}/operators/multiclusterobservability/config/manager/manager.yaml
    kustomize build ${ROOTDIR}/operators/multiclusterobservability/config/default | kubectl delete --ignore-not-found -f -
    kubectl delete ns ${OBSERVABILITY_NS}
}

wait_for_observability_ready() {
    echo "wait for mco is ready and running..."
    retry_number=10
    timeout=60s
    for (( i = 1; i <= ${retry_number}; i++ )) ; do

        if kubectl wait --timeout=${timeout} --for=condition=Ready mco/observability &> /dev/null; then
            echo "Observability has been started up and is runing."
            break
        else
            echo "timeout wait for mco are ready, retry in 10s...."
            sleep 10
            continue
        fi
        if [[ ${i} -eq ${retry_number} ]]; then
            echo "timeout wait for mco is ready."
            exit 1
        fi
    done
}

wait_for_deployment_ready() {
    if [[ -z "${1}" ]]; then
        echo "retry number is empty, exiting..."
    fi
    retry_number=${1}
    if [[ -z "${2}" ]]; then
        echo "timeout is empty, exiting..."
    fi
    timeout=${2}
    if [[ -z "${3}" ]]; then
        echo "namespace is empty, exiting..."
        exit 1
    fi
    ns=${3}
    if [[ -z "${4}" ]]; then
        echo "at least one deployment should be specified, exiting..."
        exit 1
    fi

    echo "wait for deployment ${@:4} in namespace ${ns} are starting up and running..."
    for (( i = 1; i <= ${retry_number}; i++ )) ; do
        if ! kubectl get ns ${ns} &> /dev/null; then
            echo "namespace ${ns} is not created, retry in 10s...."
            sleep 10
            continue
        fi

        if ! kubectl -n ${ns} get deploy ${@:4} &> /dev/null; then
            echo "deployment ${@:4} are not created yet, retry in 10s...."
            sleep 10
            continue
        fi

        if kubectl -n ${ns} wait --timeout=${timeout} --for=condition=Available deploy ${@:4} &> /dev/null; then
            echo "deployment ${@:4} have been started up and are runing."
            break
        else
            echo "timeout wait for deployment ${@:4} are ready, retry in 10s...."
            sleep 10
            continue
        fi
        if [[ ${i} -eq ${retry_number} ]]; then
            echo "timeout wait for deployment ${@:4} are ready."
            exit 1
        fi
    done
}

# function execute is the main routine to do the actual work
execute() {
    setup_kubectl
    setup_kustomize
    if [[ "${ACTION}" == "install" ]]; then
        deploy_hub_spoke_core
        approve_csr_joinrequest
        deploy_mco_operator
        deploy_grafana_test
        echo "OCM and MCO are installed successfuly..."
    elif [[ "${ACTION}" == "uninstall" ]]; then
        delete_mco_operator
        delete_hub_spoke_core
        delete_csr
        echo "OCM and MCO are uninstalled successfuly..."
    else
        echo "This ACTION ${ACTION} isn't recognized/supported" && exit 1
    fi
}

# start executing the ACTION
execute
