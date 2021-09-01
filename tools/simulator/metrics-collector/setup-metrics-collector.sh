#!/bin/bash
# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

METRICS_IMAGE="${METRICS_IMAGE:-quay.io/haoqing/metrics-data:latest}"
WORKDIR="$(pwd -P)"
export PATH=${PATH}:${WORKDIR}

if ! command -v jq &> /dev/null; then
	if [[ "$(uname)" == "Linux" ]]; then
		curl -o jq -L https://github.com/stedolan/jq/releases/download/jq-1.6/jq-linux64
	elif [[ "$(uname)" == "Darwin" ]]; then
		curl -o jq -L https://github.com/stedolan/jq/releases/download/jq-1.6/jq-osx-amd64
	fi
	chmod +x ./jq
fi

sed_command='sed -i'
if [[ "$(uname)" == "Darwin" ]]; then
    sed_command='sed -i -e'
fi

managed_cluster='managed'
if [ $# -eq 2 ]; then
	managed_cluster=$2
fi

if [ $# -lt 1 ]; then
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
	cluster_name=simulate-${managed_cluster}-cluster${i}
	kubectl create ns ${cluster_name}

	# create ca/sa/rolebinding for metrics collector
	kubectl get configmap metrics-collector-serving-certs-ca-bundle -n open-cluster-management-addon-observability -o json | jq 'del(.metadata.namespace,.metadata.resourceVersion,.metadata.uid) | .metadata.creationTimestamp=null' | kubectl apply -n ${cluster_name} -f -
	kubectl get secret observability-controller-open-cluster-management.io-observability-signer-client-cert -n open-cluster-management-addon-observability -o json | jq 'del(.metadata.namespace,.metadata.resourceVersion,.metadata.uid) | .metadata.creationTimestamp=null' | kubectl apply -n ${cluster_name} -f -
	kubectl get secret observability-managed-cluster-certs -n open-cluster-management-addon-observability -o json | jq 'del(.metadata.namespace,.metadata.resourceVersion,.metadata.uid) | .metadata.creationTimestamp=null' | kubectl apply -n ${cluster_name} -f -
	kubectl get sa endpoint-observability-operator-sa -n open-cluster-management-addon-observability -o json | jq 'del(.metadata.namespace,.metadata.resourceVersion,.metadata.uid) | .metadata.creationTimestamp=null' | kubectl apply -n ${cluster_name} -f -	
	kubectl -n ${cluster_name} patch secret observability-managed-cluster-certs --type='json' -p='[{"op": "replace", "path": "/metadata/ownerReferences", "value": []}]'
	kubectl -n ${cluster_name} patch sa endpoint-observability-operator-sa --type='json' -p='[{"op": "replace", "path": "/metadata/ownerReferences", "value": []}]'

	# deploy metrics collector deployment to cluster ns
	deploy_yaml_file=${cluster_name}-metrics-collector-deployment.json
	kubectl get deploy metrics-collector-deployment -n open-cluster-management-addon-observability -o json > $deploy_yaml_file

	# replace namespace, cluster and clusterID. Insert --simulated-timeseries-file
        uuid=$(cat /proc/sys/kernel/random/uuid)
        jq \
        --arg cluster_name $cluster_name \
        --arg cluster "--label=\"cluster=$cluster_name\"" \
        --arg clusterID "--label=\"clusterID=$uuid\"" \
        --arg file "--simulated-timeseries-file=/metrics-volume/timeseries.txt" \
        '.metadata.namespace=$cluster_name | .spec.template.spec.containers[0].command[.spec.template.spec.containers[0].command|length] |= . + $cluster |.spec.template.spec.containers[0].command[.spec.template.spec.containers[0].command|length] |= . + $clusterID | .spec.template.spec.containers[0].command[.spec.template.spec.containers[0].command|length] |= . + $file' $deploy_yaml_file > $deploy_yaml_file.tmp && mv $deploy_yaml_file.tmp $deploy_yaml_file

	# insert metrics initContainer
        jq \
        --argjson init '{"initContainers": [{"command":["sh","-c","cp /tmp/timeseries.txt /metrics-volume"],"image":"'$METRICS_IMAGE'","imagePullPolicy":"Always","name":"init-metrics","volumeMounts":[{"mountPath":"/metrics-volume","name":"metrics-volume"}]}]}' \
        --argjson emptydir '{"emptyDir": {}, "name": "metrics-volume"}' \
        --argjson metricsdir '{"mountPath": "/metrics-volume","name": "metrics-volume"}' \
        '.spec.template.spec += $init | .spec.template.spec.volumes += [$emptydir] | .spec.template.spec.containers[0].volumeMounts += [$metricsdir]' $deploy_yaml_file > $deploy_yaml_file.tmp && mv $deploy_yaml_file.tmp $deploy_yaml_file

	if [ "$ALLOW_SCHEDULED_TO_MASTER" == "true" ]; then
		# insert tolerations
		jq \
		--argjson tolerations '{"tolerations": [{"key":"node-role.kubernetes.io/master","operator":"Exists","effect":"NoSchedule"}]}' \
		'.spec.template.spec += $tolerations' $deploy_yaml_file > $deploy_yaml_file.tmp && mv $deploy_yaml_file.tmp $deploy_yaml_file
	fi

	cat "$deploy_yaml_file" | kubectl -n ${cluster_name} apply -f -
	rm -rf "$deploy_yaml_file" "$deploy_yaml_file".tmp
	kubectl -n ${cluster_name} patch deploy metrics-collector-deployment --type='json' -p='[{"op": "replace", "path": "/metadata/ownerReferences", "value": []}]'
	kubectl -n ${cluster_name} patch deploy metrics-collector-deployment --type='json' -p='[{"op": "remove", "path": "/spec/template/spec/containers/0/resources"}]'

	# deploy ClusterRoleBinding for read metrics from OCP prometheus
	rolebinding_yaml_file=${cluster_name}-metrics-collector-view.yaml
	cp -rf metrics-collector-view.yaml "$rolebinding_yaml_file"
	$sed_command "s~__CLUSTER_NAME__~${cluster_name}~g" "$rolebinding_yaml_file"
	cat "$rolebinding_yaml_file" | kubectl -n ${cluster_name} apply -f -
	rm -rf "$rolebinding_yaml_file"
        
done
