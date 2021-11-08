#!/bin/bash
# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

WORK_DIR="$(cd "$(dirname "$0")" ; pwd -P)"

if ! command -v jq &> /dev/null; then
	if [[ "$(uname)" == "Linux" ]]; then
		curl -o jq -L https://github.com/stedolan/jq/releases/download/jq-1.6/jq-linux64
	elif [[ "$(uname)" == "Darwin" ]]; then
		curl -o jq -L https://github.com/stedolan/jq/releases/download/jq-1.6/jq-osx-amd64
	fi
	chmod +x ./jq
fi

KUBECTL="kubectl"
if ! command -v kubectl &> /dev/null; then
    if command -v oc &> /dev/null; then
        KUBECTL="oc"
    else
        echo "kubectl or oc must be installed!"
        exit 1
    fi
fi

SED_COMMAND='sed -i'
if [[ "$(uname)" == "Darwin" ]]; then
	SED_COMMAND='sed -i -e'
fi

function usage() {
	echo "${0} -n NUMBERS [-w WORKERS] [-m MANAGED_CLUSTER_PREFIX]"
	echo ''
	# shellcheck disable=SC2016
	echo '  -n: Specifies the total number of simulated metrics collectors, required'
	# shellcheck disable=SC2016
	echo '  -w: Specifies the worker threads for each simulated metrics collector, optional, the default value is "1".'
	# shellcheck disable=SC2016
	echo '  -m: Specifies the prefix for the simulated managedcluster name, optional, the default value is "simulated-managed-cluster".'
	echo ''
}

WORKERS=1 # default worker threads for each simulated metrics collector
MANAGED_CLUSTER_PREFIX="simulated-managed-cluster" # default managedccluster name prefix
# metrics data source image
METRICS_IMAGE="${METRICS_IMAGE:-quay.io/ocm-observability/metrics-data:2.4.0}"

# Allow command-line args to override the defaults.
while getopts ":n:w:m:h" opt; do
	case ${opt} in
		n)
			NUMBERS=${OPTARG}
			;;
		w)
			WORKERS=${OPTARG}
			;;
		m)
			MANAGED_CLUSTER_PREFIX=${OPTARG}
			;;
		h)
			usage
			exit 0
			;;
		\?)
			echo "Invalid option: -${OPTARG}" >&2
			usage
			exit 1
			;;
	esac
done

if [[ -z "${NUMBERS}" ]]; then
	echo "Error: NUMBERS (-n) must be specified!"
	usage
	exit 1
fi

re='^[0-9]+$'
if ! [[ ${NUMBERS} =~ ${re} ]] ; then
	echo "error: arguments <${NUMBERS}> is not a number" >&2; exit 1
fi

if ! [[ ${WORKERS} =~ ${re} ]] ; then
	echo "error: arguments <${WORKERS}> is not a number" >&2; exit 1
fi

OBSERVABILITY_NS="open-cluster-management-addon-observability"

