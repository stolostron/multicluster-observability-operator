# HCP on AWS — Automation Scripts

Scripts to deploy a Hosted Control Plane on a spoke cluster via the hub's `hypershift-addon`.
See the [full guide](./docs/hcp-aws-spoke-via-addon-guide.md) for detailed explanations.

## Architecture

![Architecture](docs/architecture.png)

## Quick Start

Setup the environment values and run the numbered scripts in order

```bash
cp env.sh.example env.sh

./01-check-prereqs.sh       # Validate requirements
./02-aws-setup.sh           # Create Route53, OIDC S3, IAM role
./03-create-hc.sh           # Deploy hypershift-addon + hosted cluster
```

When done, tear down OCP and AWS resources

```
./04-destroy-hc.sh          # Tear down everything
```


## Environment Variables

| Variable | Description | Example |
|---|---|---|
| `HUB_KUBECONFIG` | Path to hub cluster kubeconfig | `~/.kube/hub-kubeconfig` |
| `SPOKE_KUBECONFIG` | Path to spoke cluster kubeconfig | `~/.kube/spoke-kubeconfig` |
| `MC_NAME` | ManagedCluster name on the hub | `my-spoke-cluster` |
| `HC_NAME` | HostedCluster name | `hc-aws` |
| `HC_NAMESPACE` | Namespace for HC/NodePool CRs | `clusters` |
| `AWS_REGION` | AWS region for worker nodes | `us-east-1` |
| `BASE_DOMAIN` | Route53 public hosted zone | `hcp-capa-me.dev07.red-chesterfield.com` |
| `PARENT_DOMAIN` | Parent domain for NS delegation | `dev07.red-chesterfield.com` |
| `PULL_SECRET_FILE` | Path to Red Hat pull secret JSON | `~/pull-secret.json` |
| `SSH_KEY_FILE` | Path to SSH public key | `~/.ssh/id_ed25519.pub` |
| `NODE_POOL_REPLICAS` | Number of worker nodes (default: 2) | `2` |
| `INSTANCE_TYPE` | EC2 instance type (default: m6i.xlarge) | `m6i.xlarge` |
| `AVAILABILITY_POLICY` | SingleReplica or HighlyAvailable | `SingleReplica` |
| `RELEASE_VERSION` | Optional: desired OCP version | `4.21.15` |

## Script Details

- **All scripts are idempotent** — safe to re-run, existing resources are skipped
- **State is stored in `.state/`** — resource IDs shared between scripts. In case of HCP spin-up failure, AWS infra-id will be saved for orphan resources clean-up withoud an actual Hosted Cluster

## Cleanup

```bash
./04-destroy-hc.sh                # Interactive confirmation
./04-destroy-hc.sh --yes          # Skip confirmation
./04-destroy-hc.sh --cleanup-dns  # Also remove Route53 sub-zone
```

## Contribution
Open an **Issue** to open a discussion on potetial changes or a **PR** for change proposals.
