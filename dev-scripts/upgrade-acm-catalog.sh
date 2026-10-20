#!/usr/bin/env bash
# Upgrade the ACM installed by Prow to a bundle from a custom catalog before E2E.

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib/common.sh"

# Allow callers to skip the catalog switch with CUSTOM_ACM_CATALOG_IMAGE=.
[[ -n ${CUSTOM_ACM_CATALOG_IMAGE:-} ]] || exit 0

require_env EXPECTED_MCH_OPERATOR_IMAGE
require_tool oc
require_tool jq

if [[ -z ${KUBECONFIG:-} && -n ${SHARED_DIR:-} ]]; then
  export KUBECONFIG="${SHARED_DIR}/hub-1.kc"
fi
if [[ -z ${KUBECONFIG:-} || ! -r ${KUBECONFIG} ]]; then
  log_error "Set KUBECONFIG to the hub cluster kubeconfig before upgrading ACM"
  exit 1
fi

CATALOG_NS=openshift-marketplace
CATALOG_NAME=acm-custom-registry
ACM_PACKAGE=advanced-cluster-management
MCH_DEPLOYMENT=multiclusterhub-operator
MCO_DEPLOYMENT=multicluster-observability-operator
EXPECTED_ACM_CHANNEL="${EXPECTED_ACM_CHANNEL:-release-2.17}"
UPGRADE_TIMEOUT="${ACM_CATALOG_UPGRADE_TIMEOUT:-900}"
POLL_INTERVAL=15

if ! [[ $UPGRADE_TIMEOUT =~ ^[0-9]+$ ]] || ((UPGRADE_TIMEOUT < POLL_INTERVAL)); then
  log_error "ACM_CATALOG_UPGRADE_TIMEOUT must be at least ${POLL_INTERVAL} seconds"
  exit 1
fi

