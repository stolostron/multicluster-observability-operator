#!/bin/bash
# Copyright (c) 2020 Red Hat, Inc.

set -e
set -o pipefail

WORKDIR=`pwd`
HUB_KUBECONFIG=$HOME/.kube/kind-config-hub
SPOKE_KUBECONFIG=$HOME/.kube/kind-config-spoke
MONITORING_NS="open-cluster-management-observability"
DEFAULT_NS="open-cluster-management"
AGENT_NS="open-cluster-management-agent"
HUB_NS="open-cluster-management-hub"

sed_command='sed -i-e -e'
if [[ "$(uname)" == "Darwin" ]]; then
    sed_command='sed -i '-e' -e'
fi

print_mco_operator_log() {
    kubectl --kubeconfig $HUB_KUBECONFIG -n $DEFAULT_NS get po \
        | grep multicluster-observability-operator | awk '{print $1}' \
        | xargs kubectl --kubeconfig $HUB_KUBECONFIG -n $DEFAULT_NS logs
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

deploy_mco_operator() {
    cd ${WORKDIR}
    if [[ ! -z "$1" ]]; then
        # replace the operator image with the latest image
        $sed_command "s~image:.*$~image: $1~g" deploy/operator.yaml
    fi
    # Add storage class config
    cp deploy/crds/observability.open-cluster-management.io_v1beta1_multiclusterobservability_cr.yaml deploy/crds/observability.open-cluster-management.io_v1beta1_multiclusterobservability_cr.yaml-e
    $sed_command "s~spec:.*$~spec: ~g" deploy/crds/observability.open-cluster-management.io_v1beta1_multiclusterobservability_cr.yaml
    printf "\n    statefulSetSize: 10Gi\n" >> deploy/crds/observability.open-cluster-management.io_v1beta1_multiclusterobservability_cr.yaml
    printf "\n    statefulSetStorageClass: standard\n" >> deploy/crds/observability.open-cluster-management.io_v1beta1_multiclusterobservability_cr.yaml
    printf "\n  availabilityConfig: Basic\n" >> deploy/crds/observability.open-cluster-management.io_v1beta1_multiclusterobservability_cr.yaml
    # Install the multicluster-observability-operator
    kubectl create ns ${MONITORING_NS}
    kubectl config set-context --current --namespace ${MONITORING_NS}
    # create image pull secret
    kubectl create secret docker-registry multiclusterhub-operator-pull-secret --docker-server=quay.io --docker-username=$DOCKER_USER --docker-password=$DOCKER_PASS
    kubectl apply -f deploy/req_crds
    kubectl apply -f deploy/crds/observability.open-cluster-management.io_multiclusterobservabilities_crd.yaml
    kubectl apply -f tests/e2e/req_crds
    $sed_command "s~storageClassName:.*$~storageClassName: standard~g" tests/e2e/minio/minio-pvc.yaml
    kubectl apply -f tests/e2e/minio
    sleep 2
    kubectl apply -f tests/e2e/req_crds/hub_cr
    kubectl create ns ${DEFAULT_NS}
    kubectl create secret -n ${DEFAULT_NS} docker-registry multiclusterhub-operator-pull-secret --docker-server=quay.io --docker-username=$DOCKER_USER --docker-password=$DOCKER_PASS
    kubectl apply -n ${DEFAULT_NS} -f deploy/operator.yaml
    kubectl apply -n ${DEFAULT_NS} -f deploy/role.yaml
    kubectl apply -n ${DEFAULT_NS} -f deploy/role_binding.yaml
    kubectl apply -n ${DEFAULT_NS} -f deploy/service_account.yaml
    kubectl apply -f deploy/crds/observability.open-cluster-management.io_v1beta1_multiclusterobservability_cr.yaml

    # expose grafana to test accessible
    kubectl apply -f tests/e2e/grafana/grafana-route.yaml
}

# deploy the new grafana to check the dashboards from browsers
deploy_grafana() {
    cd ${WORKDIR}
    $sed_command "s~name: grafana$~name: grafana-test~g; s~app: grafana$~app: grafana-test~g; s~secretName: grafana-config$~secretName: grafana-config-test~g; /MULTICLUSTEROBSERVABILITY_CR_NAME/d" manifests/base/grafana/deployment.yaml
    $sed_command "s~name: grafana$~name: grafana-test~g; s~app: grafana$~app: grafana-test~g" manifests/base/grafana/service.yaml
    $sed_command "s~namespace: open-cluster-management$~namespace: open-cluster-management-observability~g" manifests/base/grafana/deployment.yaml manifests/base/grafana/service.yaml

    kubectl apply -f manifests/base/grafana/deployment.yaml
    kubectl apply -f manifests/base/grafana/service.yaml
    kubectl apply -f tests/e2e/grafana
}

revert_changes() {
    cd ${WORKDIR}
    CHANGED_FILES="manifests/base/grafana/deployment.yaml manifests/base/grafana/service.yaml deploy/crds/observability.open-cluster-management.io_v1beta1_multiclusterobservability_cr.yaml deploy/operator.yaml"
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
    $sed_command "s~cpu: 100m~cpu: 10m~g" deploy/cluster-manager/*.yaml

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
    kubectl create ns $HUB_NS || true
    kubectl create quota test --hard=pods=4 -n $HUB_NS
    kubectl apply -f deploy/cluster-manager/crds

    # waiting cluster-manager ready and modify resource request and replicas due to resource insufficient
    n=1
    while true
    do
        if kubectl --kubeconfig $HUB_KUBECONFIG -n $HUB_NS get deploy cluster-manager-registration-controller cluster-manager-registration-webhook  cluster-manager-work-webhook; then
            kubectl  --kubeconfig $HUB_KUBECONFIG delete deploy -n  $DEFAULT_NS cluster-manager

            kubectl  --kubeconfig $HUB_KUBECONFIG scale --replicas=1 deployment cluster-manager-work-webhook -n $HUB_NS
            kubectl  --kubeconfig $HUB_KUBECONFIG scale --replicas=1 deployment cluster-manager-registration-controller -n $HUB_NS
            kubectl  --kubeconfig $HUB_KUBECONFIG scale --replicas=1 deployment cluster-manager-registration-webhook -n $HUB_NS

            kubectl  --kubeconfig $HUB_KUBECONFIG -n $HUB_NS patch deployment cluster-manager-registration-controller --type='json' -p='[{"op": "replace", "path": "/spec/template/spec/containers/0/resources", "value": {}}]'
            kubectl  --kubeconfig $HUB_KUBECONFIG -n $HUB_NS patch deployment cluster-manager-registration-webhook --type='json' -p='[{"op": "replace", "path": "/spec/template/spec/containers/0/resources", "value": {}}]'
            kubectl  --kubeconfig $HUB_KUBECONFIG -n $HUB_NS patch deployment cluster-manager-work-webhook --type='json' -p='[{"op": "replace", "path": "/spec/template/spec/containers/0/resources", "value": {}}]'

            break
        fi

        if [[ $n -ge 20 ]]; then
            echo "Waiting for cluster-manager ready timeout ..."
            exit 1
        fi

        n=$((n+1))
        echo "Retrying in 10s for waiting for cluster-manager ready ..."
        sleep 10
    done
}

deploy_spoke_core() {
    cd ${WORKDIR}/../registration-operator
    $sed_command "s~replicas: 3~replicas: 1~g" deploy/klusterlet/*.yaml
    $sed_command "s~cpu: 100m~cpu: 10m~g" deploy/klusterlet/*.yaml

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
    kubectl create quota test --hard=pods=4 -n $AGENT_NS
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
            print_mco_operator_log
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
            print_mco_operator_log
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
    echo "Patch observability placementrule"
    cat ~/.kube/kind-config-hub|grep certificate-authority-data|awk '{split($0, a, ": "); print a[2]}'|base64 -d  >> ca
    cat ~/.kube/kind-config-hub|grep client-certificate-data|awk '{split($0, a, ": "); print a[2]}'|base64 -d >> crt
    cat ~/.kube/kind-config-hub|grep client-key-data|awk '{split($0, a, ": "); print a[2]}'|base64 -d >> key
    SERVER=$(cat ~/.kube/kind-config-hub|grep server|awk '{split($0, a, ": "); print a[2]}')
    curl --cert ./crt --key ./key --cacert ./ca -X PATCH -H "Content-Type:application/merge-patch+json" \
        $SERVER/apis/apps.open-cluster-management.io/v1/namespaces/$MONITORING_NS/placementrules/observability/status \
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
            print_mco_operator_log
            exit 1
        fi
        n=$((n+1))
        echo "Retrying in 10s..."
        sleep 10
    done
    kubectl --kubeconfig $HUB_KUBECONFIG -n $MONITORING_NS patch route observatorium-api --patch '{"spec":{"host": "observatorium.hub", "wildcardPolicy": "None"}}' --type=merge
    obser_hub=`kind get kubeconfig --name hub --internal | grep server: | awk -F '://' '{print $2}' | awk -F ':' '{print $1}'`

    # add hostAlias to the pod prometheus-k8s-0
    kubectl --kubeconfig $SPOKE_KUBECONFIG delete deploy prometheus-operator -n openshift-monitoring
    kubectl --kubeconfig $SPOKE_KUBECONFIG patch statefulset prometheus-k8s -n openshift-monitoring --patch "{\"spec\":{\"template\":{\"spec\":{\"hostAliases\":[{\"hostnames\":[\"observatorium.hub\"], \"ip\": \"$obser_hub\"}]}}}}" --type=merge

    #docker exec --env obser_hub=$obser_hub -it $spoke_docker_id /bin/bash -c 'echo "$obser_hub observatorium.hub" >> /etc/hosts'

}

patch_for_memcached() {
    n=1
    while true
    do
        if kubectl --kubeconfig $HUB_KUBECONFIG -n $MONITORING_NS get statefulset | grep observability-observatorium-thanos-store-memcached; then
            break
        fi
        if [[ $n -ge 30 ]]; then
            # for debug pod status
            kubectl --kubeconfig $HUB_KUBECONFIG -n $MONITORING_NS get po
            kubectl --kubeconfig $HUB_KUBECONFIG -n $MONITORING_NS describe po
            print_mco_operator_log
            exit 1
        fi
        n=$((n+1))
        echo "Retrying in 10s waiting for observability-observatorium-thanos-store-memcached ..."
        sleep 10
    done
    # remove observability-observatorium-thanos-store-memcached resource request due to resource insufficient
    kubectl --kubeconfig $HUB_KUBECONFIG -n $MONITORING_NS patch statefulset observability-observatorium-thanos-store-memcached --type='json' -p='[{"op": "replace", "path": "/spec/template/spec/containers/0/resources", "value": {}}]'
    kubectl --kubeconfig $HUB_KUBECONFIG -n $MONITORING_NS delete pod observability-observatorium-thanos-store-memcached-0
}

patch_for_clusterrole()  {
    kubectl apply --kubeconfig $SPOKE_KUBECONFIG -f ./tests/e2e/templates/clusterrole.yaml
    if [ $? -ne 0 ]; then
        echo "Failed to create cluster-monitoring-view clusterrole"
        exit 1
    else
        echo "Created cluster-monitoring-view clusterrole"
    fi
}

deploy_cert_manager() {
    curl -L https://github.com/jetstack/cert-manager/releases/download/v0.10.0/cert-manager-openshift.yaml -o cert-manager-openshift.yaml
    echo "Replace namespace with ibm-common-services"
    $sed_command "s~--cluster-resource-namespace=.*~--cluster-resource-namespace=ibm-common-services~g" cert-manager-openshift.yaml
    if kubectl apply -f cert-manager-openshift.yaml ; then
        echo "cert-manager was successfully deployed"
    else
        echo "Failed to deploy cert-manager"
        exit 1
    fi
    rm cert-manager-openshift.yaml
}

deploy() {
    setup_kubectl_command
    create_kind_cluster hub
    deploy_prometheus_operator
    deploy_openshift_router
    deploy_cert_manager
    deploy_mco_operator $1
    if [[ "$2" == "grafana" ]]; then
        deploy_grafana
    fi
    deploy_hub_core
    create_kind_cluster spoke
    deploy_prometheus_operator
    deploy_spoke_core
    approve_csr_joinrequest
    patch_for_remote_write
    patch_placement_rule
    patch_for_memcached
    patch_for_clusterrole
    revert_changes
}

deploy $1 $2