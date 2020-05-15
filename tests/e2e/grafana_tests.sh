#!/bin/bash
# Copyright (c) 2020 Red Hat, Inc.

# test grafana replicas changes
kubectl patch MultiClusterMonitoring monitoring --patch '{"spec":{"grafana":{"replicas":2}}}' --type=merge

n=1
while true
do
    # check there are 2 grafana pods here
    replicas=`kubectl get deployment grafana | grep -v AVAILABLE | awk '{ print $4 }'`
    if [[ $replicas -eq 2 ]]; then
        echo "grafana replicas is update to 2 successfully."
        break
    fi
    if [[ $n -ge 10 ]]; then
        echo "grafana replicas changes test is failed."
        exit 1
    fi
    n=$((n+1))
    echo "Retrying in 10s..."
    sleep 10
done
