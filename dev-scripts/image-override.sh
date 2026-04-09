#!/usr/bin/env bash
# Applies image overrides to the MultiClusterHub operator for testing PR/snapshot builds.
#
# --- This-repo components (share a build, set all with TAG) ---
#   TAG=2.17.0-SNAPSHOT-2026-04-01-11-07-35 ./image-override.sh
#
# Per-component TAG variables (take precedence over TAG):
#   MCO_TAG                       multicluster-observability-operator
#   ENDPOINT_TAG                  endpoint-monitoring-operator
#   METRICS_COLLECTOR_TAG         metrics-collector
#   RBAC_QUERY_PROXY_TAG          rbac-query-proxy
#   GRAFANA_DASHBOARD_LOADER_TAG  grafana-dashboard-loader
#
# Default registry for this-repo components (default: quay.io/stolostron):
#   REGISTRY=quay.io/my-fork TAG=... ./image-override.sh
#
# --- External components (separate build, set tag + registry individually) ---
#   MCOA_ADDON_TAG=...                [MCOA_ADDON_REGISTRY=...]
#   OBSERVATORIUM_OPERATOR_TAG=...    [OBSERVATORIUM_OPERATOR_REGISTRY=...]
#   OBSERVATORIUM_TAG=...             [OBSERVATORIUM_REGISTRY=...]
#   GRAFANA_TAG=...                   [GRAFANA_REGISTRY=...]
#   THANOS_TAG=...                    [THANOS_REGISTRY=...]
#   PROMETHEUS_ALERTMANAGER_TAG=...   [PROMETHEUS_ALERTMANAGER_REGISTRY=...]
#   THANOS_RECEIVE_CONTROLLER_TAG=... [THANOS_RECEIVE_CONTROLLER_REGISTRY=...]
#
# Each external *_REGISTRY defaults to the global REGISTRY when not set.
#
# Full image-ref override (takes precedence over all TAG/REGISTRY vars):
#   MCO_IMAGE=quay.io/other/name:tag ./image-override.sh
#
# To revert: ./image-override-revert.sh

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib/common.sh"

DEFAULT_REGISTRY="quay.io/stolostron"
REGISTRY="${REGISTRY:-${DEFAULT_REGISTRY}}"
REPO_TAG="${TAG:-}"

# resolve_tag <component-specific-tag>
# Returns the component tag if set, else falls back to REPO_TAG.
resolve_tag() { echo "${1:-${REPO_TAG}}"; }

# resolve_registry <component-specific-registry>
# Returns the component registry if set, else falls back to global REGISTRY.
resolve_registry() { echo "${1:-${REGISTRY}}"; }

# parse_image_ref <full-ref>  →  prints "<remote> <tag>"
# Callers must supply a ref with an explicit tag (name:tag). A bare ref without
# a tag (e.g. registry:5000/image) will split on the port instead of the tag.
parse_image_ref() { echo "${1%:*}" "${1##*:}"; }

# format_entry <image-name> <key> <remote> <tag>  →  prints JSON object
format_entry() {
  echo "{\"image-name\":\"${1}\",\"image-remote\":\"${3}\",\"image-tag\":\"${4}\",\"image-key\":\"${2}\"}"
}

# add_repo_component <image-name> <key> <IMAGE_VAR> <TAG_VAR>
# Prints a JSON entry using the global REGISTRY and REPO_TAG fallback, or nothing.
add_repo_component() {
  local image_name="$1" key="$2" image_var="$3" tag_var="$4"
  local image_val="${!image_var:-}" tag_val tag_resolved r t
  tag_val="${!tag_var:-}"
  tag_resolved="$(resolve_tag "$tag_val")"

  if [[ -n $image_val ]]; then
    read -r r t <<<"$(parse_image_ref "$image_val")"
    format_entry "$image_name" "$key" "$r" "$t"
  elif [[ -n $tag_resolved ]]; then
    format_entry "$image_name" "$key" "$REGISTRY" "$tag_resolved"
  fi
}

# add_external_component <image-name> <key> <IMAGE_VAR> <TAG_VAR> <REGISTRY_VAR>
# Prints a JSON entry using per-component registry (falls back to global), or nothing.
add_external_component() {
  local image_name="$1" key="$2" image_var="$3" tag_var="$4" registry_var="$5"
  local image_val="${!image_var:-}" tag_val="${!tag_var:-}" registry_val="${!registry_var:-}" r t

  if [[ -n $image_val ]]; then
    read -r r t <<<"$(parse_image_ref "$image_val")"
    format_entry "$image_name" "$key" "$r" "$t"
  elif [[ -n $tag_val ]]; then
    format_entry "$image_name" "$key" "$(resolve_registry "$registry_val")" "$tag_val"
  fi
}

