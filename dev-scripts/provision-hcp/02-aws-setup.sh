#!/usr/bin/env bash
# Setup AWS resources for Hosted Clusters.
# Each function is idempontent: if the resource already exists, it is skipped
# So the script can safely be re-run.
# Creates:
# - Route53 hosted zone for the base domain, with delegation from the parent domain
# - S3 bucket for OIDC storage, with public read access
# - ConfigMap on the spoke with OIDC bucket info
# - Secret + AddOnDeploymentConfig on the hub for OIDC credentials
# - IAM role with permissions for Hypershift

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
STATE_DIR="${SCRIPT_DIR}/.state"

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

log_ok() { echo -e "  ${GREEN}OK${NC}   $1"; }
log_skip() { echo -e "  ${GREEN}SKIP${NC} $1 (already exists)"; }
log_run() { echo -e "  ${YELLOW}CREATE${NC} $1"; }
log_err() { echo -e "  ${RED}ERROR${NC} $1"; }
log_info() { echo -e "${YELLOW}==>${NC} $1"; }

source "${SCRIPT_DIR}/env.sh"
mkdir -p "${STATE_DIR}"

OIDC_BUCKET="hcp-oidc-${USER}-${HC_NAME}"

# ============================================================
# 1. Route53 sub-zone
# ============================================================
log_info "Route53 sub-zone: ${BASE_DOMAIN}"

SUBZONE_ID=$(aws route53 list-hosted-zones \
  --query "HostedZones[?Name=='${BASE_DOMAIN}.'].Id" --output text 2>/dev/null | head -1)

if [[ -n ${SUBZONE_ID} && ${SUBZONE_ID} != "None" ]]; then
  log_skip "Zone ${BASE_DOMAIN} (${SUBZONE_ID})"
else
  log_run "Creating zone ${BASE_DOMAIN}"
  SUBZONE_ID=$(aws route53 create-hosted-zone \
    --name "${BASE_DOMAIN}" \
    --caller-reference "hcp-$(date +%s)" \
    --query 'HostedZone.Id' --output text)
  log_ok "Created zone ${SUBZONE_ID}"
fi
echo "${SUBZONE_ID}" >"${STATE_DIR}/subzone-id"

# Get NS records
NS_RECORDS=$(aws route53 get-hosted-zone --id "${SUBZONE_ID}" \
  --query 'DelegationSet.NameServers' --output json)
log_ok "NS records: $(echo "${NS_RECORDS}" | jq -r 'join(", ")')"

# NS delegation in parent
log_info "NS delegation in parent zone: ${PARENT_DOMAIN}"

PARENT_ZONE_ID=$(aws route53 list-hosted-zones \
  --query "HostedZones[?Name=='${PARENT_DOMAIN}.'].Id" --output text 2>/dev/null | head -1)

if [[ -z ${PARENT_ZONE_ID} || ${PARENT_ZONE_ID} == "None" ]]; then
  log_err "Parent zone ${PARENT_DOMAIN} not found"
  exit 1
fi

log_run "Upserting NS delegation for ${BASE_DOMAIN} in ${PARENT_DOMAIN}"
NS_RRS=$(echo "${NS_RECORDS}" | jq '[.[] | {"Value": .}]')
aws route53 change-resource-record-sets --hosted-zone-id "${PARENT_ZONE_ID}" --change-batch '{
  "Changes": [{
    "Action": "UPSERT",
    "ResourceRecordSet": {
      "Name": "'"${BASE_DOMAIN}"'",
      "Type": "NS",
      "TTL": 300,
      "ResourceRecords": '"${NS_RRS}"'
    }
  }]
}' >/dev/null
log_ok "NS delegation set"

# Verify
NS_CHECK=$(dig +short NS "${BASE_DOMAIN}" 2>/dev/null | head -1)
if [[ -n ${NS_CHECK} ]]; then
  log_ok "DNS delegation verified: ${NS_CHECK} ..."
