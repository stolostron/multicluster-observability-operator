#!/bin/bash
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
sleep 60

for depl in ${MINIO_DEPLOYMENTS}; do
    if ! kubectl -n open-cluster-management rollout status deployments $depl --timeout=$WAIT_TIMEOUT; then 
        echo "$depl is not ready after $WAIT_TIMEOUT"
        exit 1
    fi
done


for depl in ${OBSERVATORIUM_DEPLOYMENTS}; do
    if ! kubectl -n open-cluster-management rollout status deployments $depl --timeout=$WAIT_TIMEOUT; then 
        echo "$depl is not ready after $WAIT_TIMEOUT"
        exit 1
    fi
done


for depl in ${OBSERVATORIUM_STATEFULSET}; do
    if ! kubectl -n open-cluster-management rollout status statefulset $depl --timeout=$WAIT_TIMEOUT; then 
        echo "$depl is not ready after $WAIT_TIMEOUT"
        exit 1
    fi
done

for depl in ${GRAFANA_DEPLOYMENTS}; do
    if ! kubectl -n open-cluster-management rollout status deployments $depl --timeout=$WAIT_TIMEOUT; then 
        echo "$depl is not ready after $WAIT_TIMEOUT"
        exit 1
    fi
done
