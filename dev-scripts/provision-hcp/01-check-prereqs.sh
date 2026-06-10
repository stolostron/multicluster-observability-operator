#!/usr/bin/env bash
# Check:
# - Required environment variables are set
# - Required tools are installed and in PATH
# - Required files (pull secret, SSH key) exist and are valid
# - Clusters access
# - AWS credentials

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

PASS=0
FAIL=0

log_pass() {
  echo -e "  ${GREEN}PASS${NC} $1"
  ((PASS++)) || true
}
log_fail() {
  echo -e "  ${RED}FAIL${NC} $1"
  ((FAIL++)) || true
}
log_info() { echo -e "${YELLOW}==>${NC} $1"; }

# --- Source env.sh ---
if [[ ! -f "${SCRIPT_DIR}/env.sh" ]]; then
  echo -e "${RED}ERROR:${NC} env.sh not found. Run: cp env.sh.example env.sh"
  exit 1
fi
source "${SCRIPT_DIR}/env.sh"

# --- 1. Required environment variables ---
log_info "Checking environment variables"

REQUIRED_VARS=(
  HUB_KUBECONFIG
  SPOKE_KUBECONFIG
  MC_NAME
  HC_NAME
  HC_NAMESPACE
  AWS_REGION
  BASE_DOMAIN
  PARENT_DOMAIN
  PULL_SECRET_FILE
  SSH_KEY_FILE
)

for var in "${REQUIRED_VARS[@]}"; do
  if [[ -z ${!var:-} ]]; then
    log_fail "$var is not set"
  else
    log_pass "$var=${!var}"
  fi
done

# --- 2. Required tools ---
log_info "Checking required tools"

REQUIRED_TOOLS=(oc aws hcp jq dig timeout)

for tool in "${REQUIRED_TOOLS[@]}"; do
  if command -v "$tool" &>/dev/null; then
    log_pass "$tool found: $(command -v "$tool")"
  else
    log_fail "$tool not found in PATH"
  fi
done

# --- 3. Files ---
log_info "Checking files"

if [[ -f ${PULL_SECRET_FILE} ]]; then
  if jq empty "${PULL_SECRET_FILE}" 2>/dev/null; then
    log_pass "Pull secret exists and is valid JSON: ${PULL_SECRET_FILE}"
  else
    log_fail "Pull secret exists but is not valid JSON: ${PULL_SECRET_FILE}"
  fi
else
  log_fail "Pull secret not found: ${PULL_SECRET_FILE}"
fi

if [[ -f ${SSH_KEY_FILE} ]]; then
  log_pass "SSH key exists: ${SSH_KEY_FILE}"
else
  log_fail "SSH key not found: ${SSH_KEY_FILE}"
fi

# --- 4. Hub cluster access ---
log_info "Checking hub cluster access"

if [[ ! -f ${HUB_KUBECONFIG} ]]; then
  log_fail "Hub kubeconfig not found: ${HUB_KUBECONFIG}"
else
  HUB_USER=$(KUBECONFIG="${HUB_KUBECONFIG}" oc whoami 2>/dev/null) &&
    log_pass "Hub access: logged in as ${HUB_USER}" ||
    log_fail "Hub access: cannot authenticate (check HUB_KUBECONFIG)"

  HUB_SERVER=$(KUBECONFIG="${HUB_KUBECONFIG}" oc whoami --show-server 2>/dev/null) &&
    log_pass "Hub server: ${HUB_SERVER}" || true

  MC_STATUS=$(KUBECONFIG="${HUB_KUBECONFIG}" oc get managedcluster "${MC_NAME}" -o jsonpath='{.status.conditions[?(@.type=="ManagedClusterConditionAvailable")].status}' 2>/dev/null) || true
  if [[ ${MC_STATUS} == "True" ]]; then
    log_pass "ManagedCluster ${MC_NAME} is available"
  else
    log_fail "ManagedCluster ${MC_NAME} not found or not available"
  fi
fi

# --- 5. Spoke cluster access ---
log_info "Checking spoke cluster access"

if [[ ! -f ${SPOKE_KUBECONFIG} ]]; then
  log_fail "Spoke kubeconfig not found: ${SPOKE_KUBECONFIG}"
else
  SPOKE_USER=$(KUBECONFIG="${SPOKE_KUBECONFIG}" oc whoami 2>/dev/null) &&
    log_pass "Spoke access: logged in as ${SPOKE_USER}" ||
    log_fail "Spoke access: cannot authenticate (check SPOKE_KUBECONFIG)"

  SPOKE_SERVER=$(KUBECONFIG="${SPOKE_KUBECONFIG}" oc whoami --show-server 2>/dev/null) &&
    log_pass "Spoke server: ${SPOKE_SERVER}" || true

  SPOKE_NODES=$(KUBECONFIG="${SPOKE_KUBECONFIG}" oc get nodes --no-headers 2>/dev/null | wc -l) &&
    log_pass "Spoke has ${SPOKE_NODES} node(s)" ||
    log_fail "Cannot list spoke nodes"
fi

# --- 6. AWS credentials ---
log_info "Checking AWS credentials"

if AWS_IDENTITY=$(aws sts get-caller-identity --output json 2>/dev/null); then
  AWS_ACCOUNT=$(echo "${AWS_IDENTITY}" | jq -r '.Account')
  AWS_ARN=$(echo "${AWS_IDENTITY}" | jq -r '.Arn')
  log_pass "AWS identity: ${AWS_ARN} (account ${AWS_ACCOUNT})"
else
  log_fail "AWS credentials not configured or invalid"
fi

if aws s3api list-buckets --query 'Buckets[0].Name' --output text &>/dev/null; then
  log_pass "AWS S3 access works"
else
  log_fail "AWS S3 access failed (check IAM permissions)"
fi

if aws route53 list-hosted-zones --query 'HostedZones[0].Name' --output text &>/dev/null; then
  log_pass "AWS Route53 access works"
else
  log_fail "AWS Route53 access failed (check IAM permissions)"
fi

# --- Summary ---
echo ""
echo "=============================="
echo -e "  ${GREEN}PASSED: ${PASS}${NC}    ${RED}FAILED: ${FAIL}${NC}"
echo "=============================="

if [[ ${FAIL} -gt 0 ]]; then
  echo -e "\n${RED}Fix the failures above before proceeding.${NC}"
  exit 1
else
  echo -e "\n${GREEN}All checks passed. Ready to run 02-aws-setup.sh${NC}"
fi
