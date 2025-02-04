#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'

# Constants
readonly OBS_NAMESPACE="open-cluster-management-observability"
readonly GRAFANA_POD_LABEL="app=multicluster-observability-grafana-dev"
readonly TEST_USER="test"

# Helper functions
log() {
  echo "[$(date +'%Y-%m-%d %H:%M:%S')] $*"
}

fail() {
  log "ERROR: $*"
  exit 1
}

wait_for_condition() {
  local cmd="$1"
  local desc="$2"
  local max_attempts="$3"
  local interval="${4:-5}"

  for ((i = 1; i <= max_attempts; i++)); do
    if eval "$cmd"; then
      return 0
    fi
    log "Attempt $i/$max_attempts: Waiting for $desc... Retrying in ${interval}s"
    sleep "$interval"
  done

  fail "Timeout waiting for $desc after $max_attempts attempts"
}

# Ensure working directory
BASE_DIR="$(cd "$(dirname "$0")/.." && pwd -P)"
cd "$BASE_DIR/tools" || fail "Failed to change to tools directory"

# Deploy Grafana
log "Deploying Grafana dev environment"
./setup-grafana-dev.sh --deploy || fail "Failed to deploy Grafana dev environment"

# Wait for Grafana pod
kubectl wait --for=condition=Ready --timeout=120s pods -n "$OBS_NAMESPACE" -l "$GRAFANA_POD_LABEL"

# Get pod name
POD_NAME=$(kubectl get pods -n "$OBS_NAMESPACE" -l "$GRAFANA_POD_LABEL" \
  --template '{{range .items}}{{.metadata.name}}{{"\n"}}{{end}}') || fail "Failed to get Grafana pod name"
[[ -z $POD_NAME ]] && fail "Grafana pod name is empty"

log "Creating test user"
create_test_user_cmd="
  kubectl -n \"$OBS_NAMESPACE\" exec \"$POD_NAME\" -c grafana-dashboard-loader -- /usr/bin/curl \
    -XPOST -s \
    -H \"Content-Type: application/json\" \
    -H \"X-Forwarded-User: WHAT_YOU_ARE_DOING_IS_VOIDING_SUPPORT_0000000000000000000000000000000000000000000000000000000000000000\" \
    -d '{ \"name\":\"$TEST_USER\", \"email\":\"$TEST_USER\", \"login\":\"$TEST_USER\", \"password\":\"$TEST_USER\" }' \
    '127.0.0.1:3001/api/admin/users'
"

wait_for_condition \
  "$create_test_user_cmd" \
  "Creating test user" \
  10 \
  2

# Switch to admin and generate dashboard
wait_for_condition \
  "./switch-to-grafana-admin.sh \"$TEST_USER\"" \
  "switching to Grafana admin" \
  10 \
  2

wait_for_condition \
  "./generate-dashboard-configmap-yaml.sh -f 'Alerts' 'Alert Analysis'" \
  "generating dashboard configmap" \
  10 \
  2

# Cleanup
log "Cleaning up Grafana dev environment"
./setup-grafana-dev.sh --clean || fail "Failed to clean up Grafana dev environment"

log "Script completed successfully"
