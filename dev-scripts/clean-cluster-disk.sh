#!/usr/bin/env bash
# Cleans up accumulated disk space on OpenShift nodes (especially SNO / collective dev clusters).
#
# Common causes addressed:
#   1. openshift-console/downloads pod: leaks ~3.1 GB of oc binary archives in emptyDir (/tmp)
#      on each container restart.
#   2. Evicted & ContainerStatusUnknown pods: leftover pod objects whose container layers and
#      logs cannot be collected until the pod is deleted.
#   3. Old completed pods: installer and insights-gathering pods holding logs in /var/log/pods.
#   4. Systemd journal: vacuumed down to 500MB via machine-config-daemon.
#   5. CRI-O images: unused images pruned via crictl rmi --prune.
#
# Usage:
#   ./clean-cluster-disk.sh [--dry-run]

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib/common.sh"

require_tool oc

DRY_RUN=false
for arg in "$@"; do
  case "$arg" in
    --dry-run)
      DRY_RUN=true
      shift
      ;;
    -h | --help)
      echo "Usage: $0 [--dry-run]"
      echo "Reclaims disk space on OpenShift nodes by clearing known leak sources, dead pods, logs, and unused images."
      exit 0
      ;;
    *)
      log_error "Unknown argument: $arg"
      exit 1
      ;;
  esac
done

if [[ $DRY_RUN == "true" ]]; then
  log_warn "Running in DRY-RUN mode — no resources will be deleted or modified."
fi

log_info "Connecting to cluster..."
oc whoami &>/dev/null || {
  log_error "Not logged into an OpenShift cluster. Run 'oc login' first."
  exit 1
}

# -----------------------------------------------------------------------------
# 1. Inspect Node Disk Status
# -----------------------------------------------------------------------------
log_info "--- Node Status & Disk Usage (Before) ---"
oc get nodes -o custom-columns='NAME:.metadata.name,STATUS:.status.conditions[?(@.type=="Ready")].status,DISK_PRESSURE:.status.conditions[?(@.type=="DiskPressure")].status,TAINTS:.spec.taints[*].key'

get_mcd_pod_for_node() {
  local node="$1"
  oc get pods -n openshift-machine-config-operator -l k8s-app=machine-config-daemon \
    --field-selector "spec.nodeName=${node},status.phase=Running" \
    -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true
}

exec_on_node() {
  local node="$1"
  shift
  local mcd_pod
  mcd_pod=$(get_mcd_pod_for_node "$node")
  if [[ -n $mcd_pod ]]; then
    oc exec -n openshift-machine-config-operator "$mcd_pod" -c machine-config-daemon -- chroot /rootfs "$@"
  else
    log_warn "No running machine-config-daemon found on node ${node}; skipping host command."
    return 1
  fi
}

nodes=$(oc get nodes -o jsonpath='{.items[*].metadata.name}')

for node in $nodes; do
  log_info "Disk usage on ${node}:"
  exec_on_node "$node" df -h /var 2>/dev/null || true
done

# -----------------------------------------------------------------------------
# 2. Recycle openshift-console/downloads Pods
# -----------------------------------------------------------------------------
log_info "--- Checking openshift-console/downloads pod ---"
downloads_pods=$(oc get pods -n openshift-console -l app=console,component=downloads -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.status.containerStatuses[0].restartCount}{"\n"}{end}' 2>/dev/null || true)

if [[ -n $downloads_pods ]]; then
  while IFS=$'\t' read -r pod restarts; do
    [[ -z $pod ]] && continue
    log_info "Found downloads pod: ${pod} (container restarts: ${restarts:-0})"
    if [[ ${restarts:-0} -gt 0 ]]; then
      log_warn "Pod ${pod} has restarted ${restarts} time(s). Each restart leaks ~3.1 GB of archives in emptyDir (/tmp)."
      if [[ $DRY_RUN == "true" ]]; then
        log_info "[DRY-RUN] Would delete pod ${pod} in openshift-console to reclaim emptyDir space"
      else
        log_info "Deleting pod ${pod} in openshift-console (deployment will immediately recreate a clean pod)..."
        oc delete pod "$pod" -n openshift-console --wait=false
      fi
    else
      log_info "Downloads pod ${pod} has 0 restarts (emptyDir clean)."
    fi
  done <<<"$downloads_pods"
else
  log_info "No console downloads pod found in openshift-console."
fi

