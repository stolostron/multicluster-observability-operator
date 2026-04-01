#!/usr/bin/env bash
# Reverts the MCH image overrides applied by image-override.sh,
# restoring the default images from the installed operator bundle.

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib/common.sh"

log_info "Clearing mch-imageOverridesCM annotation from MultiClusterHub..."
oc annotate multiclusterhub multiclusterhub \
  -n "${ACM_NS}" \
  --overwrite \
  "installer.open-cluster-management.io/image-overrides-configmap="

log_info "Deleting image-override ConfigMap..."
oc delete configmap image-override \
  -n "${ACM_NS}" \
  --ignore-not-found

log_info "Image overrides reverted. MCH will roll back to default images."
