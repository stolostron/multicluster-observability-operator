#!/usr/bin/env bash

# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

# Required KUBECONFIG environment variable to run this script:

set -exo pipefail

if [[ -z ${KUBECONFIG} ]]; then
  echo "Error: environment variable KUBECONFIG must be specified!"
  exit 1
fi

ROOTDIR="$(
  cd "$(dirname "$0")/.."
  pwd -P
)"

OCM_DEFAULT_NS="open-cluster-management"
AGENT_NS="open-cluster-management-agent"
HUB_NS="open-cluster-management-hub"
OBSERVABILITY_NS="open-cluster-management-observability"
IMAGE_REPO="quay.io/stolostron"
export MANAGED_CLUSTER="local-cluster" # registration-operator needs this

SED_COMMAND=${SED}' -i-e -e'

# Set the latest snapshot if it is not set
source ./scripts/test-utils.sh
LATEST_SNAPSHOT=${LATEST_SNAPSHOT:-$(get_latest_snapshot)}

# deploy the hub and spoke core via OLM
deploy_hub_spoke_core() {
  cd ${ROOTDIR}
  # we are pinned here so no need to re-fetch if we have the project locally.
  if [[ ! -d "registration-operator" ]]; then
    git clone --depth 1 -b release-2.4 https://github.com/stolostron/registration-operator.git
  fi
  cd registration-operator
  ${SED_COMMAND} "s~clusterName: cluster1$~clusterName: ${MANAGED_CLUSTER}~g" deploy/klusterlet/config/samples/operator_open-cluster-management_klusterlets.cr.yaml
  # deploy hub and spoke via OLM
  REGISTRATION_LATEST_SNAPSHOT='2.4.9-SNAPSHOT-2022-11-17-20-19-31'
  make cluster-ip IMAGE_REGISTRY=quay.io/stolostron IMAGE_TAG=${REGISTRATION_LATEST_SNAPSHOT} WORK_TAG=${REGISTRATION_LATEST_SNAPSHOT} REGISTRATION_TAG=${REGISTRATION_LATEST_SNAPSHOT} PLACEMENT_TAG=${REGISTRATION_LATEST_SNAPSHOT}
  make deploy IMAGE_REGISTRY=quay.io/stolostron IMAGE_TAG=${REGISTRATION_LATEST_SNAPSHOT} WORK_TAG=${REGISTRATION_LATEST_SNAPSHOT} REGISTRATION_TAG=${REGISTRATION_LATEST_SNAPSHOT} PLACEMENT_TAG=${REGISTRATION_LATEST_SNAPSHOT}

  # wait until hub and spoke are ready
  wait_for_deployment_ready 10 60s ${HUB_NS} cluster-manager-registration-controller cluster-manager-registration-webhook cluster-manager-work-webhook
  wait_for_deployment_ready 10 60s ${AGENT_NS} klusterlet-registration-agent klusterlet-work-agent
}

