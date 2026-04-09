#!/usr/bin/env bash
# Enables MCOA user-workload metrics collection end-to-end:
#   1. Patches the MultiClusterObservability CR to enable platform + UWL collection.
#   2. Applies a ScrapeConfig that federates 'up' and kube node/pod metrics.
#   3. Registers the ScrapeConfig in the 'global' placement of the
#      ClusterManagementAddon so that the addon deploys it to managed clusters.
#
# Requires: oc, jq
#
# Usage:
#   ./enable-mcoa-uwl.sh

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib/common.sh"

require_tool jq "Install with: brew install jq"

SCRAPE_CONFIG_NAME="dev-uwl-metrics"
CMA_NAME="multicluster-observability-addon"
PLACEMENT_NAME="global"
SCRAPE_CONFIG_GROUP="monitoring.rhobs"
SCRAPE_CONFIG_RESOURCE="scrapeconfigs"

# Step 1: Enable platform and user-workload metrics collection in the MCO CR.
log_info "Patching MultiClusterObservability to enable platform and UWL metrics collection..."
oc patch multiclusterobservability observability --type=merge -p '{
  "spec": {
    "capabilities": {
      "platform": {
        "metrics": {
          "default": {
            "enabled": true
          }
        }
      },
      "userWorkloads": {
        "metrics": {
          "default": {
            "enabled": true
          }
        }
      }
    }
  }
}'

# Step 2: Apply the ScrapeConfig.
log_info "Applying UWL ScrapeConfig '${SCRAPE_CONFIG_NAME}' in ${MCO_NS}..."
oc apply -f "${SCRIPT_DIR}/manifests/mcoa/uwl-scrape-config.yaml"

# Step 3: Add the ScrapeConfig to the 'global' placement in the ClusterManagementAddon.
# We get the current CMA, merge in the new config entry (deduplicating by name),
# and replace the resource in one shot to avoid an oc edit loop.
log_info "Registering ScrapeConfig in ClusterManagementAddon '${CMA_NAME}' (placement: ${PLACEMENT_NAME})..."
oc get clustermanagementaddon "${CMA_NAME}" -o json |
  jq \
    --arg group "${SCRAPE_CONFIG_GROUP}" \
    --arg resource "${SCRAPE_CONFIG_RESOURCE}" \
    --arg name "${SCRAPE_CONFIG_NAME}" \
    --arg namespace "${MCO_NS}" \
    --arg placement "${PLACEMENT_NAME}" \
    '
      .spec.installStrategy.placements |=
        if type != "array" then
          error("placements is not an array — has the ClusterManagementAddon been fully initialized?")
        elif (map(select(.name == $placement)) | length) == 0 then
          error("placement \"\($placement)\" not found in installStrategy — verify the ClusterManagementAddon is configured correctly")
        else
          map(
            if .name == $placement then
              .configs = (
                (.configs // []) +
                [{
                  "group":     $group,
                  "resource":  $resource,
                  "name":      $name,
                  "namespace": $namespace
                }]
                | unique_by(.name)
              )
            else . end
          )
        end
      ' |
  oc replace -f -

log_info "Done. The PrometheusAgent for user-workload metrics will be deployed to managed clusters."
log_info "  ScrapeConfig: ${SCRAPE_CONFIG_NAME} (namespace: ${MCO_NS})"
log_info "  Metrics collected: 'up', kube_node_info, kube_pod_info"
