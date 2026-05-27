#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
STATE_DIR="${SCRIPT_DIR}/.state"

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

ok() { echo -e "  ${GREEN}OK${NC}   $1"; }
skip() { echo -e "  ${GREEN}SKIP${NC} $1 (not found)"; }
run() { echo -e "  ${YELLOW}...${NC}  $1"; }
CLEANUP_FAILED=false
err() {
  echo -e "  ${RED}ERROR${NC} $1"
  CLEANUP_FAILED=true
}
info() { echo -e "${YELLOW}==>${NC} $1"; }

source "${SCRIPT_DIR}/env.sh"

HCP_NAMESPACE="clusters-${HC_NAME}"
OIDC_BUCKET=$(cat "${STATE_DIR}/oidc-bucket" 2>/dev/null || echo "hcp-oidc-${USER}-${HC_NAME}")
ROLE_ARN=$(cat "${STATE_DIR}/role-arn" 2>/dev/null ||
  aws iam get-role --role-name hcp-hypershift-role --query 'Role.Arn' --output text 2>/dev/null || echo "")

# ============================================================
# cleanup_aws_infra <infra-id>
# Deletes all AWS resources tagged with the given infra ID:
# ============================================================
cleanup_aws_infra() {
  local infra_id="$1"
  info "Cleaning AWS resources for infra ID: ${infra_id}"

  # EC2 instances
  local instance_ids
  instance_ids=$(aws ec2 describe-instances \
    --filters "Name=tag:kubernetes.io/cluster/${infra_id},Values=owned" "Name=instance-state-name,Values=running,pending,stopping,stopped" \
    --query 'Reservations[].Instances[].InstanceId' --output text --region "${AWS_REGION}" 2>/dev/null)
  if [[ -n ${instance_ids} ]]; then
    run "Terminating EC2 instances"
    aws ec2 terminate-instances --instance-ids ${instance_ids} --region "${AWS_REGION}" >/dev/null
    aws ec2 wait instance-terminated --instance-ids ${instance_ids} --region "${AWS_REGION}" 2>/dev/null || true
    ok "EC2 instances terminated"
  fi

  # VPC and its dependencies
  local vpc_id
  vpc_id=$(aws ec2 describe-vpcs \
    --filters "Name=tag:kubernetes.io/cluster/${infra_id},Values=owned" \
    --query 'Vpcs[0].VpcId' --output text --region "${AWS_REGION}" 2>/dev/null)

  if [[ -n ${vpc_id} && ${vpc_id} != "None" ]]; then
    # Load balancers (NLB/ALB)
    local lb_arns
    lb_arns=$(aws elbv2 describe-load-balancers \
      --query "LoadBalancers[?VpcId=='${vpc_id}'].LoadBalancerArn" --output text --region "${AWS_REGION}" 2>/dev/null)
    for lb in ${lb_arns}; do
      run "Deleting load balancer"
      aws elbv2 delete-load-balancer --load-balancer-arn "${lb}" --region "${AWS_REGION}" 2>/dev/null || true
    done
    # Classic ELBs
    local clb_names
    clb_names=$(aws elb describe-load-balancers \
      --query "LoadBalancerDescriptions[?VPCId=='${vpc_id}'].LoadBalancerName" --output text --region "${AWS_REGION}" 2>/dev/null)
    for clb in ${clb_names}; do
      run "Deleting classic LB ${clb}"
      aws elb delete-load-balancer --load-balancer-name "${clb}" --region "${AWS_REGION}" 2>/dev/null || true
    done
    if [[ -n ${lb_arns} || -n ${clb_names} ]]; then
      run "Waiting for LB ENIs to detach (30s)"
      sleep 30
    fi

    # Security groups (non-default)
    local sg_ids
    sg_ids=$(aws ec2 describe-security-groups \
      --filters "Name=vpc-id,Values=${vpc_id}" \
      --query "SecurityGroups[?GroupName!='default'].GroupId" --output text --region "${AWS_REGION}" 2>/dev/null)
    for sg in ${sg_ids}; do
      local ingress egress
      ingress=$(aws ec2 describe-security-groups --group-ids "${sg}" --query 'SecurityGroups[0].IpPermissions' --output json --region "${AWS_REGION}" 2>/dev/null)
      [[ ${ingress} != "[]" ]] && aws ec2 revoke-security-group-ingress --group-id "${sg}" --ip-permissions "${ingress}" --region "${AWS_REGION}" >/dev/null 2>&1 || true
      egress=$(aws ec2 describe-security-groups --group-ids "${sg}" --query 'SecurityGroups[0].IpPermissionsEgress' --output json --region "${AWS_REGION}" 2>/dev/null)
      [[ ${egress} != "[]" ]] && aws ec2 revoke-security-group-egress --group-id "${sg}" --ip-permissions "${egress}" --region "${AWS_REGION}" >/dev/null 2>&1 || true
      aws ec2 delete-security-group --group-id "${sg}" --region "${AWS_REGION}" >/dev/null 2>&1 || true
    done

    # NAT gateways
    local nat_ids
    nat_ids=$(aws ec2 describe-nat-gateways \
      --filter "Name=vpc-id,Values=${vpc_id}" "Name=state,Values=available,pending" \
      --query 'NatGateways[].NatGatewayId' --output text --region "${AWS_REGION}" 2>/dev/null)
    for nat in ${nat_ids}; do
      run "Deleting NAT gateway ${nat}"
      aws ec2 delete-nat-gateway --nat-gateway-id "${nat}" --region "${AWS_REGION}" >/dev/null 2>&1 || true
    done
    [[ -n ${nat_ids} ]] && sleep 60

    # VPC endpoints
    local vpce_ids
    vpce_ids=$(aws ec2 describe-vpc-endpoints \
      --filters "Name=vpc-id,Values=${vpc_id}" \
      --query 'VpcEndpoints[].VpcEndpointId' --output text --region "${AWS_REGION}" 2>/dev/null)
    [[ -n ${vpce_ids} ]] && aws ec2 delete-vpc-endpoints --vpc-endpoint-ids ${vpce_ids} --region "${AWS_REGION}" >/dev/null 2>&1 || true

    # Subnets
    for subnet in $(aws ec2 describe-subnets --filters "Name=vpc-id,Values=${vpc_id}" --query 'Subnets[].SubnetId' --output text --region "${AWS_REGION}" 2>/dev/null); do
      aws ec2 delete-subnet --subnet-id "${subnet}" --region "${AWS_REGION}" 2>/dev/null || true
    done

    # Internet gateway
    for igw in $(aws ec2 describe-internet-gateways --filters "Name=attachment.vpc-id,Values=${vpc_id}" --query 'InternetGateways[].InternetGatewayId' --output text --region "${AWS_REGION}" 2>/dev/null); do
      aws ec2 detach-internet-gateway --internet-gateway-id "${igw}" --vpc-id "${vpc_id}" --region "${AWS_REGION}" 2>/dev/null || true
      aws ec2 delete-internet-gateway --internet-gateway-id "${igw}" --region "${AWS_REGION}" 2>/dev/null || true
    done

    # Route tables (non-main)
    for rt in $(aws ec2 describe-route-tables --filters "Name=vpc-id,Values=${vpc_id}" --query 'RouteTables[?Associations[0].Main!=`true`].RouteTableId' --output text --region "${AWS_REGION}" 2>/dev/null); do
      for assoc in $(aws ec2 describe-route-tables --route-table-ids "${rt}" --query 'RouteTables[0].Associations[?!Main].RouteTableAssociationId' --output text --region "${AWS_REGION}" 2>/dev/null); do
        aws ec2 disassociate-route-table --association-id "${assoc}" --region "${AWS_REGION}" 2>/dev/null || true
      done
      aws ec2 delete-route-table --route-table-id "${rt}" --region "${AWS_REGION}" 2>/dev/null || true
    done

    # EIPs
    for eip in $(aws ec2 describe-addresses --filters "Name=tag:kubernetes.io/cluster/${infra_id},Values=owned" --query 'Addresses[].AllocationId' --output text --region "${AWS_REGION}" 2>/dev/null); do
      aws ec2 release-address --allocation-id "${eip}" --region "${AWS_REGION}" 2>/dev/null || true
    done

    # VPC
    run "Deleting VPC ${vpc_id}"
    aws ec2 delete-vpc --vpc-id "${vpc_id}" --region "${AWS_REGION}" 2>/dev/null &&
      ok "VPC deleted" || err "VPC deletion failed — may need manual cleanup"

    # DHCP options
    for dhcp in $(aws ec2 describe-dhcp-options --filters "Name=tag:kubernetes.io/cluster/${infra_id},Values=owned" --query 'DhcpOptions[].DhcpOptionsId' --output text --region "${AWS_REGION}" 2>/dev/null); do
      aws ec2 delete-dhcp-options --dhcp-options-id "${dhcp}" --region "${AWS_REGION}" 2>/dev/null || true
    done
  fi

  # Route53 private zones
  for zone_name in "${HC_NAME}.${BASE_DOMAIN}." "${HC_NAME}.hypershift.local."; do
    local zone_id
    zone_id=$(aws route53 list-hosted-zones \
      --query "HostedZones[?Name=='${zone_name}' && Config.PrivateZone==\`true\`].Id" --output text 2>/dev/null | head -1)
    if [[ -n ${zone_id} && ${zone_id} != "None" && ${zone_id} != "" ]]; then
      local change_batch record_count
      change_batch=$(aws route53 list-resource-record-sets --hosted-zone-id "${zone_id}" --output json |
        jq '{"Changes": [.ResourceRecordSets[] | select(.Type != "NS" and .Type != "SOA") | {"Action": "DELETE", "ResourceRecordSet": .}]}')
      record_count=$(echo "${change_batch}" | jq '.Changes | length')
      if [[ ${record_count} -gt 0 ]]; then
        echo "${change_batch}" >/tmp/zone-batch.json
        aws route53 change-resource-record-sets --hosted-zone-id "${zone_id}" \
          --change-batch file:///tmp/zone-batch.json >/dev/null 2>&1 || true
        rm -f /tmp/zone-batch.json
      fi
      aws route53 delete-hosted-zone --id "${zone_id}" >/dev/null 2>&1 || true
      ok "Deleted private zone ${zone_name}"
    fi
  done

  # IAM roles
  for role_suffix in cloud-network-config-controller openshift-ingress openshift-image-registry \
    aws-ebs-csi-driver-controller cloud-controller node-pool control-plane-operator; do
    aws iam delete-role-policy --role-name "${infra_id}-${role_suffix}" --policy-name "${infra_id}-${role_suffix}" 2>/dev/null || true
    aws iam delete-role --role-name "${infra_id}-${role_suffix}" 2>/dev/null || true
  done
  aws iam remove-role-from-instance-profile --instance-profile-name "${infra_id}-worker" --role-name "${infra_id}-worker-role" 2>/dev/null || true
  aws iam delete-instance-profile --instance-profile-name "${infra_id}-worker" 2>/dev/null || true
  aws iam delete-role-policy --role-name "${infra_id}-worker-role" --policy-name "${infra_id}-worker-policy" 2>/dev/null || true
  aws iam delete-role --role-name "${infra_id}-worker-role" 2>/dev/null || true

  # OIDC provider
  local aws_account_id
  aws_account_id=$(aws sts get-caller-identity --query Account --output text)
  aws iam delete-open-id-connect-provider \
    --open-id-connect-provider-arn "arn:aws:iam::${aws_account_id}:oidc-provider/${OIDC_BUCKET}.s3.${AWS_REGION}.amazonaws.com/${infra_id}" 2>/dev/null || true

  ok "AWS infrastructure cleanup complete for ${infra_id}"
}