else
  echo -e "  ${YELLOW}WARN${NC} DNS delegation not yet resolving (may take a few minutes)"
fi

# ============================================================
# 2. OIDC S3 bucket
# ============================================================
log_info "OIDC S3 bucket: ${OIDC_BUCKET}"

if aws s3api head-bucket --bucket "${OIDC_BUCKET}" >/dev/null 2>&1; then
  log_skip "Bucket ${OIDC_BUCKET}"
else
  log_run "Creating bucket ${OIDC_BUCKET}"
  aws s3api create-bucket --bucket "${OIDC_BUCKET}" --region "${AWS_REGION}" >/dev/null
  log_ok "Bucket created"
fi
echo "${OIDC_BUCKET}" >"${STATE_DIR}/oidc-bucket"

log_run "Configuring public access (idempotent)"
aws s3api put-public-access-block --bucket "${OIDC_BUCKET}" \
  --public-access-block-configuration \
  "BlockPublicAcls=false,IgnorePublicAcls=false,BlockPublicPolicy=false,RestrictPublicBuckets=false"

aws s3api put-bucket-policy --bucket "${OIDC_BUCKET}" --policy '{
  "Version": "2012-10-17",
  "Statement": [{
    "Sid": "AllowPublicRead",
    "Effect": "Allow",
    "Principal": "*",
    "Action": "s3:GetObject",
    "Resource": "arn:aws:s3:::'"${OIDC_BUCKET}"'/*"
  }]
}'
log_ok "Bucket policy set"

# ============================================================
# 3. OIDC ConfigMap on spoke
# ============================================================
log_info "OIDC ConfigMap on spoke (kube-public)"

KUBECONFIG="${SPOKE_KUBECONFIG}" oc apply -f - <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: oidc-storage-provider-s3-config
  namespace: kube-public
data:
  name: ${OIDC_BUCKET}
  region: ${AWS_REGION}
EOF
log_ok "ConfigMap applied"

# ============================================================
# 4. OIDC secret + AddOnDeploymentConfig on hub
# ============================================================
log_info "OIDC credentials on hub (namespace: ${MC_NAME})"

if KUBECONFIG="${HUB_KUBECONFIG}" oc get secret \
  hypershift-operator-oidc-provider-s3-credentials \
  -n "${MC_NAME}" >/dev/null 2>&1; then
  log_skip "OIDC credentials secret in ${MC_NAME}"
else
  log_run "Creating OIDC credentials secret"
  TMPFILE=$(mktemp)
  cat >"${TMPFILE}" <<CREDS
[default]
aws_access_key_id = $(aws configure get aws_access_key_id)
aws_secret_access_key = $(aws configure get aws_secret_access_key)
CREDS
  KUBECONFIG="${HUB_KUBECONFIG}" oc create secret generic \
    hypershift-operator-oidc-provider-s3-credentials \
    --from-file=credentials="${TMPFILE}" \
    --from-literal=bucket="${OIDC_BUCKET}" \
    --from-literal=region="${AWS_REGION}" \
    -n "${MC_NAME}"
  rm -f "${TMPFILE}"
  log_ok "Secret created"
fi

log_info "AddOnDeploymentConfig on hub"
KUBECONFIG="${HUB_KUBECONFIG}" oc apply -f - <<EOF
apiVersion: addon.open-cluster-management.io/v1alpha1
kind: AddOnDeploymentConfig
metadata:
  name: hypershift-operator-oidc-provider-s3
  namespace: ${MC_NAME}
spec:
  customizedVariables:
  - name: bucketSecretName
    value: hypershift-operator-oidc-provider-s3-credentials
  - name: bucketName
    value: ${OIDC_BUCKET}
  - name: bucketRegion
    value: ${AWS_REGION}
EOF
log_ok "AddOnDeploymentConfig applied"