for i in $(seq 1 ${NUMBERS})
do
	cluster_name=${MANAGED_CLUSTER_PREFIX}-${i}
	${KUBECTL} create ns ${cluster_name}

	# create ca/sa/rolebinding for metrics collector
	${KUBECTL} get configmap metrics-collector-serving-certs-ca-bundle -n ${OBSERVABILITY_NS} -o json | jq 'del(.metadata.namespace,.metadata.resourceVersion,.metadata.uid) | .metadata.creationTimestamp=null' | ${KUBECTL} apply -n ${cluster_name} -f -
	${KUBECTL} get secret observability-controller-open-cluster-management.io-observability-signer-client-cert -n ${OBSERVABILITY_NS} -o json | jq 'del(.metadata.namespace,.metadata.resourceVersion,.metadata.uid) | .metadata.creationTimestamp=null' | ${KUBECTL} apply -n ${cluster_name} -f -
	${KUBECTL} get secret observability-managed-cluster-certs -n ${OBSERVABILITY_NS} -o json | jq 'del(.metadata.namespace,.metadata.resourceVersion,.metadata.uid) | .metadata.creationTimestamp=null' | ${KUBECTL} apply -n ${cluster_name} -f -
	${KUBECTL} get sa endpoint-observability-operator-sa -n ${OBSERVABILITY_NS} -o json | jq 'del(.metadata.namespace,.metadata.resourceVersion,.metadata.uid) | .metadata.creationTimestamp=null' | ${KUBECTL} apply -n ${cluster_name} -f -
	${KUBECTL} -n ${cluster_name} patch secret observability-managed-cluster-certs --type='json' -p='[{"op": "replace", "path": "/metadata/ownerReferences", "value": []}]'
	${KUBECTL} -n ${cluster_name} patch sa endpoint-observability-operator-sa --type='json' -p='[{"op": "replace", "path": "/metadata/ownerReferences", "value": []}]'

	# deploy metrics collector deployment to cluster ns
	deploy_yaml_file=${cluster_name}-metrics-collector-deployment.json
	${KUBECTL} get deploy metrics-collector-deployment -n ${OBSERVABILITY_NS} -o json > ${deploy_yaml_file}

	# replace namespace, cluster and clusterID. Insert --simulated-timeseries-file
	uuid=$(cat /proc/sys/kernel/random/uuid)
	jq \
		--arg cluster_name ${cluster_name} \
		--arg cluster "--label=\"cluster=${cluster_name}\"" \
		--arg clusterID "--label=\"clusterID=${uuid}\"" \
		--arg workerNum "--worker-number=${WORKERS}" \
		--arg file "--simulated-timeseries-file=/metrics-volume/timeseries.txt" \
		'.metadata.namespace=$cluster_name | .spec.template.spec.containers[0].command[.spec.template.spec.containers[0].command|length] |= . + $cluster |.spec.template.spec.containers[0].command[.spec.template.spec.containers[0].command|length] |= . + $clusterID | .spec.template.spec.containers[0].command[.spec.template.spec.containers[0].command|length] |= . + $file | .spec.template.spec.containers[0].command[.spec.template.spec.containers[0].command|length] |= . + $workerNum' ${deploy_yaml_file} > ${deploy_yaml_file}.tmp && mv ${deploy_yaml_file}.tmp ${deploy_yaml_file}

	# insert metrics initContainer
    jq \
        --argjson init '{"initContainers": [{"command":["sh","-c","cp /tmp/timeseries.txt /metrics-volume"],"image":"'${METRICS_IMAGE}'","imagePullPolicy":"Always","name":"init-metrics","volumeMounts":[{"mountPath":"/metrics-volume","name":"metrics-volume"}]}]}' \
        --argjson emptydir '{"emptyDir": {}, "name": "metrics-volume"}' \
        --argjson metricsdir '{"mountPath": "/metrics-volume","name": "metrics-volume"}' \
        '.spec.template.spec += $init | .spec.template.spec.volumes += [$emptydir] | .spec.template.spec.containers[0].volumeMounts += [$metricsdir]' ${deploy_yaml_file} > ${deploy_yaml_file}.tmp && mv ${deploy_yaml_file}.tmp ${deploy_yaml_file}

	if [ "$ALLOW_SCHEDULED_TO_MASTER" == "true" ]; then
		# insert tolerations
		jq \
			--argjson tolerations '{"tolerations": [{"key":"node-role.kubernetes.io/master","operator":"Exists","effect":"NoSchedule"}]}' \
			'.spec.template.spec += $tolerations' ${deploy_yaml_file} > ${deploy_yaml_file}.tmp && mv ${deploy_yaml_file}.tmp ${deploy_yaml_file}
	fi

	cat "${deploy_yaml_file}" | ${KUBECTL} -n ${cluster_name} apply -f -
	rm -f "${deploy_yaml_file}" "${deploy_yaml_file}".tmp
	${KUBECTL} -n ${cluster_name} patch deploy metrics-collector-deployment --type='json' -p='[{"op": "replace", "path": "/metadata/ownerReferences", "value": []}]'
	${KUBECTL} -n ${cluster_name} patch deploy metrics-collector-deployment --type='json' -p='[{"op": "remove", "path": "/spec/template/spec/containers/0/resources"}]'

	# deploy ClusterRoleBinding for read metrics from OCP prometheus
	rolebinding_yaml_file=${cluster_name}-metrics-collector-view.yaml
	cp -rf metrics-collector-view.yaml "$rolebinding_yaml_file"
	${SED_COMMAND} "s~__CLUSTER_NAME__~${cluster_name}~g" "${rolebinding_yaml_file}"
	cat "${rolebinding_yaml_file}" | ${KUBECTL} -n ${cluster_name} apply -f -
	rm -f "${rolebinding_yaml_file}"
done

