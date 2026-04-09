#!/usr/bin/env bash
# Switches ACM and MCE subscriptions back to the standard redhat-operators catalog
# and removes the downstream-specific CatalogSources and ImageDigestMirrorSet.
# ACM, MCH, and all running components are left untouched.
#
# Usage:
#   ./teardown-downstream-catalog.sh

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib/common.sh"

MANIFESTS="${SCRIPT_DIR}/manifests/catalog/downstream"

log_info "Switching ACM subscription back to redhat-operators..."
oc patch subscription.operators.coreos.com acm-sub -n "${ACM_NS}" \
  --type=merge -p '{
    "spec": {
      "source":          "redhat-operators",
      "sourceNamespace": "openshift-marketplace"
    }
  }' || log_warn "acm-sub patch failed (subscription may not exist or RBAC denied)"

log_info "Switching MCE subscription back to redhat-operators..."
oc patch subscription.operators.coreos.com mce-sub -n multicluster-engine \
  --type=merge -p '{
    "spec": {
      "source":          "redhat-operators",
      "sourceNamespace": "openshift-marketplace"
    }
  }' || log_warn "mce-sub patch failed (subscription may not exist or RBAC denied)"

log_info "Removing downstream CatalogSources..."
# Delete only the CatalogSources by name — the catalogsource.yaml also contains Namespaces,
# OperatorGroups and Subscriptions which must stay intact for ACM to keep running.
oc delete catalogsource acm-custom-registry -n openshift-marketplace --ignore-not-found
oc delete catalogsource multiclusterengine-catalog -n openshift-marketplace --ignore-not-found

log_info "Removing ImageDigestMirrorSet..."
oc delete --ignore-not-found -f "${MANIFESTS}/image-digest-mirror-set.yaml"

log_info "Done. ACM is now pinned to the standard redhat-operators catalog."
log_info "Use ./image-override.sh to test specific component builds on top of this."
