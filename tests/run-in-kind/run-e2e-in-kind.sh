#!/usr/bin/env bash

set -euxo pipefail

export IS_KIND_ENV=true

# shellcheck disable=SC1091
source ./image_ref_env.sh

setup_kubectl_command() {
    if ! command -v kubectl >/dev/null 2>&1; then 
        echo "This script will install kubectl (https://kubernetes.io/docs/tasks/tools/install-kubectl/) on your machine"
        if [[ "$(uname)" == "Linux" ]]; then
            curl -LO https://storage.googleapis.com/kubernetes-release/release/v1.18.0/bin/linux/amd64/kubectl
        elif [[ "$(uname)" == "Darwin" ]]; then
            curl -LO https://storage.googleapis.com/kubernetes-release/release/v1.18.0/bin/darwin/amd64/kubectl
        fi
        chmod +x ./kubectl
        sudo mv ./kubectl /usr/local/bin/kubectl
    fi
}

create_kind_cluster() {
    if ! command -v kind >/dev/null 2>&1; then 
        echo "This script will install kind (https://kind.sigs.k8s.io/) on your machine."
        curl -Lo ./kind-amd64 "https://kind.sigs.k8s.io/dl/v0.10.0/kind-$(uname)-amd64"
        chmod +x ./kind-amd64
        sudo mv ./kind-amd64 /usr/local/bin/kind
    fi
    echo "Delete the KinD cluster if exists"
    kind delete cluster --name $1 || true

    echo "Start KinD cluster with the default cluster name - $1"
    rm -rf $HOME/.kube/kind-config-$1
    kind create cluster --kubeconfig $HOME/.kube/kind-config-$1 --name $1 --config ./kind/kind-$1.config.yaml
    export KUBECONFIG=$HOME/.kube/kind-config-$1
}

deploy_service_ca_operator() {
    kubectl create ns openshift-config-managed
    kubectl apply -f service-ca/
}

deploy_crds() {
    kubectl apply -f req_crds/
}

deploy_templates() {
    kubectl apply -f templates/
}

deploy_openshift_router() {
    kubectl create ns openshift-ingress
    kubectl apply -f router/
}

setup_e2e_test() {
    ../../cicd-scripts/setup-e2e-tests.sh -a install
}

run_e2e_test() {
    ../../cicd-scripts/run-e2e-tests.sh
}

deploy() {
    setup_kubectl_command
    create_kind_cluster hub
    deploy_crds
    deploy_templates
    deploy_service_ca_operator
    deploy_openshift_router
    setup_e2e_test
    run_e2e_test
}

deploy
