#!/bin/bash
function wait_for_popup() {
    n=1
    while true
    do
        entity=`kubectl get $1 $2 | grep -v Name | awk '{ print $1}'`
        if [ ! -z $entity ]; then
            return
        fi
        if [ $n -ge 5 ]; then
            exit 1
        fi
        n=$((n+1))
        echo "Retrying in 10s..."
        sleep 10
    done
}

export WAIT_TIMEOUT=${WAIT_TIMEOUT:-5m}

echo "Test to ensure all critical pods are running"

MULTICLUSTER_MONITORING_CR_NAME="monitoring"

MULTICLUSTER_MONITORING_DEPLOYMENTS="multicluster-monitoring-operator"
GRAFANA_DEPLOYMENTS="grafana"
MINIO_DEPLOYMENTS="minio"


OBSERVATORIUM_DEPLOYMENTS="$MULTICLUSTER_MONITORING_CR_NAME-observatorium-cortex-query-frontend $MULTICLUSTER_MONITORING_CR_NAME-observatorium-observatorium-api $MULTICLUSTER_MONITORING_CR_NAME-observatorium-observatorium-api-thanos-query $MULTICLUSTER_MONITORING_CR_NAME-observatorium-thanos-query $MULTICLUSTER_MONITORING_CR_NAME-observatorium-thanos-receive-controller"

OBSERVATORIUM_STATEFULSET="$MULTICLUSTER_MONITORING_CR_NAME-observatorium-thanos-compact $MULTICLUSTER_MONITORING_CR_NAME-observatorium-thanos-receive-default $MULTICLUSTER_MONITORING_CR_NAME-observatorium-thanos-rule $MULTICLUSTER_MONITORING_CR_NAME-observatorium-thanos-store-memcached $MULTICLUSTER_MONITORING_CR_NAME-observatorium-thanos-store-shard-0"

for depl in ${MULTICLUSTER_MONITORING_DEPLOYMENTS}; do
    if ! kubectl -n open-cluster-management rollout status deployments $depl --timeout=$WAIT_TIMEOUT; then 
        echo "$depl is not ready after $WAIT_TIMEOUT"
        exit 1
    fi
done

echo "wait for operator is ready for reconciling..."

for depl in ${MINIO_DEPLOYMENTS}; do
    wait_for_popup "deployments" $depl
    if ! kubectl -n open-cluster-management rollout status deployments $depl --timeout=$WAIT_TIMEOUT; then 
        echo "$depl is not ready after $WAIT_TIMEOUT"
        exit 1
    fi
done


for depl in ${OBSERVATORIUM_DEPLOYMENTS}; do
    wait_for_popup "deployments" $depl
    if ! kubectl -n open-cluster-management rollout status deployments $depl --timeout=$WAIT_TIMEOUT; then 
        echo "$depl is not ready after $WAIT_TIMEOUT"
        exit 1
    fi
done


for depl in ${OBSERVATORIUM_STATEFULSET}; do
    wait_for_popup "statefulset" $depl
    if ! kubectl -n open-cluster-management rollout status statefulset $depl --timeout=$WAIT_TIMEOUT; then 
        echo "$depl is not ready after $WAIT_TIMEOUT"
        exit 1
    fi
done

for depl in ${GRAFANA_DEPLOYMENTS}; do
    wait_for_popup "deployments" $depl
    if ! kubectl -n open-cluster-management rollout status deployments $depl --timeout=$WAIT_TIMEOUT; then 
        echo "$depl is not ready after $WAIT_TIMEOUT"
        exit 1
    fi
done
