#!/usr/bin/env bash

# avoid client-side throttling due to HOME=/
export HOME=/tmp

base_dir="$(cd "$(dirname "$0")/.." ; pwd -P)"
cd "$base_dir"
obs_namespace=open-cluster-management-observability

# create a dashboard for test export grafana dashboard
kubectl apply -n "$obs_namespace" -f "$base_dir"/examples/dashboards/sample_custom_dashboard/custom-sample-dashboard.yaml

# test deploy grafana-dev
cd $base_dir/tools
./setup-grafana-dev.sh --deploy
if [ $? -ne 0 ]; then
    echo "Failed run setup-grafana-dev.sh --deploy"
    exit 1
fi

n=0
until [ "$n" -ge 30 ]
do
   kubectl get pods -n "$obs_namespace" -l app=multicluster-observability-grafana-dev | grep "2/2" | grep "Running" && break
   n=$((n+1))
   echo "Retrying in 10s for waiting for grafana-dev pod ready ..."
   sleep 10
done

if [ $n -eq 30 ]; then
    echo "Failed waiting for grafana-dev pod ready in 300s"
    exit 1
fi

podName=$(kubectl get pods -n "$obs_namespace" -l app=multicluster-observability-grafana-dev --template '{{range .items}}{{.metadata.name}}{{"\n"}}{{end}}')
if [ $? -ne 0 ] || [ -z "$podName" ]; then
    echo "Failed to get grafana pod name, please check your grafana-dev deployment"
    exit 1
fi

sleep 10
# create a new test user to test
kubectl -n "$obs_namespace" exec -it "$podName" -c grafana-dashboard-loader -- /usr/bin/curl -XPOST -H "Content-Type: application/json" -H "X-Forwarded-User: WHAT_YOU_ARE_DOING_IS_VOIDING_SUPPORT_0000000000000000000000000000000000000000000000000000000000000000" -d '{ "name":"test", "email":"test", "login":"test", "password":"test" }' '127.0.0.1:3001/api/admin/users'
sleep 30

n=0
until [ "$n" -ge 10 ]
do
    # test swith user to grafana admin
    ./switch-to-grafana-admin.sh test
    if [ $? -eq 0 ]; then
        break
    fi
    n=$((n+1))
    sleep 5
done
if [ $n -eq 10 ]; then
    echo "Failed run switch-to-grafana-admin.sh test"
    exit 1
fi

n=0
until [ "$n" -ge 10 ]
do
    # test export grafana dashboard
    ./generate-dashboard-configmap-yaml.sh "Sample Dashboard for E2E"
    if [ $? -eq 0 ]; then
        break
    fi
    n=$((n+1))
    sleep 5
done
if [ $n -eq 10 ]; then
    echo "Failed run generate-dashboard-configmap-yaml.sh"
    exit 1
fi

# test clean grafan-dev
./setup-grafana-dev.sh --clean
if [ $? -ne 0 ]; then
    echo "Failed run setup-grafana-dev.sh --clean"
    exit 1
fi

# clean test env
kubectl delete -n "$obs_namespace" -f "$base_dir"/examples/dashboards/sample_custom_dashboard/custom-sample-dashboard.yaml
