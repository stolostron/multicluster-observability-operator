#!/usr/bin/env bash

# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

set -euxo pipefail

KEY="$SHARED_DIR/private.pem"
chmod 400 "$KEY"

IP="$(cat "$SHARED_DIR/public_ip")"
HOST="ec2-user@$IP"
OPT=(-q -o "UserKnownHostsFile=/dev/null" -o "StrictHostKeyChecking=no" -i "$KEY")

ssh "${OPT[@]}" "$HOST" kind create cluster --kubeconfig /tmp/kind-kubeconfig --name kind > >(tee "$ARTIFACT_DIR/run-e2e-in-kind.log") 2>&1
ssh "${OPT[@]}" "$HOST" kind version > >(tee "$ARTIFACT_DIR/run-e2e-in-kind.log") 2>&1
ssh "${OPT[@]}" "$HOST" kubectl cluster-info --kubeconfig /tmp/kind-kubeconfig > >(tee "$ARTIFACT_DIR/run-e2e-in-kind.log") 2>&1
ssh "${OPT[@]}" "$HOST" export MULTICLUSTER_OBSERVABILITY_OPERATOR_IMAGE_REF="$MULTICLUSTER_OBSERVABILITY_OPERATOR_IMAGE_REF" > >(tee "$ARTIFACT_DIR/run-e2e-in-kind.log") 2>&1
ssh "${OPT[@]}" "$HOST" export ENDPOINT_MONITORING_OPERATOR_IMAGE_REF="$ENDPOINT_MONITORING_OPERATOR_IMAGE_REF" > >(tee "$ARTIFACT_DIR/run-e2e-in-kind.log") 2>&1
ssh "${OPT[@]}" "$HOST" env

# scp "${OPT[@]}" tests/run-in-kind/run-e2e-in-kind.sh "$HOST:/tmp/run-e2e-in-kind.sh"
# ssh "${OPT[@]}" "$HOST" /tmp/run-e2e-in-kind.sh > >(tee "$ARTIFACT_DIR/run-e2e-in-kind.log") 2>&1
