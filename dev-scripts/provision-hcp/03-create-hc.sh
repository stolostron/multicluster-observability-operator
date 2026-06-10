#!/usr/bin/env bash
# Prerequisites: run 01-check-prereqs.sh and 02-aws-setup.sh first
# Executes:
# - Hypershift installation via ACM AddOn
# - HCP and AWS resource provisioning

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
STATE_DIR="${SCRIPT_DIR}/.state"

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

log_ok() { echo -e "  ${GREEN}OK${NC}   $1"; }
log_skip() { echo -e "  ${GREEN}SKIP${NC} $1 (already exists)"; }
log_run() { echo -e "  ${YELLOW}...${NC}  $1"; }
log_err() { echo -e "  ${RED}ERROR${NC} $1"; }
log_info() { echo -e "${YELLOW}==>${NC} $1"; }

source "${SCRIPT_DIR}/env.sh"

HCP_NAMESPACE="clusters-${HC_NAME}"
ROLE_ARN=$(cat "${STATE_DIR}/role-arn" 2>/dev/null || aws iam get-role --role-name hcp-hypershift-role --query 'Role.Arn' --output text 2>/dev/null || echo "")

if [[ -z ${ROLE_ARN} ]]; then
  log_err "IAM role not found. Run 02-aws-setup.sh first."
  exit 1
fi

# ============================================================
# 1. Enable hypershift-addon on hub
# ============================================================
log_info "hypershift-addon on hub (namespace: ${MC_NAME})"

if KUBECONFIG="${HUB_KUBECONFIG}" oc get managedclusteraddon \
  hypershift-addon -n "${MC_NAME}" >/dev/null 2>&1; then
  log_skip "ManagedClusterAddOn hypershift-addon"
else
  log_run "Creating ManagedClusterAddOn"
  KUBECONFIG="${HUB_KUBECONFIG}" oc apply -f - <<EOF
apiVersion: addon.open-cluster-management.io/v1alpha1
kind: ManagedClusterAddOn
metadata:
  name: hypershift-addon
  namespace: ${MC_NAME}
spec:
  installNamespace: open-cluster-management-agent-addon
  configs:
  - group: addon.open-cluster-management.io
    resource: addondeploymentconfigs
    name: hypershift-operator-oidc-provider-s3
    namespace: ${MC_NAME}
EOF
  log_ok "ManagedClusterAddOn created"
fi

log_run "Waiting for addon to become available (timeout 3m)"
for i in $(seq 1 18); do
  AVAILABLE=$(KUBECONFIG="${HUB_KUBECONFIG}" oc get managedclusteraddon \
    hypershift-addon -n "${MC_NAME}" \
    -o jsonpath='{.status.conditions[?(@.type=="Available")].status}' 2>/dev/null || echo "")
  if [[ ${AVAILABLE} == "True" ]]; then
    log_ok "Addon is available"
    break
  fi
  if [[ $i -eq 18 ]]; then
    log_err "Addon not available after 3 minutes"
    KUBECONFIG="${HUB_KUBECONFIG}" oc get managedclusteraddon -n "${MC_NAME}" hypershift-addon
    exit 1
  fi
  sleep 10
done

# ============================================================
# 2. Verify operator has been deployed on spoke
# ============================================================
log_info "HyperShift operator on spoke"

log_run "Waiting for operator pods to be ready (timeout 3m)"
for i in $(seq 1 18); do
  READY_PODS=$(KUBECONFIG="${SPOKE_KUBECONFIG}" oc get deploy -n hypershift operator \
    -o jsonpath='{.status.readyReplicas}' 2>/dev/null || echo "0")
  READY_PODS=${READY_PODS:-0}
  if [[ ${READY_PODS} -ge 2 ]]; then
    log_ok "${READY_PODS} operator pods ready"
    break
  fi
  if [[ $i -eq 18 ]]; then
    log_err "Operator pods not ready after 3 minutes"
    KUBECONFIG="${SPOKE_KUBECONFIG}" oc get pods -n hypershift
    exit 1
  fi
  sleep 10
done

OIDC_ARGS=$(KUBECONFIG="${SPOKE_KUBECONFIG}" oc get deploy -n hypershift operator \
  -o jsonpath='{.spec.template.spec.containers[0].args}' 2>/dev/null || echo "")
if echo "${OIDC_ARGS}" | grep -q "oidc-storage-provider-s3-bucket-name"; then
  log_ok "Operator has OIDC S3 args configured"
else
  log_err "Operator missing OIDC S3 args — check AddOnDeploymentConfig on hub"
  exit 1
fi

# ============================================================
# 3. Enable wildcard routes
# ============================================================
log_info "Wildcard routes on spoke"

