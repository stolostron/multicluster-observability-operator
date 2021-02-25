#!/usr/bin/env bash
# Copyright (c) 2021 Red Hat, Inc.

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
Usage: $(basename "${BASH_SOURCE[0]}") [-h] user_name

Switch the specified user to be grafana admin.

Available options:

-h, --help      Print this help and exit
user_name       Specified the user to be switch
EOF
  exit
}

start() {
  if [ $# -ne 1 ]; then
    usage
  fi

  podName=`kubectl get pods -n open-cluster-management-observability -l app=multicluster-observability-grafana-dev --template '{{range .items}}{{.metadata.name}}{{"\n"}}{{end}}'`
  if [ $? -ne 0 ] || [ -z "$podName" ]; then
      echo "Failed to get grafana pod name, please check your grafana-dev deployment"
      exit 1
  fi

  curlCMD="kubectl exec -it -n open-cluster-management-observability $podName -c grafana-dashboard-loader -- /usr/bin/curl"
  XForwardedUser="WHAT_YOU_ARE_DOING_IS_VOIDING_SUPPORT_0000000000000000000000000000000000000000000000000000000000000000"
  userID=`$curlCMD -s -X GET -H "Content-Type: application/json" -H "X-Forwarded-User: $XForwardedUser" 127.0.0.1:3001/api/users/lookup?loginOrEmail=$1 | $PYTHON_CMD -c "import sys, json; print(json.load(sys.stdin)['id'])" 2>/dev/null`
  if [ $? -ne 0 ]; then
      echo "Failed to fetch user ID, please check your user name"
      exit 1
  fi
  
  orgID=`$curlCMD -s -X GET -H "Content-Type: application/json" -H "X-Forwarded-User:$XForwardedUser" 127.0.0.1:3001/api/users/lookup?loginOrEmail=$1 | $PYTHON_CMD -c "import sys, json; print(json.load(sys.stdin)['orgId'])" 2>/dev/null`
  if [ $? -ne 0 ]; then
      echo "Failed to fetch organization ID, please check your user name"
      exit 1
  fi

  $curlCMD -s -X DELETE -H "Content-Type: application/json" -H "X-Forwarded-User:$XForwardedUser" 127.0.0.1:3001/api/orgs/$orgID/users/$userID > /dev/null
  if [ $? -ne 0 ]; then
      echo "Failed to delete user <$1>"
      exit 1
  fi

  $curlCMD -s -X POST -H "Content-Type: application/json" -d "{\"loginOrEmail\":\"$1\", \"role\": \"Admin\"}" -H "X-Forwarded-User:$XForwardedUser" 127.0.0.1:3001/api/orgs/$orgID/users > /dev/null
  if [ $? -ne 0 ]; then
      echo "Failed to switch the user <$1> to be grafana admin"
      exit 1
  fi
  echo "User <$1> switched to be grafana admin"

  #disable getting start
  kubectl exec -it -n open-cluster-management-observability $podName -c grafana-dev -- sqlite3 /var/lib/grafana/grafana.db "update user set help_flags1=1 where id=$userID;" > /dev/null
  if [ $? -ne 0 ]; then
      echo "Failed to disable getting start for the user <$1>"
      exit 1
  fi

}

start "$@"
