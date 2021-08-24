#!/usr/bin/env bash

# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

set -euxo pipefail

sed_command='sed -i-e -e'
if [[ "$(uname)" == "Darwin" ]]; then
    sed_command='sed -i '-e' -e'
fi

KEY="$SHARED_DIR/private.pem"
chmod 400 "$KEY"

IP="$(cat "$SHARED_DIR/public_ip")"
HOST="ec2-user@$IP"
OPT=(-q -o "UserKnownHostsFile=/dev/null" -o "StrictHostKeyChecking=no" -i "$KEY")

$sed_command "s~__MULTICLUSTER_OBSERVABILITY_OPERATOR_IMAGE_REF__$~$MULTICLUSTER_OBSERVABILITY_OPERATOR_IMAGE_REF~g" ./tests/run-in-kind/image_ref_env.sh
$sed_command "s~__ENDPOINT_MONITORING_OPERATOR_IMAGE_REF__$~$ENDPOINT_MONITORING_OPERATOR_IMAGE_REF~g" ./tests/run-in-kind/image_ref_env.sh
$sed_command "s~__GRAFANA_DASHBOARD_LOADER_IMAGE_REF__$~$GRAFANA_DASHBOARD_LOADER_IMAGE_REF~g" ./tests/run-in-kind/image_ref_env.sh
$sed_command "s~__METRICS_COLLECTOR_IMAGE_REF__$~$METRICS_COLLECTOR_IMAGE_REF~g" ./tests/run-in-kind/image_ref_env.sh
$sed_command "s~__RBAC_QUERY_PROXY_IMAGE_REF__$~$RBAC_QUERY_PROXY_IMAGE_REF~g" ./tests/run-in-kind/image_ref_env.sh

ssh "${OPT[@]}" "$HOST" sudo yum install gcc git -y
scp "${OPT[@]}" -r ../multicluster-observability-operator "$HOST:/tmp/multicluster-observability-operator"
ssh "${OPT[@]}" "$HOST" "cd /tmp/multicluster-observability-operator/tests/run-in-kind && ./run-e2e-in-kind.sh" > >(tee "$ARTIFACT_DIR/run-e2e-in-kind.log") 2>&1