# ============================================================
# 5. IAM role
# ============================================================
log_info "IAM role: hcp-hypershift-role"

if aws iam get-role --role-name hcp-hypershift-role &>/dev/null; then
  log_skip "IAM role hcp-hypershift-role"
else
  log_run "Creating IAM role"
  AWS_USER_ARN=$(aws sts get-caller-identity --query Arn --output text)

  TMPFILE=$(mktemp)
  cat >"${TMPFILE}" <<EOF
{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Principal": { "AWS": "${AWS_USER_ARN}" },
    "Action": "sts:AssumeRole"
  }]
}
EOF
  ROLE_ARN=$(aws iam create-role \
    --role-name hcp-hypershift-role \
    --assume-role-policy-document "file://${TMPFILE}" \
    --query 'Role.Arn' --output text)
  rm -f "${TMPFILE}"
  log_ok "Role created: ${ROLE_ARN}"
fi

ROLE_ARN=$(aws iam get-role --role-name hcp-hypershift-role --query 'Role.Arn' --output text)
echo "${ROLE_ARN}" >"${STATE_DIR}/role-arn"

log_info "IAM role policy"
TMPFILE=$(mktemp)
cat >"${TMPFILE}" <<'EOF'
{
  "Version": "2012-10-17",
  "Statement": [
    {"Sid": "EC2Full", "Effect": "Allow", "Action": "ec2:*", "Resource": "*"},
    {"Sid": "ELB", "Effect": "Allow", "Action": "elasticloadbalancing:*", "Resource": "*"},
    {"Sid": "Route53", "Effect": "Allow", "Action": "route53:*", "Resource": "*"},
    {"Sid": "IAM", "Effect": "Allow", "Action": [
      "iam:CreateRole", "iam:DeleteRole", "iam:GetRole", "iam:TagRole", "iam:UntagRole",
      "iam:AttachRolePolicy", "iam:DetachRolePolicy", "iam:PutRolePolicy",
      "iam:DeleteRolePolicy", "iam:GetRolePolicy", "iam:ListRolePolicies",
      "iam:ListAttachedRolePolicies",
      "iam:CreateInstanceProfile", "iam:DeleteInstanceProfile", "iam:GetInstanceProfile",
      "iam:AddRoleToInstanceProfile", "iam:RemoveRoleFromInstanceProfile",
      "iam:PassRole", "iam:CreateServiceLinkedRole",
      "iam:CreateOpenIDConnectProvider", "iam:DeleteOpenIDConnectProvider",
      "iam:GetOpenIDConnectProvider", "iam:TagOpenIDConnectProvider",
      "iam:ListOpenIDConnectProviders"
    ], "Resource": "*"},
    {"Sid": "S3", "Effect": "Allow", "Action": "s3:*", "Resource": "*"},
    {"Sid": "STS", "Effect": "Allow", "Action": ["sts:AssumeRole", "sts:AssumeRoleWithWebIdentity", "sts:GetCallerIdentity"], "Resource": "*"},
    {"Sid": "KMS", "Effect": "Allow", "Action": ["kms:DescribeKey", "kms:CreateGrant"], "Resource": "*"}
  ]
}
EOF
aws iam put-role-policy \
  --role-name hcp-hypershift-role \
  --policy-name hypershift-permissions \
  --policy-document "file://${TMPFILE}"
rm -f "${TMPFILE}"
log_ok "Role policy attached"

# ============================================================
# Summary
# ============================================================
echo ""
echo "=============================="
echo -e "${GREEN}AWS setup complete${NC}"
echo "=============================="
echo "  Route53 zone:  ${BASE_DOMAIN} (${SUBZONE_ID})"
echo "  OIDC bucket:   ${OIDC_BUCKET}"
echo "  IAM role:      ${ROLE_ARN}"
echo "  State dir:     ${STATE_DIR}/"
echo ""
echo -e "${GREEN}Ready to run 03-create-hc.sh${NC}"
