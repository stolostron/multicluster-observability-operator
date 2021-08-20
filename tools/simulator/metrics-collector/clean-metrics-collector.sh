#!/bin/bash
# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

sed_command='sed -i'
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
	kubectl delete deploy -n ${cluster_name} metrics-collector-deployment
	kubectl delete clusterrolebinding ${cluster_name}-clusters-metrics-collector-view
	kubectl delete -n ${cluster_name} secret/observability-managed-cluster-certs
	kubectl delete ns ${cluster_name}
done
