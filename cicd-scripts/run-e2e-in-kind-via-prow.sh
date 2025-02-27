#!/usr/bin/env bash

# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

set -euxo pipefail

source ./.bingo/variables.env

KEY="${SHARED_DIR}/private.pem"
chmod 400 "${KEY}"

IP="$(cat "${SHARED_DIR}/public_ip")"
HOST="ec2-user@${IP}"
OPT=(-q -o "UserKnownHostsFile=/dev/null" -o "StrictHostKeyChecking=no" -i "${KEY}")

# support gnu sed only give that this script will be executed in prow env
SED_COMMAND='sed -i-e -e'

source ./scripts/test-utils.sh
${SED_COMMAND} "$ a\export LATEST_SNAPSHOT=$(get_latest_acm_snapshot)" ./tests/run-in-kind/env.sh

if [ "${OPENSHIFT_CI}" == "true" ]; then
  ${SED_COMMAND} "$ a\export OPENSHIFT_CI=${OPENSHIFT_CI}" ./tests/run-in-kind/env.sh
fi

if [[ -n ${PULL_BASE_REF} ]]; then
  ${SED_COMMAND} "$ a\export PULL_BASE_REF=${PULL_BASE_REF}" ./tests/run-in-kind/env.sh
fi

if [[ -n ${MULTICLUSTER_OBSERVABILITY_OPERATOR_IMAGE_REF} ]]; then
  ${SED_COMMAND} "$ a\export MULTICLUSTER_OBSERVABILITY_OPERATOR_IMAGE_REF=${MULTICLUSTER_OBSERVABILITY_OPERATOR_IMAGE_REF}" ./tests/run-in-kind/env.sh
fi
if [[ -n ${ENDPOINT_MONITORING_OPERATOR_IMAGE_REF} ]]; then
  ${SED_COMMAND} "$ a\export ENDPOINT_MONITORING_OPERATOR_IMAGE_REF=${ENDPOINT_MONITORING_OPERATOR_IMAGE_REF}" ./tests/run-in-kind/env.sh
fi
if [[ -n ${GRAFANA_DASHBOARD_LOADER_IMAGE_REF} ]]; then
  ${SED_COMMAND} "$ a\export GRAFANA_DASHBOARD_LOADER_IMAGE_REF=${GRAFANA_DASHBOARD_LOADER_IMAGE_REF}" ./tests/run-in-kind/env.sh
fi
if [[ -n ${METRICS_COLLECTOR_IMAGE_REF} ]]; then
  ${SED_COMMAND} "$ a\export METRICS_COLLECTOR_IMAGE_REF=${METRICS_COLLECTOR_IMAGE_REF}" ./tests/run-in-kind/env.sh
fi
if [[ -n ${RBAC_QUERY_PROXY_IMAGE_REF} ]]; then
  ${SED_COMMAND} "$ a\export RBAC_QUERY_PROXY_IMAGE_REF=${RBAC_QUERY_PROXY_IMAGE_REF}" ./tests/run-in-kind/env.sh
fi

ssh "${OPT[@]}" "$HOST" sudo yum install gcc git -y
ssh "${OPT[@]}" "$HOST" sudo mkdir -p /home/ec2-user/bin
ssh "${OPT[@]}" "$HOST" sudo chmod 777 /home/ec2-user/bin
scp "${OPT[@]}" -r ../multicluster-observability-operator "$HOST:/tmp/multicluster-observability-operator"
scp "${OPT[@]}" $KUSTOMIZE "$HOST:/home/ec2-user/bin/kustomize"
scp "${OPT[@]}" $(which jq) "$HOST:/home/ec2-user/bin"
ssh "${OPT[@]}" "$HOST" "cd /tmp/multicluster-observability-operator && make mco-kind-env"
ssh "${OPT[@]}" "$HOST" "cd /tmp/multicluster-observability-operator && make e2e-tests-in-kind" > >(tee "$ARTIFACT_DIR/run-e2e-in-kind.log") 2>&1
