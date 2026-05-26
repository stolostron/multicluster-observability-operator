# HCP on AWS — Spoke Hosting via hypershift-addon

Deployment of a Hosted Control Plane with AWS infrastructure, where the HCP control plane pods run on a spoke cluster managed by an ACM hub. The HyperShift operator is deployed to the spoke via the [`hypershift-addon`](https://github.com/stolostron/hypershift-addon-operator)

---

## Prerequisites

- **An ACM hub cluster** with MCE installed and `hypershift-addon-manager` available
- **An OCP spoke cluster** with:
  - `oc` CLI access with cluster-admin
  - Already imported as a ManagedCluster on the hub
- AWS CLI
- AWS IAM user or role with broad permissions (EC2, ELB, Route53, IAM, S3, STS, KMS)
- AWS credentials configured locally (`~/.aws/credentials` or env vars)
- A pull secret from https://console.redhat.com/openshift/install/pull-secret (see point 6.3)
- An SSH public key (`~/.ssh/id_ed25519.pub` or `~/.ssh/id_rsa.pub`)

## How to get your AWS IAM user
The `observability-team` should get access to the `dev-07` AWS Account.

A list of minimum IAM user policy:
```json

  {
    "Version": "2012-10-17",
    "Statement": [
      {
        "Sid": "EC2",
        "Effect": "Allow",
        "Action": [
          "ec2:CreateVpc", "ec2:DeleteVpc", "ec2:DescribeVpcs", "ec2:ModifyVpcAttribute",
          "ec2:CreateSubnet", "ec2:DeleteSubnet", "ec2:DescribeSubnets",
          "ec2:CreateInternetGateway", "ec2:DeleteInternetGateway", "ec2:AttachInternetGateway", "ec2:DetachInternetGateway",
  "ec2:DescribeInternetGateways",
          "ec2:CreateNatGateway", "ec2:DeleteNatGateway", "ec2:DescribeNatGateways",
          "ec2:AllocateAddress", "ec2:ReleaseAddress", "ec2:DescribeAddresses",
          "ec2:CreateRouteTable", "ec2:DeleteRouteTable", "ec2:DescribeRouteTables",
          "ec2:CreateRoute", "ec2:DeleteRoute",
          "ec2:AssociateRouteTable", "ec2:DisassociateRouteTable", "ec2:ReplaceRouteTableAssociation",
          "ec2:CreateVpcEndpoint", "ec2:DeleteVpcEndpoints", "ec2:DescribeVpcEndpoints",
          "ec2:CreateDhcpOptions", "ec2:DeleteDhcpOptions", "ec2:AssociateDhcpOptions", "ec2:DescribeDhcpOptions",
          "ec2:CreateSecurityGroup", "ec2:DeleteSecurityGroup", "ec2:DescribeSecurityGroups",
          "ec2:AuthorizeSecurityGroupIngress", "ec2:AuthorizeSecurityGroupEgress",
          "ec2:RevokeSecurityGroupIngress", "ec2:RevokeSecurityGroupEgress",
          "ec2:DescribeAvailabilityZones",
          "ec2:DescribeNetworkInterfaces",
          "ec2:CreateTags"
        ],
        "Resource": "*"
      },
      {
        "Sid": "Route53",
        "Effect": "Allow",
        "Action": [
          "route53:CreateHostedZone", "route53:DeleteHostedZone",
          "route53:GetHostedZone", "route53:ListHostedZones",
          "route53:ChangeResourceRecordSets", "route53:ListResourceRecordSets",
          "route53:GetChange"
        ],
        "Resource": "*"
      },
      {
        "Sid": "IAM",
        "Effect": "Allow",
        "Action": [
          "iam:CreateRole", "iam:DeleteRole", "iam:GetRole", "iam:TagRole",
          "iam:PutRolePolicy", "iam:DeleteRolePolicy", "iam:GetRolePolicy", "iam:ListRolePolicies",
          "iam:AttachRolePolicy", "iam:DetachRolePolicy", "iam:ListAttachedRolePolicies",
          "iam:CreateInstanceProfile", "iam:DeleteInstanceProfile", "iam:GetInstanceProfile",
          "iam:AddRoleToInstanceProfile", "iam:RemoveRoleFromInstanceProfile",
          "iam:PassRole",
          "iam:CreateOpenIDConnectProvider", "iam:DeleteOpenIDConnectProvider",
          "iam:GetOpenIDConnectProvider", "iam:TagOpenIDConnectProvider",
          "iam:ListOpenIDConnectProviders"
        ],
        "Resource": "*"
      },
      {
        "Sid": "S3",
        "Effect": "Allow",
        "Action": [
          "s3:CreateBucket", "s3:DeleteBucket",
          "s3:PutBucketPolicy", "s3:GetBucketPolicy",
          "s3:PutBucketPublicAccessBlock", "s3:GetBucketPublicAccessBlock",
          "s3:PutObject", "s3:GetObject", "s3:DeleteObject", "s3:ListBucket"
        ],
        "Resource": "*"
      },
      {
        "Sid": "STS",
        "Effect": "Allow",
        "Action": [
          "sts:GetSessionToken",
          "sts:AssumeRole",
          "sts:GetCallerIdentity"
        ],
        "Resource": "*"
      },
      {
        "Sid": "KMS",
        "Effect": "Allow",
        "Action": ["kms:DescribeKey", "kms:CreateGrant"],
        "Resource": "*"
      }
    ]
  }
```
---

## Conventions

Set these at the start of every session:

```bash
export HC_NAME=hc-aws                               # HostedCluster name
export HC_NAMESPACE=clusters                        # Where HC/NodePool CRs live
export HCP_NAMESPACE=clusters-${HC_NAME}            # Where control plane pods run
export AWS_REGION=us-east-1                         # AWS region for worker nodes
export MC_NAME=<managed-cluster-name>               # Spoke name on the hub
```

Example of base domain for the ACM or observability-team **hcp-capa-myusername.dev07.red-chesterfield.com** composed of `hcp-capa-myusername` subdomain and the existing `dev07.red-chesterfield.com` parent domain.

---

## Step 0 — AWS credentials check

```bash
aws sts get-caller-identity
```
Should output your UserId and Account details.

```bash
aws configure list
```
Should output at least the `access_key` and `secret_key` generated from your AWS account.

---

## Step 1 — Route53 Delegated Sub-Zone

You need a Route53 public hosted zone that you control. If you already have one, skip to Step 2. If you don't own a top-level domain, create a delegated sub-zone under an existing zone.

### 1.1 Check existing zones

```bash
aws route53 list-hosted-zones --query 'HostedZones[].[Name,Id]' --output table
```

If you have a usable zone, set `BASE_DOMAIN` and skip ahead. Otherwise continue.
`dev07.red-chesterfield.com` should be a suitable zone for the ACM team.

### 1.2 Create the sub-zone

```bash
export BASE_DOMAIN=<your-prefix>.<parent-domain>
# Example: hcp-capa-myusername.dev07.red-chesterfield.com

aws route53 create-hosted-zone --name ${BASE_DOMAIN} --caller-reference $(date +%s) --query 'HostedZone.Id' --output text
```

Should return something like `/hostedzone/Z0XXXXXXXXXXXX`

### 1.3 Get the NS records

```bash
SUBZONE_ID=<zone-id-from-above>

aws route53 get-hosted-zone --id ${SUBZONE_ID} --query 'DelegationSet.NameServers' --output json
```

### 1.4 Add NS delegation in the parent zone

Name Server delegation is needed for future DNS records registration like `*.apps.hc-aws.hcp-capa-myusername.dev07.red-chesterfield.com`

```bash
PARENT_DOMAIN=<parent-domain>     # Like dev07.red-chesterfield.com

PARENT_ZONE_ID=$(aws route53 list-hosted-zones --query "HostedZones[?Name=='${PARENT_DOMAIN}.'].[Id]" --output text)

aws route53 change-resource-record-sets --hosted-zone-id ${PARENT_ZONE_ID} --change-batch '{
  "Changes": [{
    "Action": "CREATE",
    "ResourceRecordSet": {
      "Name": "'${BASE_DOMAIN}'",
      "Type": "NS",
      "TTL": 300,
      "ResourceRecords": [
        {"Value": "<ns-1>"},
        {"Value": "<ns-2>"},
        {"Value": "<ns-3>"},
        {"Value": "<ns-4>"}
      ]
    }
  }]
}'
```

### 1.5 Verify delegation

```bash
dig +short NS ${BASE_DOMAIN}
```

Should output the NS configured previously

---

## Step 2 — Configure OIDC S3 Bucket

The AWS Security Token Service (STS) authentication flow requires an S3 bucket to host OIDC discovery documents.

### 2.1 Create the S3 bucket

```bash
OIDC_BUCKET=hcp-oidc-${USER}-$(date +%s)

aws s3api create-bucket --bucket ${OIDC_BUCKET} --region ${AWS_REGION}

aws s3api put-public-access-block --bucket ${OIDC_BUCKET} --public-access-block-configuration "BlockPublicAcls=false,IgnorePublicAcls=false,BlockPublicPolicy=false,RestrictPublicBuckets=false"

aws s3api put-bucket-policy --bucket ${OIDC_BUCKET} --policy '{
  "Version": "2012-10-17",
  "Statement": [{
    "Sid": "AllowPublicRead",
    "Effect": "Allow",
    "Principal": "*",
    "Action": "s3:GetObject",
    "Resource": "arn:aws:s3:::'${OIDC_BUCKET}'/*"
  }]
}'

echo "OIDC bucket: ${OIDC_BUCKET}"
```

### 2.2 Create the OIDC ConfigMap on the spoke

This ConfigMap is used by the `hcp` CLI when rendering the HostedCluster manifest.

```bash
# On the spoke context
cat <<EOF | oc apply -f -
apiVersion: v1
kind: ConfigMap
metadata:
  name: oidc-storage-provider-s3-config
  namespace: kube-public
data:
  name: ${OIDC_BUCKET}
  region: ${AWS_REGION}
EOF
```

### 2.3 Create the OIDC credentials and config on the hub

**Switch to the hub** and create the secret + AddOnDeploymentConfig in the spoke's ManagedCluster namespace. The addon manager will use these to configure the HyperShift operator.

```bash
cat > /tmp/aws-creds <<EOF
[default]
aws_access_key_id = $(aws configure get aws_access_key_id)
aws_secret_access_key = $(aws configure get aws_secret_access_key)
EOF

oc create secret generic hypershift-operator-oidc-provider-s3-credentials --from-file=credentials=/tmp/aws-creds --from-literal=bucket=${OIDC_BUCKET} --from-literal=region=${AWS_REGION} -n ${MC_NAME}

rm /tmp/aws-creds

cat <<EOF | oc apply -f -
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
```

---

## Step 3 — Enable hypershift-addon on the Spoke (from Hub)

The HyperShift operator is installed via the hypershift-addon. The operator orchestrates Hosted Clusters lifecycle.

### 3.1 Verify the spoke is imported (on hub)

```bash
oc get managedcluster ${MC_NAME}
# Should show JOINED=True, AVAILABLE=True
```

### 3.2 Create the hypershift-addon ManagedClusterAddOn

Make sure the hypershift-operator-oidc-provider-s3 config has been created in Step 2.

```bash
cat <<EOF | oc apply -f -
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
```

### 3.3 Verify the addon is available

```bash
oc get managedclusteraddon -n ${MC_NAME} hypershift-addon
# Expect: AVAILABLE=True, DEGRADED=False
```

### 3.4 Verify operator pods on the spoke

Switch to the spoke and confirm the HyperShift operator appeared with the correct OIDC S3 args:

```bash
oc get pods -n hypershift
# Expect 2 operator pods Running (may take 2-3 minutes)

oc get deploy -n hypershift operator -o jsonpath='{.spec.template.spec.containers[0].args}' | grep oidc
# Should show:
#   --oidc-storage-provider-s3-bucket-name=<bucket>
#   --oidc-storage-provider-s3-region=<region>
#   --oidc-storage-provider-s3-credentials=/etc/oidc-storage-provider-s3-creds/credentials
```

### 3.5 Download the hcp CLI (from the hub)

If you don't already have the `hcp` CLI, download it from the hub's MCE route:

```bash
# Run against the hub
HCP_URL=$(oc get route -n multicluster-engine hcp-cli-download -o jsonpath='{.spec.host}')

# Adjust arch: linux/amd64, darwin/arm64, darwin/amd64
curl -kL -o /tmp/hcp.tar.gz "https://${HCP_URL}/darwin/arm64/hcp.tar.gz"
tar -xzvf /tmp/hcp.tar.gz -C /tmp/
sudo mv /tmp/hcp /usr/local/bin/
hcp version
```

---

## Step 4 — Create the IAM Role

The hcp CLI requires an IAM role for the STS flow.

### 4.1 Get your IAM identity

```bash
AWS_ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)
AWS_USER_ARN=$(aws sts get-caller-identity --query Arn --output text)
echo "Account: ${AWS_ACCOUNT_ID}, User: ${AWS_USER_ARN}"
```

### 4.2 Create the role

```bash
cat > /tmp/trust-policy.json <<EOF
{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Principal": { "AWS": "${AWS_USER_ARN}" },
    "Action": "sts:AssumeRole"
  }]
}
EOF

aws iam create-role --role-name hcp-hypershift-role --assume-role-policy-document file:///tmp/trust-policy.json --query 'Role.Arn' --output text

rm /tmp/trust-policy.json
```

This role is used by hcp cli and HyperShift operator to manage AWS resources.

### 4.3 Attach permissions

Find more fine-grained IAM permissions in the official documentation https://docs.redhat.com/en/documentation/openshift_container_platform/4.21/pdf/hosted_control_planes/OpenShift_Container_Platform-4.21-Hosted_control_planes-en-US.pdf

```bash
cat > /tmp/hypershift-policy.json <<'EOF'
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

aws iam put-role-policy --role-name hcp-hypershift-role --policy-name hypershift-permissions --policy-document file:///tmp/hypershift-policy.json

rm /tmp/hypershift-policy.json
```

> For production, scope EC2/ELB/S3 permissions to specific regions and resources.

---

## Step 5 — Determine Release Version

```bash
# On the spoke
oc logs -n hypershift -l app=operator --tail=200 | grep "Latest supported OCP"
# Example: "Latest supported OCP: 4.21.0"

# Find the latest stable patch:
curl -s https://mirror.openshift.com/pub/openshift-v4/clients/ocp/stable-4.21/release.txt | grep "Name:"
```

Set `RELEASE_VERSION` to the result (e.g., `4.21.15`).

```bash
RELEASE_VERSION=4.21.15
```

---

## Step 6 — Create the HostedCluster (on Spoke)

All commands in this phase run against the **spoke** cluster.

### 6.1 Generate STS session token

```bash
aws sts get-session-token --output json > /tmp/sts-creds.json
```

This token expires in 12 hours. It is only used at render time by the `hcp` CLI.

### 6.2 Enable wildcard routes on the spoke

This is needed to serve requests like `*.apps.<hc-name>.<base-domain>`

```bash
oc patch ingresscontroller -n openshift-ingress-operator default --type=merge -p '{"spec":{"routeAdmission":{"wildcardPolicy":"WildcardsAllowed"}}}'
```

### 6.3 If needed, create the image repository pull secret

Download a pull secret from https://console.redhat.com/openshift/install/pull-secret and save it locally:

```bash
# Save the downloaded JSON to a known path
PULL_SECRET_FILE=~/pull-secret.json
ls ${PULL_SECRET_FILE}  # verify the file exists before proceeding
```

### 6.4 Render and apply the HostedCluster

You can select a different worker instance type, navigate the sizes here: https://aws.amazon.com/ec2/instance-types/general-purpose/

> **IMPORTANT**: The `--render` option creates real AWS resources (VPC, subnets, NAT GW, IGW, route tables, Route53 zones, OIDC provider, IAM roles, instance profiles). If the command fails mid-way (e.g., wrong pull-secret path), you'll have orphaned AWS infra that needs manual cleanup. **Verify `PULL_SECRET_FILE` and `SSH_KEY_FILE` paths exist before running.**


```bash
ROLE_ARN=arn:aws:iam::${AWS_ACCOUNT_ID}:role/hcp-hypershift-role
PULL_SECRET_FILE=<path-to-pull-secret>    # e.g., ~/pull-secret.json
SSH_KEY_FILE=<path-to-ssh-pub-key>        # e.g., ~/.ssh/id_ed25519.pub

hcp create cluster aws \
  --name=${HC_NAME} \
  --namespace=${HC_NAMESPACE} \
  --region=${AWS_REGION} \
  --release-image=quay.io/openshift-release-dev/ocp-release:${RELEASE_VERSION}-x86_64 \
  --pull-secret=${PULL_SECRET_FILE} \
  --ssh-key=${SSH_KEY_FILE} \
  --node-pool-replicas=2 \
  --instance-type=m6i.xlarge \
  --base-domain=${BASE_DOMAIN} \
  --control-plane-availability-policy=SingleReplica \
  --infra-availability-policy=SingleReplica \
  --sts-creds=/tmp/sts-creds.json \
  --role-arn=${ROLE_ARN} \
  --render --render-sensitive > /tmp/hc-rendered.yaml
```

> If the render fails and you need to re-run, pass `--infra-id=<infra-id>` (visible in the render logs) to reuse the already-created AWS resources instead of creating duplicates.

### 6.5 Apply

Verify the rendered manifest and apply

```bash
oc apply -f /tmp/hc-rendered.yaml
```

---

## Step 7 — Monitor the Deployment

### 7.1 Watch the control plane

```bash
oc get hostedcluster -n ${HC_NAMESPACE} ${HC_NAME} -w

oc get pods -n ${HCP_NAMESPACE}
# Expect ~45 pods over 5-10 minutes
```

### 7.2 Verify OIDC documents were uploaded

```bash
aws s3 ls s3://${OIDC_BUCKET}/ --recursive
# Must show:
#   <infra-id>/.well-known/openid-configuration
#   <infra-id>/openid/v1/jwks
```

### 7.3 Watch worker provisioning

```bash
oc get nodepool -n ${HC_NAMESPACE}

INFRA_ID=$(oc get hostedcluster -n ${HC_NAMESPACE} ${HC_NAME} -o jsonpath='{.spec.infraID}')
aws ec2 describe-instances \
  --filters "Name=tag:kubernetes.io/cluster/${INFRA_ID},Values=owned" \
  --query 'Reservations[].Instances[].[InstanceId,State.Name,InstanceType,PrivateIpAddress]' \
  --output table --region ${AWS_REGION}
```

### 7.4 Connect to the hosted cluster

```bash
oc extract secret/${HC_NAME}-admin-kubeconfig \
  -n ${HC_NAMESPACE} --to=- > /tmp/${HC_NAME}-kubeconfig

KUBECONFIG=/tmp/${HC_NAME}-kubeconfig oc whoami
KUBECONFIG=/tmp/${HC_NAME}-kubeconfig oc get nodes
KUBECONFIG=/tmp/${HC_NAME}-kubeconfig oc get co
```

---

## Cleanup

### Destroy the hosted cluster

```bash
ROLE_ARN=arn:aws:iam::${AWS_ACCOUNT_ID}:role/hcp-hypershift-role

hcp destroy cluster aws \
  --name=${HC_NAME} \
  --namespace=${HC_NAMESPACE} \
  --sts-creds=/tmp/sts-creds.json \
  --role-arn=${ROLE_ARN} \
  --region=${AWS_REGION}
```

> This tears down EC2 instances, VPC, subnets, NAT GW, ELBs, security groups, IAM roles, and Route53 records.

### Remove the hypershift-addon (on hub)

```bash
# Switch to hub context
oc delete managedclusteraddon -n ${MC_NAME} hypershift-addon
oc delete addondeploymentconfig -n ${MC_NAME} hypershift-operator-oidc-provider-s3
oc delete secret -n ${MC_NAME} hypershift-operator-oidc-provider-s3-credentials
```

### Clean up IAM role and S3 bucket

```bash
aws iam delete-role-policy --role-name hcp-hypershift-role --policy-name hypershift-permissions
aws iam delete-role --role-name hcp-hypershift-role

aws s3 rm s3://${OIDC_BUCKET} --recursive
aws s3api delete-bucket --bucket ${OIDC_BUCKET}
```

### Clean up Route53 sub-zone delegation (if created)

```bash
# Delete the NS record from the parent zone
aws route53 change-resource-record-sets --hosted-zone-id ${PARENT_ZONE_ID} --change-batch '{
  "Changes": [{"Action": "DELETE", "ResourceRecordSet": {...}}]
}'

# Delete the sub-zone
aws route53 delete-hosted-zone --id ${SUBZONE_ID}
```

### Cleaning up orphaned AWS infra from a failed render

If `hcp create cluster aws --render` fails mid-way, it leaves partial AWS resources. The infra ID is printed in the render logs (e.g., `hc-aws-cdk2x`). Clean up:

```bash
ORPHAN_INFRA_ID=<infra-id-from-failed-render>

# Delete VPC dependencies first
# Find resources tagged with kubernetes.io/cluster/${ORPHAN_INFRA_ID}=owned
aws ec2 describe-vpcs --filters "Name=tag:kubernetes.io/cluster/${ORPHAN_INFRA_ID},Values=owned" \
  --query 'Vpcs[].VpcId' --output text --region ${AWS_REGION}

# Then delete: NAT GW -> EIP -> subnets -> IGW -> VPC endpoints -> route tables -> VPC
# Then delete: IAM roles, instance profiles, OIDC provider

# IAM roles (all prefixed with infra ID)
for role in cloud-network-config-controller openshift-ingress openshift-image-registry aws-ebs-csi-driver-controller cloud-controller node-pool control-plane-operator; do
  aws iam delete-role-policy --role-name ${ORPHAN_INFRA_ID}-${role} --policy-name ${ORPHAN_INFRA_ID}-${role} 2>/dev/null
  aws iam delete-role --role-name ${ORPHAN_INFRA_ID}-${role} 2>/dev/null
done

aws iam remove-role-from-instance-profile --instance-profile-name ${ORPHAN_INFRA_ID}-worker --role-name ${ORPHAN_INFRA_ID}-worker-role 2>/dev/null
aws iam delete-instance-profile --instance-profile-name ${ORPHAN_INFRA_ID}-worker 2>/dev/null
aws iam delete-role-policy --role-name ${ORPHAN_INFRA_ID}-worker-role \
  --policy-name ${ORPHAN_INFRA_ID}-worker-policy 2>/dev/null
aws iam delete-role --role-name ${ORPHAN_INFRA_ID}-worker-role 2>/dev/null

# OIDC provider
aws iam delete-open-id-connect-provider \
  --open-id-connect-provider-arn arn:aws:iam::${AWS_ACCOUNT_ID}:oidc-provider/${OIDC_BUCKET}.s3.${AWS_REGION}.amazonaws.com/${ORPHAN_INFRA_ID}
```

---

## Files to Keep

- `/tmp/hc-rendered.yaml` — source of truth for what was deployed
- `/tmp/${HC_NAME}-kubeconfig` — admin kubeconfig for the hosted cluster
- `/tmp/sts-creds.json` — STS session token (expires in ~12h; regenerate as needed)