# -----------------------------------------------------------------------------
# 3. Purge Evicted and ContainerStatusUnknown Pods
# -----------------------------------------------------------------------------
log_info "--- Checking for Evicted & ContainerStatusUnknown pods ---"
if command -v jq &>/dev/null; then
  pods_json=$(oc get pods -A -o json 2>/dev/null || true)
  if [[ -z $pods_json ]]; then
    log_warn "Failed to list pods cluster-wide or no pods returned."
    dead_pods=""
  else
    dead_pods=$(echo "$pods_json" | jq -r '
      .items[] |
      select(
        (.status.reason? == "Evicted") or
        (.status.containerStatuses[]?.state.waiting?.reason? == "ContainerStatusUnknown")
      ) |
      "\(.metadata.namespace)\t\(.metadata.name)\t\(.status.reason // "ContainerStatusUnknown")"
    ' || true)
  fi
else
  # Fallback when jq is not available: filter specifically for Evicted pods
  dead_pods=$(oc get pods -A -o jsonpath='{range .items[?(@.status.reason=="Evicted")]}{.metadata.namespace}{"\t"}{.metadata.name}{"\tEvicted\n"}{end}' 2>/dev/null || true)
fi

dead_pods=$(echo "$dead_pods" | sed '/^[[:space:]]*$/d' | sort -u || true)

if [[ -n $dead_pods ]]; then
  count=$(echo "$dead_pods" | wc -l | tr -d ' ')
  log_warn "Found ${count} dead/evicted pod(s) cluster-wide."
  while IFS=$'\t' read -r ns name reason; do
    [[ -z $ns || -z $name ]] && continue
    if [[ $DRY_RUN == "true" ]]; then
      log_info "[DRY-RUN] Would delete ${ns}/${name} (reason: ${reason:-Failed})"
    else
      log_info "Deleting ${ns}/${name} (${reason:-Failed})..."
      oc delete pod "$name" -n "$ns" --ignore-not-found --wait=false &>/dev/null || true
    fi
  done <<<"$dead_pods"
else
  log_info "No dead or evicted pods found."
fi

# -----------------------------------------------------------------------------
# 4. Clean Completed Installer & Gather Pods
# -----------------------------------------------------------------------------
log_info "--- Cleaning completed installer & gather pods ---"
completed_pods=$(oc get pods -A --field-selector 'status.phase=Succeeded' -o jsonpath='{range .items[*]}{.metadata.namespace}{"\t"}{.metadata.name}{"\n"}{end}' 2>/dev/null || true)

if [[ -n $completed_pods ]]; then
  while IFS=$'\t' read -r ns name; do
    [[ -z $ns || -z $name ]] && continue
    # Target openshift-kube-apiserver, openshift-insights, and marketplace completed jobs/pods
    case "$ns" in
      openshift-insights | openshift-kube-apiserver | openshift-marketplace)
        if [[ $DRY_RUN == "true" ]]; then
          log_info "[DRY-RUN] Would delete completed pod ${ns}/${name}"
        else
          log_info "Deleting completed pod ${ns}/${name}..."
          oc delete pod "$name" -n "$ns" --ignore-not-found --wait=false &>/dev/null || true
        fi
        ;;
    esac
  done <<<"$completed_pods"
else
  log_info "No succeeded pods found in target namespaces."
fi

# -----------------------------------------------------------------------------
# 5. Clean Host Journals & Prune Unused Images
# -----------------------------------------------------------------------------
log_info "--- Vacuuming Systemd Journal & Pruning Unused Images ---"
for node in $nodes; do
  mcd_pod=$(get_mcd_pod_for_node "$node")
  if [[ -z $mcd_pod ]]; then
    log_warn "Skipping node-level journal/CRI-O prune for ${node} (no machine-config-daemon running)."
    continue
  fi

  if [[ $DRY_RUN == "true" ]]; then
    log_info "[DRY-RUN] Would vacuum journal to 500M on node ${node}"
    log_info "[DRY-RUN] Would run 'crictl rmi --prune' on node ${node}"
  else
    log_info "Vacuuming journal to 500M on node ${node}..."
    exec_on_node "$node" journalctl --rotate --vacuum-size=500M 2>&1 | tail -n 2 || true

    log_info "Pruning unused images via crictl on node ${node}..."
    exec_on_node "$node" crictl rmi --prune 2>&1 || true
  fi
done

# -----------------------------------------------------------------------------
# 6. Final Status & Verification
# -----------------------------------------------------------------------------
log_info "--- Node Status & Disk Usage (After) ---"
oc get nodes -o custom-columns='NAME:.metadata.name,STATUS:.status.conditions[?(@.type=="Ready")].status,DISK_PRESSURE:.status.conditions[?(@.type=="DiskPressure")].status,TAINTS:.spec.taints[*].key'

for node in $nodes; do
  log_info "Disk usage on ${node}:"
  exec_on_node "$node" df -h /var 2>/dev/null || true
done

log_info "Cleanup complete."
