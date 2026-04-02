#!/usr/bin/env bash
# Injects quay.io credentials into the cluster pull secret so that nodes can
# pull images from quay.io:443/acm-d (downstream dev builds).
#
# Usage:
#   QUAY_USER=rh-ee-you QUAY_TOKEN=<token> ./add-pull-secret.sh

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib/common.sh"

require_env QUAY_USER QUAY_TOKEN

WORK_DIR=$(mktemp -d)
trap 'rm -rf "$WORK_DIR"' EXIT

log_info "Fetching existing cluster pull secret..."
oc get secret pull-secret -n openshift-config \
  -o jsonpath='{.data.\.dockerconfigjson}' |
  base64 -d >"${WORK_DIR}/pull-secret.json"

log_info "Merging quay.io credentials for user ${QUAY_USER}..."
oc registry login \
  --registry="quay.io:443" \
  --auth-basic="${QUAY_USER}:${QUAY_TOKEN}" \
  --to="${WORK_DIR}/pull-secret.json"

log_info "Updating cluster pull secret..."
oc set data secret/pull-secret \
  -n openshift-config \
  --from-file=.dockerconfigjson="${WORK_DIR}/pull-secret.json"

log_info "Done. Nodes will restart rolling to apply the new credentials — this may take several minutes."