KUBECONFIG="${SPOKE_KUBECONFIG}" oc patch ingresscontroller -n openshift-ingress-operator default \
  --type=merge -p '{"spec":{"routeAdmission":{"wildcardPolicy":"WildcardsAllowed"}}}' 2>/dev/null || true
log_ok "Wildcard routes enabled"

# ============================================================
# 4. Check if HC already exists
# ============================================================
log_info "Checking for existing HostedCluster ${HC_NAME}"

if KUBECONFIG="${SPOKE_KUBECONFIG}" oc get hostedcluster "${HC_NAME}" \
  -n "${HC_NAMESPACE}" >/dev/null 2>&1; then
  HC_AVAILABLE=$(KUBECONFIG="${SPOKE_KUBECONFIG}" oc get hostedcluster "${HC_NAME}" \
    -n "${HC_NAMESPACE}" -o jsonpath='{.status.conditions[?(@.type=="Available")].status}' 2>/dev/null || echo "")
  if [[ ${HC_AVAILABLE} == "True" ]]; then
    log_skip "HostedCluster ${HC_NAME} (already available)"
    KUBECONFIG="${SPOKE_KUBECONFIG}" oc get hostedcluster -n "${HC_NAMESPACE}" "${HC_NAME}"
  else
    echo -e "  ${YELLOW}WAIT${NC} HostedCluster ${HC_NAME} exists but not yet available — skipping to wait step"
  fi
else
  # ============================================================
  # 5. Determine release version
  # ============================================================
  if [[ -z ${RELEASE_VERSION:-} ]]; then
    log_info "Determining release version"
    SUPPORTED_OCP=$(KUBECONFIG="${SPOKE_KUBECONFIG}" oc logs -n hypershift -l app=operator \
      --tail=200 2>/dev/null | grep -o "Latest supported OCP: [0-9.]*" | head -1 | awk '{print $NF}')
    if [[ -z ${SUPPORTED_OCP} ]]; then
      log_err "Could not determine supported OCP version from operator logs"
      exit 1
    fi
    OCP_MAJOR_MINOR=$(echo "${SUPPORTED_OCP}" | grep -o '^[0-9]*\.[0-9]*')
    RELEASE_VERSION=$(curl -s "https://mirror.openshift.com/pub/openshift-v4/clients/ocp/stable-${OCP_MAJOR_MINOR}/release.txt" |
      grep "Name:" | awk '{print $2}')
    if [[ -z ${RELEASE_VERSION} ]]; then
      log_err "Could not determine latest stable patch for ${OCP_MAJOR_MINOR}"
      exit 1
    fi
    log_ok "Release version: ${RELEASE_VERSION} (auto-detected)"
  else
    log_ok "Release version: ${RELEASE_VERSION} (from env.sh)"
  fi

  # ============================================================
  # 6. Generate STS session token
  # ============================================================
  log_info "Generating STS session token"
  aws sts get-session-token --output json >/tmp/sts-creds.json
  log_ok "STS token saved to /tmp/sts-creds.json"

  # ============================================================
  # 7. Render + apply
  # ============================================================
  log_info "Rendering HostedCluster manifests"

  if [[ ! -f ${PULL_SECRET_FILE} ]]; then
    log_err "Pull secret not found: ${PULL_SECRET_FILE}"
    exit 1
  fi
  if [[ ! -f ${SSH_KEY_FILE} ]]; then
    log_err "SSH key not found: ${SSH_KEY_FILE}"
    exit 1
  fi

  log_run "Running hcp create cluster aws --render (creates AWS infra)"
  RENDER_LOG="/tmp/hcp-render-${HC_NAME}.log"

  if KUBECONFIG="${SPOKE_KUBECONFIG}" hcp create cluster aws \
    --name="${HC_NAME}" \
    --namespace="${HC_NAMESPACE}" \
    --region="${AWS_REGION}" \
    --release-image="quay.io/openshift-release-dev/ocp-release:${RELEASE_VERSION}-x86_64" \
    --pull-secret="${PULL_SECRET_FILE}" \
    --ssh-key="${SSH_KEY_FILE}" \
    --node-pool-replicas="${NODE_POOL_REPLICAS:-2}" \
    --instance-type="${INSTANCE_TYPE:-m6i.xlarge}" \
    --base-domain="${BASE_DOMAIN}" \
    --control-plane-availability-policy="${AVAILABILITY_POLICY:-SingleReplica}" \
    --infra-availability-policy="${AVAILABILITY_POLICY:-SingleReplica}" \
    --sts-creds=/tmp/sts-creds.json \
    --role-arn="${ROLE_ARN}" \
    --render --render-sensitive >/tmp/hc-rendered.yaml 2> >(tee "${RENDER_LOG}" >&2); then
    log_ok "Rendered to /tmp/hc-rendered.yaml"
  else
    log_err "Render failed. Check output above for details."
    FAILED_INFRA_ID=$(grep '"Creating infrastructure"' "${RENDER_LOG}" 2>/dev/null | grep -o '"id":"[^"]*"' | cut -d'"' -f4) || true
    if [[ -n ${FAILED_INFRA_ID} ]]; then
      echo "${FAILED_INFRA_ID}" >"${STATE_DIR}/infra-id"
      log_err "AWS resources were created with infra ID: ${FAILED_INFRA_ID}"
    fi
    log_err "Run ./04-destroy-hc.sh to clean up orphaned AWS resources."
    rm -f "${RENDER_LOG}"
    exit 1
  fi
  rm -f "${RENDER_LOG}"

  if [[ ! -s /tmp/hc-rendered.yaml ]]; then
    log_err "Rendered file is empty — render produced no output."
    exit 1
  fi

  log_info "Applying HostedCluster manifest"
  KUBECONFIG="${SPOKE_KUBECONFIG}" oc apply -f /tmp/hc-rendered.yaml
  log_ok "Applied"
