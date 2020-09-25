#!/bin/bash
# Copyright (c) 2020 Red Hat, Inc.

if [ $# -ne 1 ]; then
	echo "this script must be run with the number of clusters:"
    echo -e "\n$0 total_clusters\n"
    exit 1
fi

re='^[0-9]+$'
if ! [[ $1 =~ $re ]] ; then
   echo "error: arguments <$1> not a number" >&2; exit 1
fi

test_namespace=test-clusters

for i in $(seq 1 $1)
do
	cluster_name=fake-cluster${i}
	deploy_name=${cluster_name}-metrics-collector-deployment
	kubectl delete deploy -n ${cluster_name} $deploy_name
	kubectl delete clusterrolebinding ${cluster_name}-clusters-metrics-collector-view
	kubectl delete -n ${cluster_name}  -f metrics-collector-serving-certs-ca-bundle.yaml
	kubectl delete ns ${cluster_name}
done
