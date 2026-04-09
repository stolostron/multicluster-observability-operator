# Dev Scripts

Scripts for setting up a real OCP cluster to test the Multi-Cluster Observability Operator.
These are developer tools, not intended for production use.

## Prerequisites

- `oc` logged into your target OCP cluster
- `gettext` / `envsubst` (`brew install gettext` on macOS, `dnf install gettext` on Fedora)
- `jq` — optional but recommended; enables per-component progress display during MCH install/teardown, and required by `enable-mcoa-uwl.sh`

## Configuration

All scripts automatically load variables from `dev-scripts/.env` if it exists.
Variables already set in your shell take precedence, so CLI overrides still work:

```bash
cp dev-scripts/.env.example dev-scripts/.env
# edit dev-scripts/.env
```

```bash
# .env is loaded automatically — no need to repeat this on every command
ACM_VERSION=2.16
MCE_VERSION=2.11
```

```bash
# CLI override still wins over .env
ACM_VERSION=2.17 ./dev-scripts/setup-upstream-catalog.sh
```

See `.env.example` for all supported variables.

---

## Path A — Fresh install from the standard OLM catalog

Use this on a bare cluster with no ACM installed, when you don't need downstream dev builds.

```bash
ACM_VERSION=2.16 ./dev-scripts/setup-upstream-catalog.sh
./dev-scripts/setup-observability.sh
```

If ACM is already installed and running, skip straight to:

```bash
./dev-scripts/setup-observability.sh
```

---

## Path B — Downstream dev builds

Use this when you need to test against internal dev images from `quay.io:443/acm-d`.

### 1. Add quay.io credentials to the cluster pull secret

The `quay.io:443/acm-d` repository requires Red Hat employee credentials. You need
your personal quay.io CLI token (not your password) — find it at
**quay.io → Account Settings → CLI Password → Generate Encrypted Password**.

Add your credentials to `dev-scripts/.env` (gitignored, never commit it):

```bash
# If you haven't created .env yet:
cp dev-scripts/.env.example dev-scripts/.env
# edit dev-scripts/.env and fill in QUAY_USER and QUAY_TOKEN
```

Then run:

```bash
./dev-scripts/add-pull-secret.sh
```

Or pass them inline without the file:

```bash
QUAY_USER=rh-ee-you QUAY_TOKEN=<your-cli-token> ./dev-scripts/add-pull-secret.sh
```

### 2. Install dev catalog sources and ACM

Latest dev snapshot:

```bash
ACM_VERSION=2.16 MCE_VERSION=2.11 ./dev-scripts/setup-downstream-catalog.sh
```

Specific downstream build or release (the OLM channel stays `release-2.16` / `stable-2.11`):

```bash
# Pinned snapshot
ACM_VERSION=2.16 MCE_VERSION=2.11 \
ACM_CATALOG_TAG=2.16.1-DOWNSTREAM-2026-03-30-06-49-38 \
MCE_CATALOG_TAG=2.11.1-DOWNSTREAM-2026-03-30-06-49-38 \
  ./dev-scripts/setup-downstream-catalog.sh

# Released version
ACM_VERSION=2.16 MCE_VERSION=2.11 \
ACM_CATALOG_TAG=v2.16.1 MCE_CATALOG_TAG=v2.11.1 \
  ./dev-scripts/setup-downstream-catalog.sh
```

This applies an `ImageDigestMirrorSet` (no node reboots required, unlike the legacy `ImageContentSourcePolicy`),
installs the OLM catalog sources, and creates + waits for the `MultiClusterHub` CR.

### 3. Deploy observability

```bash
./dev-scripts/setup-observability.sh
```

---

## Testing a PR build with image overrides

### This-repo components

All five components built from this repo share a snapshot tag. Override them all at once with `TAG`:

```bash
TAG=2.17.0-SNAPSHOT-2026-04-01-11-07-35 ./dev-scripts/image-override.sh
```

Override the registry (default: `quay.io/stolostron`):

```bash
REGISTRY=quay.io/my-fork TAG=2.17.0-SNAPSHOT-... ./dev-scripts/image-override.sh
```

Pin one component to a different tag while the rest use `TAG`:

```bash
TAG=2.17.0-SNAPSHOT-... MCO_TAG=2.17.0-SNAPSHOT-other-build ./dev-scripts/image-override.sh
```

Per-component `*_TAG` variables:

| Variable | Component |
|---|---|
| `MCO_TAG` | `multicluster-observability-operator` |
| `ENDPOINT_TAG` | `endpoint-monitoring-operator` |
| `METRICS_COLLECTOR_TAG` | `metrics-collector` |
| `RBAC_QUERY_PROXY_TAG` | `rbac-query-proxy` |
| `GRAFANA_DASHBOARD_LOADER_TAG` | `grafana-dashboard-loader` |

### External components

These are not built from this repo and must be set explicitly. Each has an independent `*_TAG` and `*_REGISTRY` (defaults to the global `REGISTRY` when not set):

| TAG variable | REGISTRY variable | Component |
|---|---|---|
| `MCOA_ADDON_TAG` | `MCOA_ADDON_REGISTRY` | `multicluster-observability-addon` |
| `OBSERVATORIUM_OPERATOR_TAG` | `OBSERVATORIUM_OPERATOR_REGISTRY` | `observatorium-operator` |
| `OBSERVATORIUM_TAG` | `OBSERVATORIUM_REGISTRY` | `observatorium` |
| `GRAFANA_TAG` | `GRAFANA_REGISTRY` | `grafana` |
| `THANOS_TAG` | `THANOS_REGISTRY` | `thanos` |
| `PROMETHEUS_ALERTMANAGER_TAG` | `PROMETHEUS_ALERTMANAGER_REGISTRY` | `prometheus-alertmanager` |
| `THANOS_RECEIVE_CONTROLLER_TAG` | `THANOS_RECEIVE_CONTROLLER_REGISTRY` | `thanos-receive-controller` |

