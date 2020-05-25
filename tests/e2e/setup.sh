#!/bin/bash
# Copyright (c) 2020 Red Hat, Inc.

set -e
set -o pipefail

WORKDIR=`pwd`
HUB_KUBECONFIG=$HOME/.kube/kind-config-hub
SPOKE_KUBECONFIG=$HOME/.kube/kind-config-spoke

sed_command='sed -i-e -e'
if [[ "$(uname)" == "Darwin" ]]; then
    sed_command='sed -i '-e' -e'
fi

# update prometheus CR to enable remote write to thanos
update_prometheus_remote_write() {
    if [[ "$(uname)" == "Darwin" ]]; then
        $sed_command "\$a\\
        \ \ remoteWrite:\\
        \ \ - url: http://monitoring-observatorium-observatorium-api.open-cluster-management.svc:8080/api/metrics/v1/write\\
        \ \ \ \ remoteTimeout: 30s\\
        \ \ \ \ writeRelabelConfigs:\\
        \ \ \ \ - sourceLabels:\\
        \ \ \ \ \ \ - __name__\\
        \ \ \ \ \ \ targetLabel: cluster\\
        \ \ \ \ \ \ replacement: hub_cluster" kube-prometheus/manifests/prometheus-prometheus.yaml
    elif [[ "$(uname)" == "Linux" ]]; then
        $sed_command "\$a\ \ remoteWrite:\n\ \ - url: http://monitoring-observatorium-observatorium-api.open-cluster-management.svc:8080/api/metrics/v1/write\n\ \ \ \ remoteTimeout: 30s\n\ \ \ \ writeRelabelConfigs:\n\ \ \ \ - sourceLabels:\n\ \ \ \ \ \ - __name__\n\ \ \ \ \ \ targetLabel: cluster\n\ \ \ \ \ \ replacement: hub_cluster" kube-prometheus/manifests/prometheus-prometheus.yaml
    fi
}

create_kind_cluster() {
    if [[ ! -f /usr/local/bin/kind ]]; then
        echo "This script will install kind (https://kind.sigs.k8s.io/) on your machine."
        curl -Lo ./kind "https://kind.sigs.k8s.io/dl/v0.7.0/kind-$(uname)-amd64"
        chmod +x ./kind
        sudo mv ./kind /usr/local/bin/kind
    fi
    echo "Delete the KinD cluster if exists"
    kind delete cluster --name $1 || true

    echo "Start KinD cluster with the default cluster name - kind"
    kind create cluster --kubeconfig $HOME/.kube/kind-config-$1 --name $1 --config ${WORKDIR}/tests/e2e/kind/kind-$1.config.yaml
    export KUBECONFIG=$HOME/.kube/kind-config-$1

}