# ============================================================
# force_cleanup_hcp
# Removes K8s finalizers and force-deletes the HostedCluster,
# then calls cleanup_aws_infra to clean orphaned AWS resources.
# ============================================================
force_cleanup_hcp() {
  local infra_id
  infra_id=$(KUBECONFIG="${SPOKE_KUBECONFIG}" oc get hostedcluster "${HC_NAME}" \
    -n "${HC_NAMESPACE}" -o jsonpath='{.spec.infraID}' 2>/dev/null || echo "")

  KUBECONFIG="${SPOKE_KUBECONFIG}" oc patch hostedcontrolplane -n "${HCP_NAMESPACE}" "${HC_NAME}" \
    --type=merge -p '{"metadata":{"finalizers":[]}}' 2>/dev/null || true
  KUBECONFIG="${SPOKE_KUBECONFIG}" oc patch cluster.cluster.x-k8s.io -n "${HCP_NAMESPACE}" "${HC_NAME}" \
    --type=merge -p '{"metadata":{"finalizers":[]}}' 2>/dev/null || true
  KUBECONFIG="${SPOKE_KUBECONFIG}" oc delete hostedcluster -n "${HC_NAMESPACE}" "${HC_NAME}" \
    --force --grace-period=0 2>/dev/null || true
  KUBECONFIG="${SPOKE_KUBECONFIG}" oc delete namespace "${HCP_NAMESPACE}" \
    --force --grace-period=0 2>/dev/null || true
  ok "Kubernetes resources force-deleted"

  if [[ -n ${infra_id} ]]; then
    cleanup_aws_infra "${infra_id}"
  else
    err "Could not determine infra ID — check for orphaned AWS resources manually"
  fi
}

