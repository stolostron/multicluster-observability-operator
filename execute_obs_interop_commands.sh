#!/bin/bash

export PARAM_AWS_SECRET_ACCESS_KEY=${PARAM_AWS_SECRET_ACCESS_KEY:-}
export PARAM_AWS_ACCESS_KEY_ID=${PARAM_AWS_ACCESS_KEY_ID:-}
export CLOUD_PROVIDER=${CLOUD_PROVIDER:-}
export OC_CLUSTER_USER=${OC_CLUSTER_USER:-}
export OC_HUB_CLUSTER_PASS=${OC_HUB_CLUSTER_PASS:-}
export OC_HUB_CLUSTER_API_URL=${OC_HUB_CLUSTER_API_URL:-}
export HUB_CLUSTER_NAME=${HUB_CLUSTER_NAME:-}
export BASE_DOMAIN=${BASE_DOMAIN:-}
export MANAGED_CLUSTER_NAME=${MANAGED_CLUSTER_NAME:-}
export MANAGED_CLUSTER_BASE_DOMAIN=${MANAGED_CLUSTER_BASE_DOMAIN:-}
export MANAGED_CLUSTER_USER=${MANAGED_CLUSTER_USER:-}
export MANAGED_CLUSTER_PASS=${MANAGED_CLUSTER_PASS:-}
export MANAGED_CLUSTER_API_URL=${MANAGED_CLUSTER_API_URL}
export BUCKET=${BUCKET:-'obs-v1'}
export REGION=${REGION:-'us-east-1'}
export USE_MINIO=${USE_MINIO:-'false'}
export SKIP_INSTALL_STEP=${SKIP_INSTALL_STEP:-'false'}
export SKIP_UNINSTALL_STEP=${SKIP_UNINSTALL_STEP:-'true'}
export TAGGING=${TAGGING:-}

if [[ -n ${PARAM_AWS_ACCESS_KEY_ID} ]]; then
    export AWS_ACCESS_KEY_ID=${PARAM_AWS_ACCESS_KEY_ID}
fi

if [[ -n ${PARAM_AWS_SECRET_ACCESS_KEY} ]]; then
    export AWS_SECRET_ACCESS_KEY=${PARAM_AWS_SECRET_ACCESS_KEY}
fi

# if [[ ${!USE_MINIO} == "false" ]]; then
#     export IS_CANARY_ENV=true
# fi  

export IS_CANARY_ENV=true

if [[ -z ${HUB_CLUSTER_NAME} || -z ${BASE_DOMAIN} || -z ${OC_CLUSTER_USER} || -z ${OC_HUB_CLUSTER_PASS} || -z ${OC_HUB_CLUSTER_API_URL} ]]; then
    echo "Aborting test.. OCP HUB details are required for the test execution"
    exit 1
else
    if [[ -n ${MANAGED_CLUSTER_USER} && -n ${MANAGED_CLUSTER_PASS} && -n ${MANAGED_CLUSTER_API_URL} ]]; then
        oc login --insecure-skip-tls-verify -u $MANAGED_CLUSTER_USER -p $MANAGED_CLUSTER_PASS $MANAGED_CLUSTER_API_URL
        oc config view --minify --raw=true > ~/.kube/managed_kubeconfig
        export MAKUBECONFIG=~/.kube/managed_kubeconfig
    fi
    set +x
    oc login --insecure-skip-tls-verify -u $OC_CLUSTER_USER -p $OC_HUB_CLUSTER_PASS $OC_HUB_CLUSTER_API_URL
    set -x
 
    oc config view --minify --raw=true > userfile
    //cat userfile
    whoami
    rm -rf ~/.kube/config
    cp userfile ~/.kube/config
    //cat ~/.kube/config
    export KUBECONFIG=~/.kube/config

    go mod vendor && ginkgo build ./tests/pkg/tests/
    cd tests
    cp resources/options.yaml.template resources/options.yaml
    /usr/local/bin/yq e -i '.options.hub.name="'"$HUB_CLUSTER_NAME"'"' resources/options.yaml
    /usr/local/bin/yq e -i '.options.hub.baseDomain="'"$BASE_DOMAIN"'"' resources/options.yaml
    /usr/local/bin/yq e -i '.options.clusters.name="'"$MANAGED_CLUSTER_NAME"'"' resources/options.yaml
    /usr/local/bin/yq e -i '.options.clusters.baseDomain="'"$MANAGED_CLUSTER_BASE_DOMAIN"'"' resources/options.yaml
    /usr/local/bin/yq e -i '.options.clusters.kubeconfig="'"$MAKUBECONFIG"'"' resources/options.yaml
    cat resources/options.yaml
    ginkgo --focus=$TAGGING -v pkg/tests/ -- -options=../../resources/options.yaml -v=5
fi