setup_kubectl_command() {
    if [[ ! -f /usr/local/bin/kubectl ]]; then
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

install_jq() {
    if [[ ! -f /usr/local/bin/jq ]]; then
        if [[ "$(uname)" == "Linux" ]]; then
            curl -o jq -LO https://github.com/stedolan/jq/releases/download/jq-1.6/jq-linux64
        elif [[ "$(uname)" == "Darwin" ]]; then
            curl -o jq -LO https://github.com/stedolan/jq/releases/download/jq-1.6/jq-osx-amd64
        fi
        chmod +x ./jq
        sudo mv ./jq /usr/local/bin/jq
    fi
}

deploy_prometheus_operator() {
    echo "Install prometheus operator. Observatorium requires it."
    cd ${WORKDIR}/..
    git clone https://github.com/coreos/kube-prometheus.git

    echo "Replace namespace with openshift-monitoring"
    $sed_command "s~namespace: monitoring~namespace: openshift-monitoring~g" kube-prometheus/manifests/*.yaml
    $sed_command "s~namespace: monitoring~namespace: openshift-monitoring~g" kube-prometheus/manifests/setup/*.yaml
    $sed_command "s~name: monitoring~name: openshift-monitoring~g" kube-prometheus/manifests/setup/*.yaml
    $sed_command "s~replicas:.*$~replicas: 1~g" kube-prometheus/manifests/prometheus-prometheus.yaml
    echo "Remove alertmanager and grafana to free up resource"
    rm -rf kube-prometheus/manifests/alertmanager-*.yaml
    rm -rf kube-prometheus/manifests/grafana-*.yaml

    update_prometheus_remote_write

    kubectl create -f kube-prometheus/manifests/setup
    until kubectl get servicemonitors --all-namespaces ; do date; sleep 1; echo ""; done
    kubectl create -f kube-prometheus/manifests/
    rm -rf kube-prometheus
}

deploy_openshift_router() {
    cd ${WORKDIR}/..
    echo "Install openshift route"
    git clone https://github.com/openshift/router.git
    kubectl apply -f router/deploy/route_crd.yaml
    kubectl apply -f router/deploy/router_rbac.yaml
    kubectl apply -f router/deploy/router.yaml
    rm -rf router
}

deploy_mcm_operator() {
    cd ${WORKDIR}
    if [[ ! -z "$1" ]]; then
        # replace the operator image with the latest image
        $sed_command "s~image:.*$~image: $1~g" deploy/operator.yaml
    fi
    # Add storage class config
    cp deploy/crds/monitoring.open-cluster-management.io_v1alpha1_multiclustermonitoring_cr.yaml deploy/crds/monitoring.open-cluster-management.io_v1alpha1_multiclustermonitoring_cr.yaml-e
    printf "spec:\n  storageClass: local\n" >> deploy/crds/monitoring.open-cluster-management.io_v1alpha1_multiclustermonitoring_cr.yaml
    # Install the multicluster-monitoring-operator
    kubectl create ns open-cluster-management
    kubectl config set-context --current --namespace open-cluster-management
    # create image pull secret
    kubectl create secret docker-registry multiclusterhub-operator-pull-secret --docker-server=quay.io --docker-username=$DOCKER_USER --docker-password=$DOCKER_PASS

    # for mac, there is no /mnt
    if [[ "$(uname)" == "Darwin" ]]; then
        $sed_command "s~/mnt/thanos/teamcitydata1~/opt/thanos/teamcitydata1~g" tests/e2e/samples/persistentVolume.yaml
    fi

    kubectl apply -f tests/e2e/samples
    kubectl apply -f deploy/req_crds
    kubectl apply -f deploy/crds/monitoring.open-cluster-management.io_multiclustermonitorings_crd.yaml
    kubectl apply -f tests/e2e/req_crds/apps.open-cluster-management.io_placementrules_crd.yaml
    kubectl apply -f deploy
    kubectl apply -f deploy/crds/monitoring.open-cluster-management.io_v1alpha1_multiclustermonitoring_cr.yaml
    
    # expose grafana to test accessible
    kubectl apply -f tests/e2e/grafana/grafana-test-route.yaml
}

# deploy the new grafana to check the dashboards from browsers
deploy_grafana() {
    cd ${WORKDIR}
    $sed_command "s~name: grafana$~name: grafana-test~g; s~app: grafana$~app: grafana-test~g; s~secretName: grafana-config$~secretName: grafana-config-test~g; /MULTICLUSTERMONITORING_CR_NAME/d" manifests/base/grafana/deployment.yaml
    $sed_command "s~name: grafana$~name: grafana-test~g; s~app: grafana$~app: grafana-test~g" manifests/base/grafana/service.yaml

    kubectl apply -f manifests/base/grafana/deployment.yaml
    kubectl apply -f manifests/base/grafana/service.yaml
    kubectl apply -f tests/e2e/grafana
}

revert_changes() {
    cd ${WORKDIR}
    CHANGED_FILES="manifests/base/grafana/deployment.yaml manifests/base/grafana/service.yaml tests/e2e/samples/persistentVolume.yaml deploy/crds/monitoring.open-cluster-management.io_v1alpha1_multiclustermonitoring_cr.yaml deploy/operator.yaml"
    # revert the changes
    for file in ${CHANGED_FILES}; do
        if [[ -f "${file}-e" ]]; then
            mv -f "${file}-e" "${file}"
        fi
    done
}

deploy_hub_core() {
    cd ${WORKDIR}/..
    git clone https://github.com/open-cluster-management/nucleus.git
    cd nucleus/
    $sed_command "s~namespace: open-cluster-management-core~namespace: open-cluster-management~g" deploy/nucleus-hub/*.yaml
    $sed_command "s~replicas: 3~replicas: 1~g" deploy/nucleus-hub/*.yaml
if [[ "$(uname)" == "Darwin" ]]; then
    $sed_command "\$a\\
    imagePullSecrets:\\
    - name: multiclusterhub-operator-pull-secret" deploy/nucleus-hub/service_account.yaml
elif [[ "$(uname)" == "Linux" ]]; then
    $sed_command "\$aimagePullSecrets:\n- name: multiclusterhub-operator-pull-secret" deploy/nucleus-hub/service_account.yaml
fi
    kubectl apply -f deploy/nucleus-hub/
    kubectl apply -f deploy/nucleus-hub/crds/*crd.yaml
    kubectl apply -f deploy/nucleus-hub/crds
    kubectl apply -f ${WORKDIR}/tests/e2e/nucleus/hubcore.yaml
}

deploy_spoke_core() {
    cd ${WORKDIR}/../nucleus
    $sed_command "s~namespace: open-cluster-management-core~namespace: default~g" deploy/nucleus-spoke/*.yaml
    $sed_command "s~namespace: open-cluster-management~namespace: default~g" deploy/nucleus-spoke/*.yaml
    $sed_command "s~replicas: 3~replicas: 1~g" deploy/nucleus-hub/*.yaml
kubectl create secret docker-registry multiclusterhub-operator-pull-secret --docker-server=quay.io --docker-username=$DOCKER_USER --docker-password=$DOCKER_PASS
if [[ "$(uname)" == "Darwin" ]]; then
    $sed_command "\$a\\
    imagePullSecrets:\\
    - name: multiclusterhub-operator-pull-secret" deploy/nucleus-spoke/service_account.yaml
elif [[ "$(uname)" == "Linux" ]]; then
    $sed_command "\$aimagePullSecrets:\n- name: multiclusterhub-operator-pull-secret" deploy/nucleus-spoke/service_account.yaml
fi
    kubectl apply -f deploy/nucleus-spoke/
    kubectl apply -f deploy/nucleus-spoke/crds/*crd.yaml
    kubectl apply -f deploy/nucleus-spoke/crds
    kubectl apply -f ${WORKDIR}/tests/e2e/nucleus/spokecore.yaml
    rm -rf ${WORKDIR}/../nucleus
    kind get kubeconfig --name hub --internal > $HOME/.kube/kind-config-hub
    kubectl create secret generic bootstrap-hub-kubeconfig --from-file=kubeconfig=$HOME/.kube/kind-config-hub
}

approve_csr_joinrequest() {
    n=1
    while true
    do
        csr=`kubectl --kubeconfig $HUB_KUBECONFIG get csr -lopen-cluster-management.io/cluster-name=cluster1 `
        if [[ ! -z $csr ]]; then
            csrname=`kubectl --kubeconfig $HUB_KUBECONFIG get csr -lopen-cluster-management.io/cluster-name=cluster1 | grep -v Name | awk 'NR==2' | awk '{ print $1 }' `
            echo "Approve CSR: $csrname"
            kubectl --kubeconfig $HUB_KUBECONFIG certificate approve $csrname
            break
        fi
        if [[ $n -ge 20 ]]; then
            exit 1
        fi
        n=$((n+1))
        echo "Retrying in 5s..."
        sleep 5
    done
    n=1
    while true
    do
        cluster=`kubectl --kubeconfig $HUB_KUBECONFIG get spokecluster`
        if [[ ! -z $cluster ]]; then
            clustername=`kubectl --kubeconfig $HUB_KUBECONFIG get spokecluster | grep -v Name | awk 'NR==2' | awk '{ print $1 }'`
            echo "Approve joinrequest for $clustername"
              kubectl --kubeconfig $HUB_KUBECONFIG patch spokecluster $clustername --patch '{"spec":{"hubAcceptsClient":true}}' --type=merge
            break
        fi
        if [[ $n -ge 20 ]]; then
            exit 1
        fi
        n=$((n+1))
        echo "Retrying in 5s..."
        sleep 5
    done

    # apply the temporary fix for RBAC
    kubectl --kubeconfig $HUB_KUBECONFIG apply -f $WORKDIR/tests/e2e/nucleus/role.yaml
    kubectl --kubeconfig $HUB_KUBECONFIG apply -f $WORKDIR/tests/e2e/nucleus/rolebinding.yaml
}

deploy() {
    setup_kubectl_command
    create_kind_cluster hub
    deploy_prometheus_operator
    deploy_openshift_router
    deploy_mcm_operator $1
    if [[ "$2" == "grafana" ]]; then
        deploy_grafana
    fi    
    deploy_hub_core
    create_kind_cluster spoke
    deploy_prometheus_operator
    deploy_spoke_core
    approve_csr_joinrequest
    revert_changes
}

deploy $1