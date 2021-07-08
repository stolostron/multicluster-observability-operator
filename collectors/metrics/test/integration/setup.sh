#!/bin/bash
# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

if [ "$#" -ne 1 ] ; then
  echo "Usage: $0 IMAGE" >&2
  exit 1
fi

echo "=====running kind exploration=====" 

IMAGE_NAME=$1
echo "IMAGE: " $IMAGE_NAME

DEFAULT_NS="open-cluster-management"
HUB_KUBECONFIG=$HOME/.kube/kind-config-hub
WORKDIR=`pwd`

sed_command='sed -i-e -e'
if [[ "$(uname)" == "Darwin" ]]; then
	sed_command='sed -i '-e' -e'
fi

deploy() {
    #setup_kubectl_command
	create_kind_hub
	deploy_prometheus_operator
	deploy_observatorium
	deploy_thanos
	deploy_metrics_collector $IMAGE_NAME	
}

setup_kubectl_command() {
	echo "=====Setup kubectl=====" 
	# kubectl required for kind
	echo "Install kubectl from openshift mirror (https://mirror.openshift.com/pub/openshift-v4/clients/ocp/4.4.14/openshift-client-mac-4.4.14.tar.gz)" 
	mv README.md README.md.tmp 
    if [[ "$(uname)" == "Darwin" ]]; then # then we are on a Mac 
		curl -LO https://mirror.openshift.com/pub/openshift-v4/clients/ocp/4.4.14/openshift-client-mac-4.4.14.tar.gz 
		tar xzvf openshift-client-mac-4.4.14.tar.gz  # xzf to quiet logs
		rm openshift-client-mac-4.4.14.tar.gz
    elif [[ "$(uname)" == "Linux" ]]; then # we are in travis, building in rhel 
		curl -LO https://mirror.openshift.com/pub/openshift-v4/clients/ocp/4.4.14/openshift-client-linux-4.4.14.tar.gz
		tar xzvf openshift-client-linux-4.4.14.tar.gz  # xzf to quiet logs
		rm openshift-client-linux-4.4.14.tar.gz
    fi
	# this package has a binary, so:

	echo "Current directory"
	echo $(pwd)
	mv README.md.tmp README.md 
	chmod +x ./kubectl
	if [[ ! -f /usr/local/bin/kubectl ]]; then
		sudo cp ./kubectl /usr/local/bin/kubectl
	fi
	# kubectl are now installed in current dir 
	echo -n "kubectl version" && kubectl version
}
 
create_kind_hub() { 
    WORKDIR=`pwd`
    echo "Delete hub if it exists"
    kind delete cluster --name hub || true
    
    echo "Start hub cluster" 
    rm -rf $HOME/.kube/kind-config-hub
    kind create cluster --kubeconfig $HOME/.kube/kind-config-hub --name hub --config ${WORKDIR}/test/integration/kind/kind-hub.config.yaml
    # kubectl cluster-info --context kind-hub --kubeconfig $(pwd)/.kube/kind-config-hub # confirm connection 
    export KUBECONFIG=$HOME/.kube/kind-config-hub
} 
deploy_observatorium() {
	echo "=====Setting up observatorium in kind cluster=====" 
	echo "Current directory"
	echo $(pwd)

	echo -n "Create namespace open-cluster-management-observability: " && kubectl create namespace open-cluster-management-observability
	echo "Apply observatorium yamls" 
	echo -n "Apply client ca cert and server certs: " && kubectl apply -f ./metrics-collector/test/integration/manifests/observatorium-ca-cert.yaml
	echo -n "Apply secret with tenant yaml : " && kubectl apply -f ./metrics-collector/test/integration/manifests/observatorium-api-secret.yaml
	echo -n "Apply configmap with rbac yaml : " && kubectl apply -f ./metrics-collector/test/integration/manifests/observatorium-api-configmap.yaml
	echo -n "Apply Deployment yaml : " && kubectl apply -f ./metrics-collector/test/integration/manifests/observatorium-api.yaml
	echo -n "Apply Service yaml : " && kubectl apply -f ./metrics-collector/test/integration/manifests/observatorium-api-service.yaml
}
deploy_thanos() {
	echo "=====Setting up thanos in kind cluster=====" 
	echo -n "Apply create pvc yaml : " && kubectl apply -f ./metrics-collector/test/integration/manifests/thanos-pvc.yaml
	echo -n "Apply configmap with hashring yaml : " && kubectl apply -f ./metrics-collector/test/integration/manifests/thanos-configmap.yaml
	echo -n "Apply Deployment yaml : " && kubectl apply -f ./metrics-collector/test/integration/manifests/thanos-api.yaml
	echo -n "Apply Service yaml : " && kubectl apply -f ./metrics-collector/test/integration/manifests/thanos-service.yaml
	echo "Waiting 2 minutes for observatorium and thanos to start... " && sleep 120
}

