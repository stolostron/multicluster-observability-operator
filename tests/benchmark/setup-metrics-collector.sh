#!/bin/bash
# Copyright (c) 2021 Red Hat, Inc.

export IMAGE=quay.io/stolostron/metrics-collector:2.2.0-SNAPSHOT-2021-01-17-18-45-18
export FROM=https://prometheus-k8s.openshift-monitoring.svc:9091
export TO=https://observatorium-api-open-cluster-management-observability.apps.cyang-ocp3.dev05.red-chesterfield.com/api/metrics/v1/default/api/v1/receive

sed_command='sed -i-e -e'
if [[ "$(uname)" == "Darwin" ]]; then
    sed_command='sed -i '-e' -e'
fi

if [ $# -ne 1 ]; then
	echo "this script must be run with the number of clusters:"
    echo -e "\n$0 total_clusters\n"
    exit 1
fi

re='^[0-9]+$'
if ! [[ $1 =~ $re ]] ; then
   echo "error: arguments <$1> not a number" >&2; exit 1
fi

for i in $(seq 1 $1)
do
	cluster_name=fake-cluster${i}
	kubectl create ns ${cluster_name}

	# create ca/sa/rolebinding for metrics collector
	kubectl apply -n ${cluster_name}  -f metrics-collector-serving-certs-ca-bundle.yaml
	kubectl get secret observability-managed-cluster-certs -n open-cluster-management-addon-observability --export -o yaml | kubectl apply -n ${cluster_name} -f -
	kubectl get sa endpoint-observability-operator-sa -n open-cluster-management-addon-observability --export -o yaml | kubectl apply -n ${cluster_name} -f -
	kubectl -n ${cluster_name} patch secret observability-managed-cluster-certs --type='json' -p='[{"op": "replace", "path": "/metadata/ownerReferences", "value": []}]'
	kubectl -n ${cluster_name} patch sa endpoint-observability-operator-sa --type='json' -p='[{"op": "replace", "path": "/metadata/ownerReferences", "value": []}]'

	# deploy metrics collector deployment to cluster ns
	deploy_yaml_file=${cluster_name}-metrics-collector-deployment.yaml
	cp -rf metrics-collector-deployment-template.yaml "$deploy_yaml_file"
	$sed_command "s~__CLUSTER_NAME__~${cluster_name}~g" "$deploy_yaml_file"
	$sed_command "s~__IMAGE__~${IMAGE}~g" "$deploy_yaml_file"
	$sed_command "s~__FROM__~${FROM}~g" "$deploy_yaml_file"
	$sed_command "s~__TO__~${TO}~g" "$deploy_yaml_file"
	rm -rf "$deploy_yaml_file"-e
	cat "$deploy_yaml_file" | kubectl -n ${cluster_name} apply -f -
	rm -rf "$deploy_yaml_file"

	# deploy ClusterRoleBinding for read metrics from OCP prometheus
	rolebinding_yaml_file=${cluster_name}-metrics-collector-view.yaml
	cp -rf metrics-collector-view.yaml "$rolebinding_yaml_file"
	$sed_command "s~__CLUSTER_NAME__~${cluster_name}~g" "$rolebinding_yaml_file"
	rm -rf "$rolebinding_yaml_file"-e
	cat "$rolebinding_yaml_file" | kubectl -n ${cluster_name} apply -f -
	rm -rf "$rolebinding_yaml_file"
        
        #deply image pull secret
        DOCKER_CONFIG_JSON=`oc extract secret/multiclusterhub-operator-pull-secret -n open-cluster-management-observability --to=-`
        oc create secret generic multiclusterhub-operator-pull-secret \
        -n ${cluster_name} \
        --from-literal=.dockerconfigjson="$DOCKER_CONFIG_JSON" \
        --type=kubernetes.io/dockerconfigjson
done
