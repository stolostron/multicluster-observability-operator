#!/usr/bin/env bash
# Copyright (c) 2020 Red Hat, Inc.

usage() {
  cat <<EOF
Usage: $(basename "${BASH_SOURCE[0]}") [-h] dashboard_name [configmap_path]

Fetch grafana dashboard and save with a configmap.

Available options:

-h, --help           Print this help and exit
dashboard_name       Specified the dashboard to be fetch
configmap_path       Specified the path to save the configmap
EOF
  exit
}

start() {
  if ! [ $# -eq 1 -o $# -eq 2 ]; then
    usage
  fi

  save_path="."
  if [ $# -eq 2 ]; then
    save_path=$2
  fi

  if [ ! -d $save_path ]; then
    mkdir -p $save_path
    if [ $? -ne 0 ]; then
        echo "Failed to create directory <$save_path>"
        exit 1
    fi
  fi

  podName=`kubectl get pods -n open-cluster-management-observability -l app=multicluster-observability-grafana-dev --template '{{range .items}}{{.metadata.name}}{{"\n"}}{{end}}'`
    if [ $? -ne 0 ] || [ -z "$podName" ]; then
      echo "Failed to get grafana pod name, please check your grafana-dev deployment"
      exit 1
  fi

  dashboard_name=`echo ${$1// /-}`
  curlCMD="kubectl exec -it -n open-cluster-management-observability $podName -c grafana-dev -- /usr/bin/curl"
  XForwardedUser="WHAT_YOU_ARE_DOING_IS_VOIDING_SUPPORT_0000000000000000000000000000000000000000000000000000000000000000"
  dashboardUID=`$curlCMD -s -X GET -H "Content-Type: application/json" -H "X-Forwarded-User: $XForwardedUser" 127.0.0.1:3001/api/dashboards/db/$dashboard_name | python -c "import sys, json; print(json.load(sys.stdin)['dashboard']['uid'])" 2>/dev/null`
  if [ $? -ne 0 ]; then
      echo "Failed to fetch dashboard UID, please check your dashboard name"
      exit 1
  fi
  
  dashboardJson=`$curlCMD -s -X GET -H "Content-Type: application/json" -H "X-Forwarded-User:$XForwardedUser" 127.0.0.1:3001/api/dashboards/uid/$dashboardUID | python -c "import sys, json; print(json.load(sys.stdin)['dashboard'])"`
  if [ $? -ne 0 ]; then
      echo "Failed to fetch dashboard json data <$dashboard_name>"
      exit 1
  fi

  cat > $save_path/$dashboard_name.yaml <<EOF
kind: ConfigMap
apiVersion: v1
metadata:
  name: $dashboard_name
  namespace: open-cluster-management-observability
  labels:
    grafana-custom-dashboard: "true"
data:
  $dashboard_name.json: |
    $dashboardJson
EOF
  echo "Save dashboard <$dashboard_name> to $save_path/$dashboard_name.yaml"
}

start "$@"