# ============================================================
# Parse flags
# ============================================================
CLEANUP_DNS=false
SKIP_CONFIRM=false
for arg in "$@"; do
  case "${arg}" in
    --cleanup-dns) CLEANUP_DNS=true ;;
    --yes) SKIP_CONFIRM=true ;;
  esac
done

# ============================================================
# Confirmation
# ============================================================
if [[ ${SKIP_CONFIRM} != "true" ]]; then
  echo ""
  echo -e "${RED}This will destroy:${NC}"
  echo "  - HostedCluster ${HC_NAME} (and all its AWS resources: VPC, EC2, ELB, etc.)"
  echo "  - hypershift-addon on spoke ${MC_NAME}"
  echo "  - IAM role hcp-hypershift-role"
  echo "  - OIDC S3 bucket ${OIDC_BUCKET}"
  if [[ ${CLEANUP_DNS} == "true" ]]; then
    echo "  - Route53 sub-zone ${BASE_DOMAIN} and NS delegation"
  fi
  echo ""
  read -rp "Continue? [y/N] " confirm
  if [[ ${confirm} != "y" && ${confirm} != "Y" ]]; then
    echo "Aborted."
    exit 0
  fi
fi

# ============================================================
# 1. Destroy hosted cluster
# ============================================================
info "Destroying HostedCluster ${HC_NAME}"