fi

# ============================================================
# 8. Wait for availability
# ============================================================
log_info "Waiting for HostedCluster to become available (timeout 30m)"

for i in $(seq 1 180); do
  HC_AVAILABLE=$(KUBECONFIG="${SPOKE_KUBECONFIG}" oc get hostedcluster "${HC_NAME}" \
    -n "${HC_NAMESPACE}" -o jsonpath='{.status.conditions[?(@.type=="Available")].status}' 2>/dev/null || echo "")
  HC_MSG=$(KUBECONFIG="${SPOKE_KUBECONFIG}" oc get hostedcluster "${HC_NAME}" \
    -n "${HC_NAMESPACE}" -o jsonpath='{.status.conditions[?(@.type=="Available")].message}' 2>/dev/null || echo "")

  if [[ ${HC_AVAILABLE} == "True" ]]; then
    log_ok "HostedCluster is available: ${HC_MSG}"
    break
  fi

  if [[ $((i % 6)) -eq 0 ]]; then
    echo -e "  ${YELLOW}...${NC}  [$((i * 10 / 60))m] ${HC_MSG:-waiting...}"
  fi

  if [[ $i -eq 180 ]]; then
    log_err "HostedCluster not available after 30 minutes"
    KUBECONFIG="${SPOKE_KUBECONFIG}" oc get hostedcluster -n "${HC_NAMESPACE}" "${HC_NAME}"
    exit 1
  fi
  sleep 10
done

# Wait for nodes
log_info "Waiting for worker nodes"
DESIRED=$(KUBECONFIG="${SPOKE_KUBECONFIG}" oc get nodepool -n "${HC_NAMESPACE}" \
  -o jsonpath='{.items[0].spec.replicas}' 2>/dev/null || echo "${NODE_POOL_REPLICAS:-2}")

for i in $(seq 1 60); do
  CURRENT=$(KUBECONFIG="${SPOKE_KUBECONFIG}" oc get nodepool -n "${HC_NAMESPACE}" \
    -o jsonpath='{.items[0].status.replicas}' 2>/dev/null || echo "0")
  if [[ ${CURRENT} -ge ${DESIRED} ]]; then
    log_ok "${CURRENT}/${DESIRED} nodes ready"
    break
  fi
  if [[ $i -eq 60 ]]; then
    echo -e "  ${YELLOW}WARN${NC} Nodes ${CURRENT}/${DESIRED} after 10 minutes — continuing anyway"
    break
  fi
  sleep 10
done

# Extract kubeconfig
log_info "Extracting admin kubeconfig"
HC_KUBECONFIG="/tmp/${HC_NAME}-kubeconfig"
KUBECONFIG="${SPOKE_KUBECONFIG}" oc extract "secret/${HC_NAME}-admin-kubeconfig" \
  -n "${HC_NAMESPACE}" --to=- >"${HC_KUBECONFIG}" 2>/dev/null
echo "${HC_KUBECONFIG}" >"${STATE_DIR}/hc-kubeconfig"
log_ok "Kubeconfig saved to ${HC_KUBECONFIG}"

# ============================================================
# Summary
# ============================================================
echo ""
echo "=============================="
echo -e "${GREEN}Hosted cluster ${HC_NAME} is ready${NC}"
echo "=============================="
echo ""
echo "  Destroy:"
echo "    ./04-destroy-hc.sh"