deploy_prometheus_operator() {
	echo "=====Setting up prometheus in kind cluster=====" 

    WORKDIR=`pwd`
    echo "Install prometheus operator." 
    echo "Current directory"
    echo $(pwd)
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
    
    echo "HUB_KUBECONFIG" ${HUB_KUBECONFIG}
    echo "KUBECONFIG" ${KUBECONFIG}
    
    echo "Creating prometheus manifests setup" && kubectl create -f kube-prometheus/manifests/setup
    until kubectl get servicemonitors --all-namespaces; do date; sleep 1; echo ""; done
    echo "Creating prometheus manifests" && kubectl create -f kube-prometheus/manifests/
    rm -rf kube-prometheus
    echo "Installed prometheus operator." 
    sleep 60
    echo -n "available services: " && kubectl get svc --all-namespaces
}

deploy_metrics_collector() {
	echo "=====Deploying metrics-collector====="
	echo -n "Switch to namespace: " && kubectl config set-context --current --namespace open-cluster-management-observability

	echo "Current directory"
	echo $(pwd)
	# git clone https://github.com/open-cluster-management/multicluster-observability-operator/collectors/metrics.git

	cd metrics-collector
    echo -n "Creating pull secret: " && kubectl create secret docker-registry multiclusterhub-operator-pull-secret --docker-server=quay.io --docker-username=$DOCKER_USER --docker-password=$DOCKER_PASS 
	
	# apply yamls 
	echo "Apply hub yamls" 
	echo -n "Apply client-serving-certs-ca-bundle: " && kubectl apply -f ./test/integration/manifests/client-serving-certs-ca-bundle.yaml
	echo -n "Apply rolebinding: " && kubectl apply -f ./test/integration/manifests/rolebinding.yaml
	echo -n "Apply client secret: " && kubectl apply -f ./test/integration/manifests/client_secret.yaml
	echo -n "Apply mtls certs: " && kubectl apply -f ./test/integration/manifests/metrics-collector-cert.yaml
	cp ./test/integration/manifests/deployment.yaml ./test/integration/manifests/deployment_update.yaml
	$sed_command "s~{{ METRICS_COLLECTOR_IMAGE }}~$1~g" ./test/integration/manifests/deployment_update.yaml
    $sed_command "s~cluster=func_e2e_test_travis~cluster=func_e2e_test_travis-$1~g" ./test/integration/manifests/deployment_update.yaml
	echo "Display deployment yaml" 
	cat ./test/integration/manifests/deployment_update.yaml
	echo -n "Apply metrics collector deployment: " && kubectl apply -f ./test/integration/manifests/deployment_update.yaml
	rm ./test/integration/manifests/deployment_update.yaml*

    echo -n "available pods: " && kubectl get pods --all-namespaces
	echo "Waiting 3 minutes for the pod to set up and send data... " && sleep 180
	POD=$(kubectl get pod -l k8s-app=metrics-collector -n open-cluster-management-observability -o jsonpath="{.items[0].metadata.name}")
	echo "Monitoring pod logs" 
	count=0
	
	while true ; do   
	  count=`expr $count + 1`
	  result=$(kubectl logs $POD | grep -i "Metrics pushed successfully" > /dev/null && echo "SUCCESS" || echo "FAILURE")
	  if [ $result == "SUCCESS"  ]
	  then
	     echo "SUCCESS sending metrics to Thanos"
		 exit 0
	  fi
	  echo "No Sucess yet ..Sleeping for 30s"
	  echo "available pods: " && kubectl describe pod $POD
	  sleep 30s
	  if [ $count -gt 10 ]
	  then
	     echo "FAILED sending metrics to Thanos"
		 exit 1
	  fi

	done 
	echo "available pods: " && kubectl get pods --all-namespaces

}


deploy 
