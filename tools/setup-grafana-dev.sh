#!/usr/bin/env bash
# Copyright (c) 2021 Red Hat, Inc.

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
  kubectl get secret -n open-cluster-management-observability grafana-config -o 'go-template={{index .data "grafana.ini"}}' | base64 --decode > grafana-dev-config.ini
  if [ $? -ne 0 ]; then
      echo "Failed to get grafana config secret"
      exit 1
  fi
  $sed_command "s~%(domain)s/grafana/$~%(domain)s/grafana-dev/~g" grafana-dev-config.ini
  kubectl create secret generic grafana-dev-config -n open-cluster-management-observability --from-file=grafana.ini=grafana-dev-config.ini

  kubectl get deployment -n open-cluster-management-observability -l app=multicluster-observability-grafana -o yaml > grafana-dev-deploy.yaml
  if [ $? -ne 0 ]; then
      echo "Failed to get grafana deployment"
      exit 1
  fi
  $sed_command "s~name: grafana$~name: grafana-dev~g" grafana-dev-deploy.yaml
  $sed_command "s~replicas:.*$~replicas: 1~g" grafana-dev-deploy.yaml
  $sed_command "s~grafana-config$~grafana-dev-config~g" grafana-dev-deploy.yaml
  $sed_command "s~app: multicluster-observability-grafana$~app: multicluster-observability-grafana-dev~g" grafana-dev-deploy.yaml
  $sed_command "s~grafana-config$~grafana-dev-config~g" grafana-dev-deploy.yaml
  $sed_command "s~  securityContext:.*$~  securityContext: {runAsUser: 0}~g" grafana-dev-deploy.yaml
  sed "s~- emptyDir: {}$~- persistentVolumeClaim:$            claimName: grafana-dev~g" grafana-dev-deploy.yaml > grafana-dev-deploy.yaml.bak
  tr $ '\n' < grafana-dev-deploy.yaml.bak > grafana-dev-deploy.yaml
  kubectl apply -f grafana-dev-deploy.yaml

  kubectl get svc -n open-cluster-management-observability -l app=multicluster-observability-grafana -o yaml > grafana-dev-svc.yaml
  if [ $? -ne 0 ]; then
      echo "Failed to get grafana service"
      exit 1
  fi
  $sed_command "s~name: grafana$~name: grafana-dev~g" grafana-dev-svc.yaml
  $sed_command "s~app: multicluster-observability-grafana$~app: multicluster-observability-grafana-dev~g" grafana-dev-svc.yaml
  $sed_command "s~clusterIP:.*$~ ~g" grafana-dev-svc.yaml
  kubectl apply -f grafana-dev-svc.yaml

  kubectl get ingress -n open-cluster-management-observability grafana -o yaml > grafana-dev-ingress.yaml
  if [ $? -ne 0 ]; then
      echo "Failed to get grafana ingress"
      exit 1
  fi
  $sed_command "s~name: grafana$~name: grafana-dev~g" grafana-dev-ingress.yaml
  $sed_command "s~serviceName: grafana$~serviceName: grafana-dev~g" grafana-dev-ingress.yaml
  $sed_command "s~path: /grafana$~path: /grafana-dev~g" grafana-dev-ingress.yaml
  kubectl apply -f grafana-dev-ingress.yaml
  
  cat >grafana-pvc.yaml <<EOL
kind: PersistentVolumeClaim
apiVersion: v1
metadata:
  name: grafana-dev
  namespace: open-cluster-management-observability
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
  storageClassName: gp2
EOL
  PVC_CLASS=$(kubectl get pvc -n open-cluster-management-observability alertmanager-db-alertmanager-0 -o yaml|grep "  storageClassName")
  $sed_command "s~  storageClassName:.*$~${PVC_CLASS}~g" grafana-pvc.yaml
  kubectl apply -f grafana-pvc.yaml

  # clean all tmp files
  rm -rf grafana-dev-deploy.yaml* grafana-dev-svc.yaml* grafana-dev-ingress.yaml* grafana-dev-config.ini* grafana-pvc.yaml*
}

clean() {
  kubectl delete secret -n open-cluster-management-observability grafana-dev-config
  kubectl delete deployment -n open-cluster-management-observability grafana-dev
  kubectl delete svc -n open-cluster-management-observability grafana-dev
  kubectl delete ingress -n open-cluster-management-observability grafana-dev
  kubectl delete pvc -n open-cluster-management-observability grafana-dev
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