if KUBECONFIG="${SPOKE_KUBECONFIG}" oc get hostedcluster "${HC_NAME}" \
  -n "${HC_NAMESPACE}" >/dev/null 2>&1; then
  if [[ ! -f /tmp/sts-creds.json ]]; then
    run "Generating STS session token"
    aws sts get-session-token --output json >/tmp/sts-creds.json
  fi

  run "Running hcp destroy cluster aws (timeout 15m)"
  if KUBECONFIG="${SPOKE_KUBECONFIG}" timeout 900 hcp destroy cluster aws \
    --name="${HC_NAME}" \
    --namespace="${HC_NAMESPACE}" \
    --sts-creds=/tmp/sts-creds.json \
    --role-arn="${ROLE_ARN}" \
    --region="${AWS_REGION}" 2>&1; then
    ok "HostedCluster destroyed"
  else
    echo -e "  ${YELLOW}WARN${NC} hcp destroy timed out or failed — attempting force cleanup"
    force_cleanup_hcp
  fi
else
  # No HC on cluster — check for orphaned infra from a failed render
  ORPHAN_INFRA_ID=$(cat "${STATE_DIR}/infra-id" 2>/dev/null || echo "")
  if [[ -n ${ORPHAN_INFRA_ID} ]]; then
    info "No HostedCluster found, but orphaned infra ID detected: ${ORPHAN_INFRA_ID}"
    cleanup_aws_infra "${ORPHAN_INFRA_ID}"
    rm -f "${STATE_DIR}/infra-id"
  else
    skip "HostedCluster ${HC_NAME}"
  fi
fi

# ============================================================
# 2. Remove hypershift-addon (on hub)
# ============================================================
info "Removing hypershift-addon from hub"

KUBECONFIG="${HUB_KUBECONFIG}" oc delete managedclusteraddon \
  hypershift-addon -n "${MC_NAME}" --ignore-not-found 2>/dev/null &&
  ok "ManagedClusterAddOn deleted" || skip "ManagedClusterAddOn"

KUBECONFIG="${HUB_KUBECONFIG}" oc delete addondeploymentconfig \
  hypershift-operator-oidc-provider-s3 -n "${MC_NAME}" --ignore-not-found 2>/dev/null &&
  ok "AddOnDeploymentConfig deleted" || skip "AddOnDeploymentConfig"

KUBECONFIG="${HUB_KUBECONFIG}" oc delete secret \
  hypershift-operator-oidc-provider-s3-credentials -n "${MC_NAME}" --ignore-not-found 2>/dev/null &&
  ok "OIDC credentials secret deleted" || skip "OIDC credentials secret"

# ============================================================
# 3. Clean up spoke-side OIDC resources
# ============================================================
info "Cleaning spoke-side OIDC ConfigMap"

KUBECONFIG="${SPOKE_KUBECONFIG}" oc delete configmap \
  oidc-storage-provider-s3-config -n kube-public --ignore-not-found 2>/dev/null &&
  ok "ConfigMap deleted" || skip "ConfigMap"

# ============================================================
# 4. Clean up IAM role
# ============================================================
info "Cleaning IAM role"

if aws iam get-role --role-name hcp-hypershift-role &>/dev/null; then
  aws iam delete-role-policy --role-name hcp-hypershift-role \
    --policy-name hypershift-permissions 2>/dev/null || true
  aws iam delete-role --role-name hcp-hypershift-role 2>/dev/null
  ok "IAM role hcp-hypershift-role deleted"
