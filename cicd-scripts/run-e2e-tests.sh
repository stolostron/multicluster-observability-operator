#!/bin/bash
# Copyright (c) 2021 Red Hat, Inc.

echo "<repo>/<component>:<tag> : $1"

git config --global url."https://$GITHUB_TOKEN@github.com/stolostron".insteadOf  "https://github.com/stolostron"

WORKDIR=`pwd`
cd ${WORKDIR}/..
git clone --depth 1 -b release-2.2 https://github.com/stolostron/observability-kind-cluster.git
cd observability-kind-cluster
./setup.sh $1
if [ $? -ne 0 ]; then
    echo "Cannot setup environment successfully."
    exit 1
fi

cd ${WORKDIR}/..
git clone --depth 1 -b release-2.2 https://github.com/stolostron/observability-e2e-test.git
cd observability-e2e-test

HUB_KUBECONFIG=$HOME/.kube/kind-config-hub
SPOKE_KUBECONFIG=$HOME/.kube/kind-config-spoke

kubectl get manifestworks endpoint-observability-work -n cluster1 --kubeconfig $HUB_KUBECONFIG -oyaml

kubectl logs --kubeconfig $HUB_KUBECONFIG -n open-cluster-management $(kubectl get po --kubeconfig $HUB_KUBECONFIG -n open-cluster-management|grep observability|awk '{split($0, a, " "); print a[1]}')

sleep 120

kubectl logs --kubeconfig $SPOKE_KUBECONFIG -n open-cluster-management-addon-observability $(kubectl get po --kubeconfig $SPOKE_KUBECONFIG -n open-cluster-management-addon-observability|grep endpoint|awk '{split($0, a, " "); print a[1]}')
kubectl logs --kubeconfig $SPOKE_KUBECONFIG -n open-cluster-management-addon-observability $(kubectl get po --kubeconfig $SPOKE_KUBECONFIG -n open-cluster-management-addon-observability|grep collector|awk '{split($0, a, " "); print a[1]}')

kubectl get pods --kubeconfig $SPOKE_KUBECONFIG -n open-cluster-management-agent

kubectl logs --kubeconfig $SPOKE_KUBECONFIG -n open-cluster-management-agent $(kubectl get po --kubeconfig $SPOKE_KUBECONFIG -n open-cluster-management-agent|grep work|sed -n 1p|awk '{split($0, a, " "); print a[1]}')
kubectl logs --kubeconfig $SPOKE_KUBECONFIG -n open-cluster-management-agent $(kubectl get po --kubeconfig $SPOKE_KUBECONFIG -n open-cluster-management-agent|grep work|sed -n 2p|awk '{split($0, a, " "); print a[1]}')
kubectl logs --kubeconfig $SPOKE_KUBECONFIG -n open-cluster-management-agent $(kubectl get po --kubeconfig $SPOKE_KUBECONFIG -n open-cluster-management-agent|grep work|sed -n 3p|awk '{split($0, a, " "); print a[1]}')

kubectl get deployment --kubeconfig $SPOKE_KUBECONFIG -n open-cluster-management-addon-observability -oyaml $(kubectl get po --kubeconfig $SPOKE_KUBECONFIG -n open-cluster-management-addon-observability|grep endpoint|awk '{split($0, a, " "); print a[1]}')

# run test cases
./cicd-scripts/tests.sh
if [ $? -ne 0 ]; then
    echo "Cannot pass all test cases."
    cat ./pkg/tests/results.xml
    exit 1
fi
