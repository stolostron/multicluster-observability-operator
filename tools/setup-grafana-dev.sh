#!/usr/bin/env bash
# Copyright (c) 2020 Red Hat, Inc.

sed_command='sed -i-e -e'
if [[ "$(uname)" == "Darwin" ]]; then
    sed_command='sed -i '-e' -e'
fi

usage() {
  cat <<EOF
Usage: $(basename "${BASH_SOURCE[0]}") [-h] [-d | -c]

Grafana deployment tools.

Available options:

-h, --help      Print this help and exit
-d, --deploy    Deploy all related grafana resources
-c, --clean     Clean all related grafana resources
EOF
  exit
}

deploy() {
  echo "Check MCO CR and fetch grafana image info ..."
  if kubectl get mco observability >/dev/null 2>&1; then
    grafana_img=`kubectl get deployment -n open-cluster-management-observability -l app=multicluster-observability-grafana -o yaml | grep quay.io/open-cluster-management/grafana@ | awk '{print $2}'`
    $sed_command "s~image: quay.io/open-cluster-management/grafana@.*$~image: $grafana_img~g" manifests/deployment.yaml
    grafana_dashboard_loader_img=`kubectl get deployment -n open-cluster-management-observability -l app=multicluster-observability-grafana -o yaml | grep quay.io/open-cluster-management/grafana-dashboard-loader@ | awk '{print $3}'`
    $sed_command "s~image: quay.io/open-cluster-management/grafana-dashboard-loader.*$~image: $grafana_dashboard_loader_img~g" manifests/deployment.yaml
  else
    echo "Failed to get MCO CR, exit"
    exit 1
  fi
  echo "Deploy all related grafana resources ..."
  kubectl apply -k ./manifests
}

clean() {
  echo "Clean all related grafana resources ..."
  kubectl delete -k ./manifests
}

msg() {
  echo >&2 -e "${1-}"
}

die() {
  local msg=$1
  local code=${2-1}
  msg "$msg"
  exit "$code"
}

start() {
  case "${1-}" in
  -h | --help) usage ;;
  -d | --deploy) deploy ;; 
  -c | --clean) clean ;;
  -?*) die "Unknown option: $1" ;;
    *) usage ;;
  esac
}

start "$@"