# approve the CSR for cluster join request
approve_csr_joinrequest() {
  echo "wait for CSR for cluster join reqest is created..."
  for i in {1..60}; do
    # TODO(morvencao): remove the hard-coded cluster label
    csrs=$(kubectl get csr -lopen-cluster-management.io/cluster-name=${MANAGED_CLUSTER})
    if [[ -n ${csrs} ]]; then
      csrnames=$(kubectl get csr -lopen-cluster-management.io/cluster-name=${MANAGED_CLUSTER} -o jsonpath={.items..metadata.name})
      for csrname in ${csrnames}; do
        echo "approve CSR: ${csrname}"
        kubectl certificate approve ${csrname}
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
    if [[ -n ${clusters} ]]; then
      clusternames=$(kubectl get managedcluster -o jsonpath={.items..metadata.name})
      for clustername in ${clusternames}; do
        echo "approve joinrequest for ${clustername}"
        kubectl patch managedcluster ${clustername} --patch '{"spec":{"hubAcceptsClient":true}}' --type=merge
        if [[ -n ${IS_KIND_ENV} ]]; then
          # update vendor label for KinD env
          kubectl label managedcluster ${clustername} vendor-
          kubectl label managedcluster ${clustername} vendor=GKE
        fi
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

# deploy the grafana-test to check the dashboards from browsers
deploy_grafana_test() {
  cd ${ROOTDIR}
  ${SED_COMMAND} "s~name: grafana$~name: grafana-test~g; s~app: multicluster-observability-grafana$~app: multicluster-observability-grafana-test~g; s~secretName: grafana-config$~secretName: grafana-config-test~g; s~secretName: grafana-datasources$~secretName: grafana-datasources-test~g; /MULTICLUSTEROBSERVABILITY_CR_NAME/d" operators/multiclusterobservability/manifests/base/grafana/deployment.yaml
  ${SED_COMMAND} "s~image: quay.io/stolostron/grafana-dashboard-loader:.*$~image: ${IMAGE_REPO}/grafana-dashboard-loader:${LATEST_SNAPSHOT}~g" operators/multiclusterobservability/manifests/base/grafana/deployment.yaml
  ${SED_COMMAND} "s~image: quay.io/stolostron/grafana:.*$~image: ${IMAGE_REPO}/grafana:${LATEST_SNAPSHOT}~g" operators/multiclusterobservability/manifests/base/grafana/deployment.yaml
  ${SED_COMMAND} "s~replicas: 2$~replicas: 1~g" operators/multiclusterobservability/manifests/base/grafana/deployment.yaml
  kubectl apply -f operators/multiclusterobservability/manifests/base/grafana/deployment.yaml
  kubectl apply -f ${ROOTDIR}/tests/run-in-kind/grafana # create grafana-test svc, grafana-test config and datasource configmaps

  if [[ -z ${IS_KIND_ENV} ]]; then
    # TODO(morvencao): remove the following two extra routes after after accessing metrics from grafana url with bearer token is supported
    temp_route=$(mktemp -d /tmp/grafana.XXXXXXXXXX)
    # install grafana-test route
    cat <<EOF >${temp_route}/grafana-test-route.yaml
apiVersion: route.openshift.io/v1
kind: Route
metadata:
  name: grafana-test
spec:
  host: grafana-test
  wildcardPolicy: None
  to:
    kind: Service
    name: grafana-test
EOF

    app_domain=$(kubectl -n openshift-ingress-operator get ingresscontrollers default -o jsonpath='{.status.domain}')
    ${SED_COMMAND} "s~host: grafana-test$~host: grafana-test.${app_domain}~g" ${temp_route}/grafana-test-route.yaml
    kubectl -n ${OBSERVABILITY_NS} apply -f ${temp_route}/grafana-test-route.yaml
  fi
}

# deploy the MCO operator via the kustomize resources
deploy_mco_operator() {
  # makes sure we get the MULTICLUSTER_OBSERVABILITY_OPERATOR_IMAGE_REF
  if [[ -n ${IS_KIND_ENV} ]]; then
    source ${ROOTDIR}/tests/run-in-kind/env.sh
  fi

  if [[ -n ${MULTICLUSTER_OBSERVABILITY_OPERATOR_IMAGE_REF} ]]; then
    cd ${ROOTDIR}/operators/multiclusterobservability/config/manager && kustomize edit set image quay.io/stolostron/multicluster-observability-operator=${MULTICLUSTER_OBSERVABILITY_OPERATOR_IMAGE_REF}
  else
    cd ${ROOTDIR}/operators/multiclusterobservability/config/manager && kustomize edit set image quay.io/stolostron/multicluster-observability-operator="${IMAGE_REPO}/multicluster-observability-operator:${LATEST_SNAPSHOT}"
  fi
  cd ${ROOTDIR}
  kustomize build ${ROOTDIR}/operators/multiclusterobservability/config/default | kubectl apply -n ${OCM_DEFAULT_NS} --server-side=true -f -

  # wait until mco is ready
  wait_for_deployment_ready 10 60s ${OCM_DEFAULT_NS} multicluster-observability-operator
  echo "mco operator is deployed successfully."

  kubectl create ns ${OBSERVABILITY_NS} || true
}

# wait for MCO CR reaadiness with budget
wait_for_observability_ready() {
  echo "wait for mco is ready and running..."
  retry_number=10
  timeout=60s
  for ((i = 1; i <= retry_number; i++)); do

    if kubectl wait --timeout=${timeout} --for=condition=Ready mco/observability &>/dev/null; then
      echo "Observability has been started up and is running."
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

# wait until deployment are ready with budget
wait_for_deployment_ready() {
  if [[ -z ${1} ]]; then
    echo "retry number is empty, exiting..."
  fi
  retry_number=${1}
  if [[ -z ${2} ]]; then
    echo "timeout is empty, exiting..."
  fi
  timeout=${2}
  if [[ -z ${3} ]]; then
    echo "namespace is empty, exiting..."
    exit 1
  fi
  ns=${3}
  if [[ -z ${4} ]]; then
    echo "at least one deployment should be specified, exiting..."
    exit 1
  fi

  echo "wait for deployment ${@:4} in namespace ${ns} are starting up and running..."
  for ((i = 1; i <= retry_number; i++)); do
    if ! kubectl get ns ${ns} &>/dev/null; then
      echo "namespace ${ns} is not created, retry in 10s...."
      sleep 10
      continue
    fi

    if ! kubectl -n ${ns} get deploy ${@:4} &>/dev/null; then
      echo "deployment ${@:4} are not created yet, retry in 10s...."
      sleep 10
      continue
    fi

    if kubectl -n ${ns} wait --timeout=${timeout} --for=condition=Available deploy ${@:4} &>/dev/null; then
      echo "deployment ${@:4} have been started up and are running."
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
  deploy_hub_spoke_core
  approve_csr_joinrequest
  deploy_mco_operator
  deploy_grafana_test
  echo "OCM and MCO are installed successfully..."
}

# start executing the ACTION
execute
