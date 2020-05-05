#!/bin/bash

echo "This script will install kind (https://kind.sigs.k8s.io/) on your machine."

curl -Lo ./kind "https://kind.sigs.k8s.io/dl/v0.7.0/kind-$(uname)-amd64"
chmod +x ./kind
sudo mv ./kind /usr/local/bin/kind

echo "This script will install kubectl (https://kubernetes.io/docs/tasks/tools/install-kubectl/) on your machine"
curl -LO https://storage.googleapis.com/kubernetes-release/release/v1.18.0/bin/linux/amd64/kubectl
chmod +x ./kubectl
sudo mv ./kubectl /usr/local/bin/kubectl

echo "Delete the KinD cluster if exists"
kind delete cluster || true

echo "Start KinD cluster with the default cluster name - kind"
kind create cluster

echo "Install prometheus operator. Observatorium requires it."
cd ..
git clone https://github.com/coreos/kube-prometheus.git
cd kube-prometheus
kubectl create -f manifests/setup
until kubectl get servicemonitors --all-namespaces ; do date; sleep 1; echo ""; done
kubectl create -f manifests/

echo "Install openshift route"
cd ..
git clone https://github.com/openshift/router.git
cd router
kubectl create ns openshift-ingress
kubectl config set-context --current --namespace openshift-ingress
kubectl create -f deploy/

cd ../multicluster-monitoring-operator
# replace the operator image with the latest image
sed -i "s~image:.*$~image: $1~g" deploy/operator.yaml
sed -i "s/gp2/local/g" deploy/crds/monitoring.open-cluster-management.io_v1alpha1_multiclustermonitoring_cr.yaml
# Install the multicluster-monitoring-operator
kubectl create ns open-cluster-management
kubectl config set-context --current --namespace open-cluster-management
# TODO: create image pull secret
kubectl apply -f tests/e2e/samples
kubectl apply -f deploy/req_crds
kubectl apply -f deploy/crds/monitoring.open-cluster-management.io_multiclustermonitorings_crd.yaml
kubectl apply -f deploy

echo "sleep 10s to wait for CRD ready"
sleep 10

kubectl apply -f deploy/crds/monitoring.open-cluster-management.io_v1alpha1_multiclustermonitoring_cr.yaml

echo "create openshift-monitoring namespace if no existence"
kubectl create ns openshift-monitoring
