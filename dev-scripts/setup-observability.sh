#!/usr/bin/env bash
# Deploys ACM Observability with MinIO as the object storage backend.
#
# Prerequisites:
#   - ACM (MultiClusterHub) must be installed and in Running state.
#   - oc must be logged into the target cluster.
#
# Usage:
#   ./setup-observability.sh

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib/common.sh"

MANIFESTS="${SCRIPT_DIR}/manifests"

log_info "Creating namespace ${MCO_NS}..."
oc apply -f "${MANIFESTS}/observability/namespace.yaml"

# Copy the cluster pull secret into the MCO namespace. MCO uses this to pull
# component images. We read it from openshift-config and recreate it rather
# than hardcoding credentials anywhere.
log_info "Copying cluster pull secret into ${MCO_NS}..."
PULL_SECRET_TMP=$(mktemp)
trap 'rm -f "$PULL_SECRET_TMP"' EXIT
oc get secret pull-secret -n openshift-config \
  -o jsonpath='{.data.\.dockerconfigjson}' | base64 -d >"$PULL_SECRET_TMP"
oc create secret generic multiclusterhub-operator-pull-secret \
  -n "${MCO_NS}" \
  --from-file=.dockerconfigjson="$PULL_SECRET_TMP" \
  --type=kubernetes.io/dockerconfigjson \
  --dry-run=client -o yaml | oc apply -f -

log_info "Deploying MinIO (ephemeral storage — data is lost on pod restart)..."
oc apply -f "${MANIFESTS}/storage/minio-deployment.yaml"
oc apply -f "${MANIFESTS}/storage/minio-service.yaml"
oc apply -f "${MANIFESTS}/storage/minio-route.yaml"

log_info "Waiting for MinIO to be ready..."
oc rollout status deployment/minio -n "${MCO_NS}" --timeout=120s

log_info "Creating Thanos object storage secret..."
oc apply -f "${MANIFESTS}/storage/thanos-storage-secret.yaml"

log_info "Deploying MultiClusterObservability CR..."
oc apply -f "${MANIFESTS}/observability/multiclusterobservability-cr.yaml"

wait_for_mco_ready 600

MINIO_ROUTE=$(oc get route minio -n "${MCO_NS}" -o jsonpath='{.spec.host}' 2>/dev/null || true)
log_info "Setup complete!"
log_info "  MinIO console: https://${MINIO_ROUTE} (admin: minioadmin / minioadmin)"
log_info "  To enable MCOA user-workload metrics, run: ./enable-mcoa-uwl.sh"
