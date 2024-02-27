#!/usr/bin/env bash

set -exo pipefail

ROOTDIR="$(
  cd "$(dirname "$0")/../.."
  pwd -P
)"
WORKDIR=${ROOTDIR}/tests/run-in-kind

export IS_KIND_ENV=true

# shellcheck disable=SC1091
source ${WORKDIR}/env.sh

deploy_service_ca_operator() {
  kubectl create ns openshift-config-managed
  kubectl apply -f ${WORKDIR}/service-ca/
}

deploy_crds() {
  kubectl apply -f ${WORKDIR}/req_crds/
}

deploy_templates() {
  kubectl apply -f ${WORKDIR}/templates/
}

deploy_openshift_router() {
  kubectl create ns openshift-ingress
  kubectl apply -f ${WORKDIR}/router/
}

deploy_all() {
  deploy_crds
  deploy_templates
  deploy_service_ca_operator
  deploy_openshift_router
}

$*