Example — override Thanos from a custom registry alongside all repo components:

```bash
TAG=2.17.0-SNAPSHOT-... \
THANOS_TAG=v0.35.1 THANOS_REGISTRY=quay.io/thanos \
  ./dev-scripts/image-override.sh
```

For any component, a full image ref (`*_IMAGE`) takes precedence over `*_TAG`/`*_REGISTRY`:

```bash
MCO_IMAGE=quay.io/other/custom-name:tag ./dev-scripts/image-override.sh
```

### Reverting

```bash
./dev-scripts/image-override-revert.sh
```

---

## Enabling MCOA user-workload metrics

This sets up the MCOA addon to collect user-workload metrics from managed clusters.
Requires observability to be deployed first.

```bash
./dev-scripts/enable-mcoa-uwl.sh
```

This script:
1. Patches the `MultiClusterObservability` CR to enable platform + UWL collection
2. Creates a `ScrapeConfig` that federates `up`, `kube_node_info`, and `kube_pod_info`
3. Registers the `ScrapeConfig` in the `global` placement of the `ClusterManagementAddon`

---

## Switching between upstream and downstream catalogs

The catalog setup/teardown scripts are safe to run on a cluster with ACM already installed —
they patch subscriptions idempotently without uninstalling ACM.

**Upstream → Downstream** (ACM already installed from `redhat-operators`):
```bash
# Only needed if quay.io:443 credentials are not already in the cluster pull secret
./dev-scripts/add-pull-secret.sh

ACM_VERSION=2.16 MCE_VERSION=2.11 ./dev-scripts/setup-downstream-catalog.sh
```

**Downstream → Upstream**:
```bash
./dev-scripts/teardown-downstream-catalog.sh
```

---

## Teardown

### Remove only the observability stack

Deletes the `MultiClusterObservability` CR and waits for all observability pods to
terminate. MCH and ACM are left running.

```bash
./dev-scripts/teardown-observability.sh
```

### Switch back to the standard catalog

Patches the ACM and MCE subscriptions back to `redhat-operators`, removes the downstream
`CatalogSources` and `ImageDigestMirrorSet`. Namespaces, OperatorGroups, CSVs and ACM
itself are left intact — only the catalog wiring changes.

```bash
./dev-scripts/teardown-downstream-catalog.sh
```

### Fully remove ACM

Follows the OLM uninstall sequence: deletes the MCH CR (operator runs finalizer cleanup),
then removes CSVs, Subscriptions, and both namespaces. Run `teardown-observability.sh`
first if MCO is still deployed.

```bash
./dev-scripts/teardown-observability.sh   # if observability is deployed
./dev-scripts/teardown-acm.sh
```

---

### Typical workflows

**Long-running cluster — swap catalog and redeploy:**
```
teardown-observability.sh          # remove MCO stack
teardown-downstream-catalog.sh     # switch back to redhat-operators
image-override.sh                  # test a specific PR build on top
setup-observability.sh             # re-deploy the MCO stack
```

**Full reset — clean cluster for a fresh install:**
```
teardown-observability.sh          # remove MCO stack (if deployed)
teardown-acm.sh                    # fully remove ACM and MCE
setup-upstream-catalog.sh          # reinstall from scratch
setup-observability.sh
```

---

## Running e2e tests

`run-e2e.sh` is a thin wrapper around `dev-scripts/cmd/run-e2e`, a Go tool that:

1. Connects to the hub cluster and lists `ManagedCluster` resources
2. Matches each managed cluster to a kubecontext by comparing API server URLs
3. Verifies connectivity to each reachable cluster
4. Generates `tests/resources/options.yaml`
5. Invokes `ginkgo` with the requested focus/skip patterns

MCO is assumed to be already deployed, so install and uninstall steps are skipped by default.

Requires `ginkgo` on your PATH:
```bash
go install github.com/onsi/ginkgo/v2/ginkgo@latest
```

```bash
# Hub = current context; spokes auto-discovered from ManagedCluster resources
./dev-scripts/run-e2e.sh

# Explicit hub context (spokes still auto-discovered)
./dev-scripts/run-e2e.sh --hub ctx-hub

# Focus on a specific area (any substring of the test label works)
./dev-scripts/run-e2e.sh --focus mcoa
./dev-scripts/run-e2e.sh --focus metrics --focus alert

# Run only fast tests (labelled g0)
./dev-scripts/run-e2e.sh --focus g0

# Skip specific tests
./dev-scripts/run-e2e.sh --skip grafana
./dev-scripts/run-e2e.sh --focus mcoa --skip requires-ocp

# Let the suite install/uninstall MCO itself
./dev-scripts/run-e2e.sh --install --uninstall
```

Per-cluster kubeconfigs are written to `tests/resources/kubeconfigs/` (gitignored).
Results are written to `tests/pkg/tests/results.xml`.

---

## Notes

- **MinIO storage is ephemeral** — all Thanos data is lost if the MinIO pod restarts.
  This is intentional for dev clusters where persistence is not needed.
- MinIO credentials: `minioadmin` / `minioadmin`
- The MinIO console URL is printed at the end of `setup-observability.sh`
