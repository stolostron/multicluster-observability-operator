#!/bin/bash
# Copyright (c) 2020 Red Hat, Inc.

set -e
set -o pipefail

WORKDIR=`pwd`
HUB_KUBECONFIG=$HOME/.kube/kind-config-hub
SPOKE_KUBECONFIG=$HOME/.kube/kind-config-spoke
MONITORING_NS="open-cluster-management-monitoring"
DEFAULT_NS="open-cluster-management"
AGENT_NS="open-cluster-management-agent"
HUB_NS="open-cluster-management-hub"

sed_command='sed -i-e -e'
if [[ "$(uname)" == "Darwin" ]]; then
    sed_command='sed -i '-e' -e'
fi

# update prometheus CR to enable remote write to thanos
update_prometheus_remote_write() {
    obs_url="http://monitoring-observatorium-observatorium-api.$DEFAULT_NS.svc:8080/api/metrics/v1/write"
    cluster_replacement="hub_cluster"
    if [[ ! -z "$1" ]]; then
       obs_url="http://$1/api/metrics/v1/write"
       cluster_replacement="cluster1"
    fi
    if [[ "$(uname)" == "Darwin" ]]; then
        $sed_command "\$a\\
        \ \ remoteWrite:\\
        \ \ - url: ${obs_url}\\
        \ \ \ \ remoteTimeout: 30s\\
        \ \ \ \ writeRelabelConfigs:\\
        \ \ \ \ - sourceLabels:\\
        \ \ \ \ \ \ - __name__\\
        \ \ \ \ \ \ targetLabel: cluster\\
        \ \ \ \ \ \ replacement: $cluster_replacement" $WORKDIR/../kube-prometheus/manifests/prometheus-prometheus.yaml
    elif [[ "$(uname)" == "Linux" ]]; then
        $sed_command "\$a\ \ remoteWrite:\n\ \ - url: ${obs_url}\n\ \ \ \ remoteTimeout: 30s\n\ \ \ \ writeRelabelConfigs:\n\ \ \ \ - sourceLabels:\n\ \ \ \ \ \ - __name__\n\ \ \ \ \ \ targetLabel: cluster\n\ \ \ \ \ \ replacement: $cluster_replacement" $WORKDIR/../kube-prometheus/manifests/prometheus-prometheus.yaml
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

    echo "Start KinD cluster with the default cluster name - $1"
    rm -rf $HOME/.kube/kind-config-$1
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
    if [[ ! -z "$1" ]]; then
        update_prometheus_remote_write $1
    else
        update_prometheus_remote_write
    fi

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
    printf "\n  storageClass: local\n" >> deploy/crds/monitoring.open-cluster-management.io_v1alpha1_multiclustermonitoring_cr.yaml
    # Install the multicluster-monitoring-operator
    kubectl create ns ${MONITORING_NS}
    kubectl config set-context --current --namespace ${MONITORING_NS}
    # create image pull secret
    kubectl create secret docker-registry multiclusterhub-operator-pull-secret --docker-server=quay.io --docker-username=$DOCKER_USER --docker-password=$DOCKER_PASS

    # for mac, there is no /mnt
    if [[ "$(uname)" == "Darwin" ]]; then
        $sed_command "s~/mnt/thanos/teamcitydata1~/opt/thanos/teamcitydata1~g" tests/e2e/samples/persistentVolume.yaml
    fi

    kubectl apply -f tests/e2e/samples
    kubectl apply -f deploy/req_crds
    kubectl apply -f deploy/crds/monitoring.open-cluster-management.io_multiclustermonitorings_crd.yaml
    kubectl apply -f tests/e2e/req_crds
    sleep 2
    kubectl apply -f tests/e2e/req_crds/hub_cr
    kubectl apply -f deploy
    kubectl apply -f deploy/crds/monitoring.open-cluster-management.io_v1alpha1_multiclustermonitoring_cr.yaml
    
    # expose grafana to test accessible
    kubectl apply -f tests/e2e/grafana/grafana-route.yaml
}