else
  skip "IAM role hcp-hypershift-role"
fi

# ============================================================
# 5. Clean up S3 bucket
# ============================================================
info "Cleaning OIDC S3 bucket: ${OIDC_BUCKET}"

if aws s3api head-bucket --bucket "${OIDC_BUCKET}" >/dev/null 2>&1; then
  aws s3 rm "s3://${OIDC_BUCKET}" --recursive --quiet 2>/dev/null || true
  aws s3api delete-bucket --bucket "${OIDC_BUCKET}" 2>/dev/null
  ok "Bucket ${OIDC_BUCKET} deleted"
else
  skip "Bucket ${OIDC_BUCKET}"
fi

# ============================================================
# 6. Clean up Route53 (optional)
# ============================================================
if [[ ${CLEANUP_DNS} == "true" ]]; then
  info "Cleaning Route53 sub-zone: ${BASE_DOMAIN}"

  SUBZONE_ID=$(cat "${STATE_DIR}/subzone-id" 2>/dev/null ||
    aws route53 list-hosted-zones --query "HostedZones[?Name=='${BASE_DOMAIN}.'].Id" --output text 2>/dev/null | head -1)

  if [[ -n ${SUBZONE_ID} && ${SUBZONE_ID} != "None" ]]; then
    PARENT_ZONE_ID=$(aws route53 list-hosted-zones \
      --query "HostedZones[?Name=='${PARENT_DOMAIN}.'].Id" --output text 2>/dev/null | head -1)

    if [[ -n ${PARENT_ZONE_ID} && ${PARENT_ZONE_ID} != "None" ]]; then
      NS_RECORDS=$(aws route53 get-hosted-zone --id "${SUBZONE_ID}" \
        --query 'DelegationSet.NameServers' --output json 2>/dev/null || echo "[]")
      NS_RRS=$(echo "${NS_RECORDS}" | jq '[.[] | {"Value": .}]')

      aws route53 change-resource-record-sets --hosted-zone-id "${PARENT_ZONE_ID}" --change-batch '{
              "Changes": [{
                "Action": "DELETE",
                "ResourceRecordSet": {
                  "Name": "'"${BASE_DOMAIN}"'",
                  "Type": "NS",
                  "TTL": 300,
                  "ResourceRecords": '"${NS_RRS}"'
                }
              }]
            }' >/dev/null 2>&1 && ok "NS delegation removed" || echo -e "  ${YELLOW}WARN${NC} Could not remove NS delegation"
    fi

    # Delete all records in the sub-zone before deleting it
    CHANGE_BATCH=$(aws route53 list-resource-record-sets --hosted-zone-id "${SUBZONE_ID}" --output json |
      jq '{"Changes": [.ResourceRecordSets[] | select(.Type != "NS" and .Type != "SOA") | {"Action": "DELETE", "ResourceRecordSet": .}]}')
    RECORD_COUNT=$(echo "${CHANGE_BATCH}" | jq '.Changes | length')

    if [[ ${RECORD_COUNT} -gt 0 ]]; then
      echo "${CHANGE_BATCH}" >/tmp/zone-batch.json
      aws route53 change-resource-record-sets --hosted-zone-id "${SUBZONE_ID}" \
        --change-batch file:///tmp/zone-batch.json >/dev/null 2>&1 || true
      rm -f /tmp/zone-batch.json
    fi

    aws route53 delete-hosted-zone --id "${SUBZONE_ID}" >/dev/null 2>&1 &&
      ok "Sub-zone ${BASE_DOMAIN} deleted" || err "Could not delete sub-zone"
  else
    skip "Sub-zone ${BASE_DOMAIN}"
  fi
else
  echo -e "\n${YELLOW}NOTE:${NC} Route53 sub-zone ${BASE_DOMAIN} was NOT deleted."
  echo "      Pass --cleanup-dns to also remove the sub-zone and NS delegation."
fi

# ============================================================
# 7. Clean up state
# ============================================================
rm -f "/tmp/${HC_NAME}-kubeconfig" /tmp/hc-rendered.yaml /tmp/sts-creds.json

# ============================================================
# Summary
# ============================================================
echo ""
if [[ ${CLEANUP_FAILED} == "true" ]]; then
  echo "=============================="
  echo -e "${RED}Cleanup completed with errors${NC}"
  echo "=============================="
  echo -e "  .state/ preserved for retry. Re-run ./04-destroy-hc.sh"
  exit 1
else
  rm -rf "${STATE_DIR}"
  echo "=============================="
  echo -e "${GREEN}Cleanup complete${NC}"
  echo "=============================="
fi
