#!/usr/bin/env bash
# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

if command -v python &> /dev/null
then
    PYTHON_CMD="python"
elif command -v python2 &> /dev/null
then
    PYTHON_CMD="python2"
elif command -v python3 &> /dev/null
then
    PYTHON_CMD="python3"
else
    echo "Failed to found python command, please install firstly"
    exit 1
fi

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

  savePath="."
  if [ $# -eq 2 ]; then
    savePath=$2
  fi

  if [ ! -d $savePath ]; then
    mkdir -p $savePath
    if [ $? -ne 0 ]; then
        echo "Failed to create directory <$savePath>"
        exit 1
    fi
  fi

  podName=`kubectl get pods -n open-cluster-management-observability -l app=multicluster-observability-grafana-dev --template '{{range .items}}{{.metadata.name}}{{"\n"}}{{end}}'`
  if [ $? -ne 0 ] || [ -z "$podName" ]; then
      echo "Failed to get grafana pod name, please check your grafana-dev deployment"
      exit 1
  fi

  dashboard_name=`echo ${1// /-} | tr '[:upper:]' '[:lower:]'`
  curlCMD="kubectl exec -it -n open-cluster-management-observability $podName -c grafana-dashboard-loader -- /usr/bin/curl"
  XForwardedUser="WHAT_YOU_ARE_DOING_IS_VOIDING_SUPPORT_0000000000000000000000000000000000000000000000000000000000000000"
  dashboard=`$curlCMD -s -X GET -H "Content-Type: application/json" -H "X-Forwarded-User: $XForwardedUser" 127.0.0.1:3001/api/dashboards/db/$dashboard_name`
  if [ $? -ne 0 ]; then
      echo "Failed to fetch dashboard UID, please check your dashboard name"
      exit 1
  fi
  dashboardUID=`echo $dashboard | $PYTHON_CMD -c "import sys, json; print(json.load(sys.stdin)['dashboard']['uid'])" 2>/dev/null`
  dashboardFolderId=`echo $dashboard | $PYTHON_CMD -c "import sys, json; print(json.load(sys.stdin)['meta']['folderId'])" 2>/dev/null`
  dashboardFolderTitle=`echo $dashboard | $PYTHON_CMD -c "import sys, json; print(json.load(sys.stdin)['meta']['folderTitle'])" 2>/dev/null`
  
  dashboardJson=`$curlCMD -s -X GET -H "Content-Type: application/json" -H "X-Forwarded-User:$XForwardedUser" 127.0.0.1:3001/api/dashboards/uid/$dashboardUID | $PYTHON_CMD -c "import sys, json; print(json.dumps(json.load(sys.stdin)['dashboard']))" 2>/dev/null`
  if [ $? -ne 0 ]; then
      echo "Failed to fetch dashboard json data, please check your dashboard name <$1>"
      exit 1
  fi

  if [ $dashboardFolderId -ne 0 ]; then
  cat > $savePath/$dashboard_name.yaml <<EOF
kind: ConfigMap
apiVersion: v1
metadata:
  name: $dashboard_name
  namespace: open-cluster-management-observability
  annotations:
    observability.open-cluster-management.io/dashboard-folder: $dashboardFolderTitle
  labels:
    grafana-custom-dashboard: "true"
data:
  $dashboard_name.json: |
    $dashboardJson
EOF
  else
  cat > $savePath/$dashboard_name.yaml <<EOF
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
  fi
  echo "Save dashboard <$dashboard_name> to $savePath/$dashboard_name.yaml"
}

start "$@"