# deploy the new grafana to check the dashboards from browsers
deploy_grafana() {
    cd ${WORKDIR}
    $sed_command "s~name: grafana$~name: grafana-test~g; s~app: grafana$~app: grafana-test~g; s~secretName: grafana-config$~secretName: grafana-config-test~g; /MULTICLUSTERMONITORING_CR_NAME/d" manifests/base/grafana/deployment.yaml
    $sed_command "s~name: grafana$~name: grafana-test~g; s~app: grafana$~app: grafana-test~g" manifests/base/grafana/service.yaml
    $sed_command "s~namespace: open-cluster-management$~namespace: open-cluster-management-monitoring~g" manifests/base/grafana/deployment.yaml manifests/base/grafana/service.yaml

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
    git clone https://github.com/open-cluster-management/registration-operator.git
    cd registration-operator/
    $sed_command "s~replicas: 3~replicas: 1~g" deploy/cluster-manager/*.yaml

    if [[ "$(uname)" == "Darwin" ]]; then
        $sed_command "\$a\\
        imagePullSecrets:\\
        - name: multiclusterhub-operator-pull-secret" deploy/cluster-manager/service_account.yaml
    elif [[ "$(uname)" == "Linux" ]]; then
        $sed_command "\$aimagePullSecrets:\n- name: multiclusterhub-operator-pull-secret" deploy/cluster-manager/service_account.yaml
    fi
    kubectl create ns $DEFAULT_NS || true
    kubectl config set-context --current --namespace $DEFAULT_NS
    kubectl apply -f deploy/cluster-manager/
    kubectl apply -f deploy/cluster-manager/crds/*crd.yaml
    sleep 2
    kubectl apply -f deploy/cluster-manager/crds
}

deploy_spoke_core() {
    cd ${WORKDIR}/../registration-operator
    $sed_command "s~replicas: 3~replicas: 1~g" deploy/klusterlet/*.yaml
    kubectl create ns ${DEFAULT_NS}
    kubectl config set-context --current --namespace ${DEFAULT_NS}
    kubectl create secret docker-registry multiclusterhub-operator-pull-secret --docker-server=quay.io --docker-username=$DOCKER_USER --docker-password=$DOCKER_PASS
    
    kubectl create namespace $MONITORING_NS
    kubectl create secret docker-registry multiclusterhub-operator-pull-secret --docker-server=quay.io --docker-username=$DOCKER_USER --docker-password=$DOCKER_PASS -n $MONITORING_NS
    
    if [[ "$(uname)" == "Darwin" ]]; then
        $sed_command "\$a\\
        imagePullSecrets:\\
        - name: multiclusterhub-operator-pull-secret" deploy/klusterlet/service_account.yaml
    elif [[ "$(uname)" == "Linux" ]]; then
        $sed_command "\$aimagePullSecrets:\n- name: multiclusterhub-operator-pull-secret" deploy/klusterlet/service_account.yaml
    fi
    kubectl apply -f deploy/klusterlet/
    kubectl apply -f deploy/klusterlet/crds/*crd.yaml
    sleep 2
    kubectl apply -f deploy/klusterlet/crds
    kubectl apply -f ${WORKDIR}/tests/e2e/req_crds
    sleep 2
    kubectl apply -f ${WORKDIR}/tests/e2e/req_crds/spoke_cr
    rm -rf ${WORKDIR}/../registration-operator
    kind get kubeconfig --name hub --internal > $HOME/.kube/kind-config-hub-internal
    kubectl create namespace $AGENT_NS
    kubectl create secret generic bootstrap-hub-kubeconfig --from-file=kubeconfig=$HOME/.kube/kind-config-hub-internal -n $AGENT_NS
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
        echo "Retrying in 10s..."
        sleep 10
    done
    n=1
    while true
    do
        cluster=`kubectl --kubeconfig $HUB_KUBECONFIG get managedcluster`
        if [[ ! -z $cluster ]]; then
            clustername=`kubectl --kubeconfig $HUB_KUBECONFIG get managedcluster | grep -v Name | awk 'NR==2' | awk '{ print $1 }'`
            echo "Approve joinrequest for $clustername"
            kubectl --kubeconfig $HUB_KUBECONFIG patch managedcluster $clustername --patch '{"spec":{"hubAcceptsClient":true}}' --type=merge
            break
        fi
        if [[ $n -ge 20 ]]; then
            exit 1
        fi
        n=$((n+1))
        echo "Retrying in 5s..."
        sleep 5
    done

}

patch_placement_rule() {
    cd ${WORKDIR}
    # Workaround for placementrules operator
    echo "Patch open-cluster-management-monitoring placementrule"
    cat ~/.kube/kind-config-hub|grep certificate-authority-data|awk '{split($0, a, ": "); print a[2]}'|base64 -d  >> ca
    cat ~/.kube/kind-config-hub|grep client-certificate-data|awk '{split($0, a, ": "); print a[2]}'|base64 -d >> crt
    cat ~/.kube/kind-config-hub|grep client-key-data|awk '{split($0, a, ": "); print a[2]}'|base64 -d >> key
    SERVER=$(cat ~/.kube/kind-config-hub|grep server|awk '{split($0, a, ": "); print a[2]}')
    curl --cert ./crt --key ./key --cacert ./ca -X PATCH -H "Content-Type:application/merge-patch+json" \
        $SERVER/apis/apps.open-cluster-management.io/v1/namespaces/$MONITORING_NS/placementrules/open-cluster-management-monitoring/status \
        -d @./tests/e2e/templates/status.json   
    rm ca crt key

}

patch_for_remote_write() {
    # patch observatorium route
    n=1
    while true
    do
        entity=$(kubectl --kubeconfig $HUB_KUBECONFIG -n $MONITORING_NS get route | grep observatorium-api) || true
        if [[ ! -z $entity ]]; then
            break
        fi
        if [[ $n -ge 20 ]]; then
            exit 1
        fi
        n=$((n+1))
        echo "Retrying in 10s..."
        sleep 10
    done
    kubectl --kubeconfig $HUB_KUBECONFIG -n $MONITORING_NS patch route observatorium-api --patch '{"spec":{"host": "observatorium.hub", "wildcardPolicy": "None"}}' --type=merge
    #obser_hub=`kind get kubeconfig --name hub --internal | grep server: | awk -F '://' '{print $2}' | awk -F ':' '{print $1}'`

    #spoke_docker_id=`docker ps | grep spoke-control-plane | awk -F ' ' '{print $1}'`
    #docker exec --env obser_hub=$obser_hub -it $spoke_docker_id /bin/bash -c 'echo "$obser_hub observatorium.hub" >> /etc/hosts'

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
    deploy_prometheus_operator observatorium.hub
    deploy_spoke_core
    approve_csr_joinrequest
    patch_for_remote_write
    patch_placement_rule
    revert_changes
}

deploy $1 $2