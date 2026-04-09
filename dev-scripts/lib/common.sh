#!/usr/bin/env bash
# Shared utilities for dev-scripts.

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m'

log_info() { printf "${GREEN}[INFO]${NC}  %s\n" "$*"; }
log_warn() { printf "${YELLOW}[WARN]${NC}  %s\n" "$*"; }
log_error() { printf "${RED}[ERROR]${NC} %s\n" "$*" >&2; }

# Erase N previously printed lines in-place. No-op when stdout is not a TTY
# (e.g. when output is piped or redirected), so logs remain readable in CI.
_erase_lines() {
  local n="$1"
  [[ -t 1 ]] && [[ $n -gt 0 ]] && printf "\033[%dA\033[J" "$n" || true
}

# Load variables from .env if present, without overwriting variables already set
# in the environment — so CLI overrides (e.g. ACM_VERSION=2.17 ./script.sh) win.
_load_dotenv() {
  local env_file="${SCRIPT_DIR}/.env"
  [[ -f $env_file ]] || return 0
  local line varname
  while IFS= read -r line || [[ -n $line ]]; do
    [[ $line =~ ^[[:space:]]*# ]] && continue # skip comments
    [[ $line =~ ^[[:space:]]*$ ]] && continue # skip blank lines
    varname="${line%%=*}"
    # ${!varname+x} is non-empty iff the variable is already set (bash 3.2+)
    [[ -n ${!varname+x} ]] || export "$line"
  done <"$env_file"
}
_load_dotenv

# Shared namespace constants used across scripts.
# NOTE: YAML manifests under manifests/ hardcode these namespace names and must
# be updated manually if these values ever change.
# shellcheck disable=SC2034  # used by scripts that source this file
MCO_NS="open-cluster-management-observability"
ACM_NS="open-cluster-management"

# Fail fast if required environment variables are not set.
require_env() {
  local missing=0
  for var in "$@"; do
    if [[ -z ${!var:-} ]]; then
      log_error "Required environment variable \$${var} is not set"
      missing=1
    fi
  done
  [[ $missing -eq 0 ]] || exit 1
}

# Fail fast if required CLI tools are not installed.
require_tool() {
  local tool="$1" hint="${2:-}"
  if ! command -v "$tool" &>/dev/null; then
    log_error "'${tool}' is required but not found.${hint:+ ${hint}}"
    exit 1
  fi
}

# Poll until a resource exists, then return.
wait_for_resource() {
  local resource="$1" name="$2" namespace="${3:-}" timeout="${4:-300}"

  # Build the oc command as an array to avoid empty-array issues under set -u.
  local cmd=(oc get "$resource" "$name")
  [[ -n $namespace ]] && cmd+=(-n "$namespace")

  log_info "Waiting for ${resource}/${name} to exist (timeout: ${timeout}s)..."
  local elapsed=0
  until "${cmd[@]}" &>/dev/null; do
    if [[ $elapsed -ge $timeout ]]; then
      log_error "Timed out waiting for ${resource}/${name} after ${timeout}s"
      return 1
    fi
    sleep 5
    elapsed=$((elapsed + 5))
  done
  log_info "${resource}/${name} found."
}

# Poll until a resource is fully gone.
wait_for_deletion() {
  local resource="$1" name="$2" namespace="${3:-}" timeout="${4:-300}"
  local cmd=(oc get "$resource" "$name")
  [[ -n $namespace ]] && cmd+=(-n "$namespace")

  log_info "Waiting for ${resource}/${name} to be deleted (timeout: ${timeout}s)..."
  local elapsed=0
  while "${cmd[@]}" &>/dev/null; do
    if [[ $elapsed -ge $timeout ]]; then
      log_error "Timed out waiting for ${resource}/${name} to be deleted after ${timeout}s"
      return 1
    fi
    sleep 10
    elapsed=$((elapsed + 10))
  done
  log_info "${resource}/${name} deleted."
}

# Poll until no pods remain in a namespace.
wait_for_no_pods_in_namespace() {
  local namespace="$1" timeout="${2:-300}"
  log_info "Waiting for all pods in ${namespace} to terminate (timeout: ${timeout}s)..."
  local elapsed=0
  until [[ "$(oc get pods -n "$namespace" --no-headers 2>/dev/null | wc -l)" -eq 0 ]]; do
    if [[ $elapsed -ge $timeout ]]; then
      log_warn "Timed out waiting for pods in ${namespace} — they may still be terminating"
      return 0
    fi
    sleep 10
    elapsed=$((elapsed + 10))
  done
  log_info "All pods in ${namespace} terminated."
}

# Poll until MultiClusterHub reaches Running phase.
# On each iteration, refreshes the full component list in-place with colored status
# indicators so the display height is stable and there is no need to scroll.
# Requires jq for the component breakdown; falls back to the current phase if absent.
wait_for_mch_running() {
  local timeout="${1:-900}"
  log_info "Waiting for MultiClusterHub to reach Running state (timeout: ${timeout}s)..."
  local elapsed=0 prev_lines=0

  until [[ "$(oc get multiclusterhub multiclusterhub -n "${ACM_NS}" \
    -o jsonpath='{.status.phase}' 2>/dev/null)" == "Running" ]]; do
    if [[ $elapsed -ge $timeout ]]; then
      _erase_lines "$prev_lines"
      log_error "Timed out waiting for MultiClusterHub after ${timeout}s"
      return 1
    fi
    if command -v jq &>/dev/null; then
      local status_msg
      status_msg=$(oc get multiclusterhub multiclusterhub -n "${ACM_NS}" -o json 2>/dev/null |
        jq -r '
          [.status.components // {} | to_entries[] | select(.value.type != "Available" or .value.status != "True") | .key] | sort as $pending |
          ($pending | length) as $len |
          if $len > 0 then
            ($len|tostring) + " pending [" +
            if $len > 3 then
              ($pending[0:3] | join(", ")) + ", ..."
            else
              ($pending | join(", "))
            end + "]"
          else
            "All components available"
          end
        ')
      _erase_lines "$prev_lines"
      if [[ -n $status_msg ]]; then
        log_info "Component status (${elapsed}s elapsed): $status_msg"
        prev_lines=1
      else
        prev_lines=0
      fi
    else
      local phase
      phase=$(oc get multiclusterhub multiclusterhub -n "${ACM_NS}" \
        -o jsonpath='{.status.phase}' 2>/dev/null || true)
      _erase_lines "$prev_lines"
      log_info "Current phase: ${phase:-unknown} (${elapsed}s elapsed)"
      prev_lines=1
    fi
    sleep 15
    elapsed=$((elapsed + 15))
  done
  _erase_lines "$prev_lines"
  log_info "MultiClusterHub is Running."
}

# Poll until MultiClusterHub CR is fully gone, refreshing the full component list
# in-place with colored status indicators so the display height is stable.
# Requires jq for the component breakdown; falls back to the current phase if absent.
wait_for_mch_deleted() {
  local timeout="${1:-600}"
  log_info "Waiting for MultiClusterHub components to be removed (timeout: ${timeout}s)..."
  local elapsed=0 prev_lines=0

  until ! oc get multiclusterhub multiclusterhub -n "${ACM_NS}" &>/dev/null; do
    if [[ $elapsed -ge $timeout ]]; then
      _erase_lines "$prev_lines"
      log_error "Timed out waiting for MultiClusterHub deletion after ${timeout}s"
      return 1
    fi
    if command -v jq &>/dev/null; then
      local status_msg
      status_msg=$(oc get multiclusterhub multiclusterhub -n "${ACM_NS}" -o json 2>/dev/null |
        jq -r '
          [.status.components // {} | to_entries[] | select(.value.status != "Unknown") | .key] | sort as $rem |
          ($rem | length) as $len |
          if $len > 0 then
            ($len|tostring) + " remaining [" +
            if $len > 3 then
              ($rem[0:3] | join(", ")) + ", ..."
            else
              ($rem | join(", "))
            end + "]"
          else
            "0 remaining"
          end
        ')
      _erase_lines "$prev_lines"
      if [[ -n $status_msg ]]; then
        log_info "Removal status (${elapsed}s elapsed): $status_msg"
        prev_lines=1
      else
        prev_lines=0
      fi
    else
      local phase
      phase=$(oc get multiclusterhub multiclusterhub -n "${ACM_NS}" \
        -o jsonpath='{.status.phase}' 2>/dev/null || true)
      _erase_lines "$prev_lines"
      log_info "Current phase: ${phase:-unknown} (${elapsed}s elapsed)"
      prev_lines=1
    fi
    sleep 15
    elapsed=$((elapsed + 15))
  done
  _erase_lines "$prev_lines"
  log_info "MultiClusterHub CR deleted."
}

# Poll until MultiClusterObservability has a Ready=True condition.
wait_for_mco_ready() {
  local timeout="${1:-600}"
  log_info "Waiting for MultiClusterObservability to be Ready (timeout: ${timeout}s)..."
  local elapsed=0
  until [[ "$(oc get multiclusterobservability observability \
    -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null)" == "True" ]]; do
    if [[ $elapsed -ge $timeout ]]; then
      log_error "Timed out waiting for MultiClusterObservability after ${timeout}s"
      return 1
    fi
    sleep 15
    elapsed=$((elapsed + 15))
  done
  log_info "MultiClusterObservability is Ready."
}