build_override_json() {
  local entries=() entry

  # --- This-repo components (covered by TAG) ---
  entry=$(add_repo_component multicluster-observability-operator multicluster_observability_operator MCO_IMAGE MCO_TAG)
  [[ -n $entry ]] && entries+=("$entry")

  entry=$(add_repo_component endpoint-monitoring-operator endpoint_monitoring_operator ENDPOINT_IMAGE ENDPOINT_TAG)
  [[ -n $entry ]] && entries+=("$entry")

  entry=$(add_repo_component metrics-collector metrics_collector METRICS_COLLECTOR_IMAGE METRICS_COLLECTOR_TAG)
  [[ -n $entry ]] && entries+=("$entry")

  entry=$(add_repo_component rbac-query-proxy rbac_query_proxy RBAC_QUERY_PROXY_IMAGE RBAC_QUERY_PROXY_TAG)
  [[ -n $entry ]] && entries+=("$entry")

  entry=$(add_repo_component grafana-dashboard-loader grafana_dashboard_loader GRAFANA_DASHBOARD_LOADER_IMAGE GRAFANA_DASHBOARD_LOADER_TAG)
  [[ -n $entry ]] && entries+=("$entry")

  # --- External components (independent tag + registry per component) ---
  entry=$(add_external_component multicluster-observability-addon multicluster_observability_addon MCOA_ADDON_IMAGE MCOA_ADDON_TAG MCOA_ADDON_REGISTRY)
  [[ -n $entry ]] && entries+=("$entry")

  entry=$(add_external_component observatorium-operator observatorium_operator OBSERVATORIUM_OPERATOR_IMAGE OBSERVATORIUM_OPERATOR_TAG OBSERVATORIUM_OPERATOR_REGISTRY)
  [[ -n $entry ]] && entries+=("$entry")

  entry=$(add_external_component observatorium observatorium OBSERVATORIUM_IMAGE OBSERVATORIUM_TAG OBSERVATORIUM_REGISTRY)
  [[ -n $entry ]] && entries+=("$entry")

  entry=$(add_external_component grafana grafana GRAFANA_IMAGE GRAFANA_TAG GRAFANA_REGISTRY)
  [[ -n $entry ]] && entries+=("$entry")

  entry=$(add_external_component thanos thanos THANOS_IMAGE THANOS_TAG THANOS_REGISTRY)
  [[ -n $entry ]] && entries+=("$entry")

  entry=$(add_external_component prometheus-alertmanager prometheus_alertmanager PROMETHEUS_ALERTMANAGER_IMAGE PROMETHEUS_ALERTMANAGER_TAG PROMETHEUS_ALERTMANAGER_REGISTRY)
  [[ -n $entry ]] && entries+=("$entry")

  entry=$(add_external_component thanos-receive-controller thanos_receive_controller THANOS_RECEIVE_CONTROLLER_IMAGE THANOS_RECEIVE_CONTROLLER_TAG THANOS_RECEIVE_CONTROLLER_REGISTRY)
  [[ -n $entry ]] && entries+=("$entry")

  if [[ ${#entries[@]} -eq 0 ]]; then
    log_error "Set at least one override. Examples:"
    log_error "  TAG=<tag>                                  all this-repo components"
    log_error "  MCO_TAG=<tag>                              MCO operator only"
    log_error "  THANOS_TAG=<tag> THANOS_REGISTRY=<reg>    external component with custom registry"
    exit 1
  fi

  local joined
  joined=$(printf ',%s' "${entries[@]}")
  echo "[${joined:1}]"
}

TMPFILE=$(mktemp /tmp/image-override-XXXXXX.json)
trap 'rm -f "$TMPFILE"' EXIT

log_info "Generating image-override.json (default registry: ${REGISTRY})..."
build_override_json >"$TMPFILE"
log_info "Overrides: $(cat "$TMPFILE")"

log_info "Creating image-override ConfigMap in ${ACM_NS}..."
oc create configmap image-override \
  -n "${ACM_NS}" \
  --from-file=image-override.json="$TMPFILE" \
  --dry-run=client -o yaml | oc apply -f -

log_info "Annotating MultiClusterHub to activate overrides..."
oc annotate multiclusterhub multiclusterhub \
  -n "${ACM_NS}" \
  --overwrite \
  "installer.open-cluster-management.io/image-overrides-configmap=image-override"

log_info "Image overrides applied. MCH will begin rolling out the new images."