subscription_json=$(oc get subscriptions -n "$ACM_NS" -o json)
subscription_json=$(jq -ce --arg package "$ACM_PACKAGE" --arg source "$CATALOG_NAME" \
  --arg namespace "$CATALOG_NS" \
  '[.items[] | select(.spec.name == $package and .spec.source == $source and .spec.sourceNamespace == $namespace)] |
   if length == 1 then .[0] else error("expected exactly one ACM subscription using acm-custom-registry") end' \
  <<<"$subscription_json")

subscription_name=$(jq -r '.metadata.name' <<<"$subscription_json")
installed_csv=$(jq -r '.status.installedCSV // empty' <<<"$subscription_json")
channel=$(jq -r '.spec.channel // empty' <<<"$subscription_json")
approval=$(jq -r '.spec.installPlanApproval // "Automatic"' <<<"$subscription_json")

if [[ -z $installed_csv || $channel != "$EXPECTED_ACM_CHANNEL" || $approval != Automatic ]]; then
  log_error "ACM subscription ${subscription_name} must have an installed CSV, channel ${EXPECTED_ACM_CHANNEL}, and Automatic approval"
  log_error "Found installedCSV=${installed_csv:-<empty>}, channel=${channel:-<empty>}, approval=${approval}"
  exit 1
fi

log_info "Installed ACM CSV: ${installed_csv}"

# The shared Prow deploy step injects this PR's MCO image into the original ACM
# CSV. Preserve the running image because the replacement CSV can undo that edit.
mco_image=$(oc get deployment "$MCO_DEPLOYMENT" -n "$ACM_NS" -o json |
  jq -r '[.spec.template.spec.containers[] | select(.name == "multicluster-observability-operator") | .image][0] // empty')
if [[ -z $mco_image ]]; then
  log_error "Could not determine the running MCO operator image"
  exit 1
fi
log_info "Preserving MCO operator image: ${mco_image}"

catalog_image=$(oc get catalogsource "$CATALOG_NAME" -n "$CATALOG_NS" -o jsonpath='{.spec.image}')
if [[ $catalog_image != "$CUSTOM_ACM_CATALOG_IMAGE" ]]; then
  log_info "Changing CatalogSource ${CATALOG_NS}/${CATALOG_NAME} to ${CUSTOM_ACM_CATALOG_IMAGE}"
  image_patch=$(jq -nc --arg image "$CUSTOM_ACM_CATALOG_IMAGE" '{spec:{image:$image}}')
  oc patch catalogsource "$CATALOG_NAME" -n "$CATALOG_NS" --type=merge -p "$image_patch"
else
  log_info "CatalogSource already uses ${CUSTOM_ACM_CATALOG_IMAGE}"
fi

csv_has_expected_mch_image() {
  oc get csv "$1" -n "$ACM_NS" -o json |
    jq -e --arg image "$EXPECTED_MCH_OPERATOR_IMAGE" \
      '[.spec.install.spec.deployments[]?.spec.template.spec.containers[]?.image] | index($image) != null' \
      >/dev/null
}

new_csv="$installed_csv"
if ! csv_has_expected_mch_image "$installed_csv"; then
  log_info "Waiting for ACM to upgrade from ${installed_csv} (timeout: ${UPGRADE_TIMEOUT}s)..."
  elapsed=0
  while ((elapsed < UPGRADE_TIMEOUT)); do
    new_csv=$(oc get subscription "$subscription_name" -n "$ACM_NS" \
      -o jsonpath='{.status.installedCSV}')
    if [[ -n $new_csv && $new_csv != "$installed_csv" ]]; then
      break
    fi
    sleep "$POLL_INTERVAL"
    elapsed=$((elapsed + POLL_INTERVAL))
  done
  if [[ -z $new_csv || $new_csv == "$installed_csv" ]]; then
    log_error "ACM did not upgrade. Confirm the new catalog's ${EXPECTED_ACM_CHANNEL} channel replaces ${installed_csv}."
    oc describe subscription "$subscription_name" -n "$ACM_NS" || true
    oc get installplans -n "$ACM_NS" || true
    exit 1
  fi
fi

log_info "ACM CSV selected by OLM: ${new_csv}"
elapsed=0
while ((elapsed < UPGRADE_TIMEOUT)); do
  phase=$(oc get csv "$new_csv" -n "$ACM_NS" -o jsonpath='{.status.phase}' 2>/dev/null || true)
  if [[ $phase == Succeeded ]]; then
    break
  fi
  if [[ $phase == Failed ]]; then
    log_error "ACM CSV ${new_csv} failed"
    oc describe csv "$new_csv" -n "$ACM_NS" || true
    exit 1
  fi
  sleep "$POLL_INTERVAL"
  elapsed=$((elapsed + POLL_INTERVAL))
done
if [[ $phase != Succeeded ]] || ! csv_has_expected_mch_image "$new_csv"; then
  log_error "CSV ${new_csv} did not succeed with the expected MCH operator image"
  exit 1
fi

# Update the same CSV image references that the shared clusterpool deploy step
# overrides. This keeps the PR-built MCO image after OLM replaces the CSV.
csv_json=$(oc get csv "$new_csv" -n "$ACM_NS" -o json)
mco_patch=$(jq -c --arg image "$mco_image" '
  [paths(strings) as $path
   | select($path[0] == "spec")
   | getpath($path) as $current
   | select($current | test("(^|/)multicluster-observability-operator(:|@)"))
   | select($current != $image)
   | {op: "replace",
      path: ("/" + ($path | map(tostring | gsub("~"; "~0") | gsub("/"; "~1")) | join("/"))),
      value: $image}]
' <<<"$csv_json")

if [[ $(jq 'length' <<<"$mco_patch") -gt 0 ]]; then
  log_info "Restoring the PR-built MCO image in CSV ${new_csv}"
  oc patch csv "$new_csv" -n "$ACM_NS" --type=json -p "$mco_patch"
elif ! jq -e --arg image "$mco_image" \
  '.spec | [.. | strings | select(. == $image)] | length > 0' <<<"$csv_json" >/dev/null; then
  log_error "CSV ${new_csv} has no MCO image reference to preserve"
  exit 1
fi

log_info "Waiting for the operator Deployments to use the requested images..."
elapsed=0
while ((elapsed < UPGRADE_TIMEOUT)); do
  mch_images=$(oc get deployment "$MCH_DEPLOYMENT" -n "$ACM_NS" -o json 2>/dev/null |
    jq -r '[.spec.template.spec.containers[]?.image] | join(" ")' || true)
  mco_images=$(oc get deployment "$MCO_DEPLOYMENT" -n "$ACM_NS" -o json 2>/dev/null |
    jq -r '[.spec.template.spec.containers[]?.image] | join(" ")' || true)
  if [[ " ${mch_images} " == *" ${EXPECTED_MCH_OPERATOR_IMAGE} "* && " ${mco_images} " == *" ${mco_image} "* ]]; then
    oc rollout status deployment/"$MCH_DEPLOYMENT" -n "$ACM_NS" --timeout=300s
    oc rollout status deployment/"$MCO_DEPLOYMENT" -n "$ACM_NS" --timeout=300s
    mch_images=$(oc get deployment "$MCH_DEPLOYMENT" -n "$ACM_NS" -o json |
      jq -r '[.spec.template.spec.containers[]?.image] | join(" ")')
    mco_images=$(oc get deployment "$MCO_DEPLOYMENT" -n "$ACM_NS" -o json |
      jq -r '[.spec.template.spec.containers[]?.image] | join(" ")')
    if [[ " ${mch_images} " == *" ${EXPECTED_MCH_OPERATOR_IMAGE} "* && " ${mco_images} " == *" ${mco_image} "* ]]; then
      log_info "ACM upgrade complete: MCH=${EXPECTED_MCH_OPERATOR_IMAGE}, MCO=${mco_image}"
      exit 0
    fi
  fi
  sleep "$POLL_INTERVAL"
  elapsed=$((elapsed + POLL_INTERVAL))
done

log_error "Timed out waiting for MCH=${EXPECTED_MCH_OPERATOR_IMAGE} and MCO=${mco_image}"
log_error "Current MCH images: ${mch_images:-<none>}"
log_error "Current MCO images: ${mco_images:-<none>}"
exit 1
