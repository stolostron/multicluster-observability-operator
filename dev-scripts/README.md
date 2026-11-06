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

## Run E2E against a custom ACM catalog

If ACM is already installed, `make e2e-tests` switches its existing
`acm-custom-registry` CatalogSource to the temporary managed log store catalog
configured in the Makefile, waits for OLM to upgrade the ACM CSV, and verifies
the MCH operator image before starting the tests:

```bash
export KUBECONFIG=/path/to/hub.kubeconfig
make e2e-tests
```

Prow leaves the ACM Subscription channel empty, so the script first sets it to
`release-2.17` before switching catalogs.

The Makefile defaults can be overridden with `CUSTOM_ACM_CATALOG_IMAGE` and
`EXPECTED_MCH_OPERATOR_IMAGE`. Set `CUSTOM_ACM_CATALOG_IMAGE=` to skip the
catalog switch. In Prow, `make e2e-tests` uses `${SHARED_DIR}/hub-1.kc` when no
`KUBECONFIG` is set. The bundle image is selected by the catalog and is not an
input to this script.

The custom catalog must contain a new CSV in the subscribed channel with an
upgrade path from the CSV already installed on the cluster. The script fails if
OLM does not perform that upgrade or if the running MCH or MCO operator image
does not match the expected image. The cluster must be able to pull the custom
catalog and bundle images.

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

## Building a custom ACM bundle for upgrade testing

`build-acm-bundle.sh` uses `just dev-branch-e2e` from a local
[`installer-dev-tools`](https://github.com/openshift/installer-dev-tools) checkout to
build a custom ACM catalog/bundle. Use this when end-to-end upgrade tests need an ACM
bundle containing a development change.

### 1. Prepare `installer-dev-tools`

Clone the upstream repository, add the development fork as a remote, and check out the
branch containing the required bundle-build changes:

```bash
git clone https://github.com/openshift/installer-dev-tools.git
cd installer-dev-tools
git remote add acm-upgrade git@github.com:GeorgeFrankAeillo/installer-dev-tools.git
git fetch acm-upgrade
git switch --track acm-upgrade/gaeillo/addUpgradeOption
```

The build script runs `just` from the custom-catalog directory. Install `just` and any
container/build tools required by the installer-dev-tools `dev-branch-e2e` recipe. You
also need access to the Quay account used to publish the resulting catalog image. Note: this is subject to change, the workaround to build the acm bundle is not merged to main in `installer-dev-tools` (and doesn't seem likely to soon)

### 2. Configure the build

Create the dev-scripts environment file and set the custom-bundle variables in it:

```bash
cp dev-scripts/.env.example dev-scripts/.env
```

For example, for an ACM 5.0 bundle (adjust versions, branches, paths, and tags to match
the ACM build and MCO change you are testing):

```dotenv
QUAY_USER=rh-ee-yourname
INSTALLER_DEV_TOOLS_DIR=/absolute/path/to/installer-dev-tools/scripts/custom-catalog
TARGET_CSV_VERSION=5.0.1-managedlogstore
FROM_CSV_VERSION=advanced-cluster-management.v5.0.0
BASE_ACM=release-5.0
BASE_CATALOG_TAG=latest-5.0
ACM_IMG_TAG=managed-logstore-test-5.0-amd64
COMPONENT_BRANCH=multicluster-observability-operator:main
```

`QUAY_USER` is passed to the build recipe and must be set; authenticate to Quay as needed
to push the generated image. Keep personal credentials out of committed files. The
variables mean:

| Variable | Purpose |
|---|---|
| `INSTALLER_DEV_TOOLS_DIR` | Path to `installer-dev-tools/scripts/custom-catalog`, where the script runs `just`. |
| `TARGET_CSV_VERSION` | Version suffix for the new ACM CSV, without the `advanced-cluster-management.v` prefix. |
| `FROM_CSV_VERSION` | Full currently-installed ACM CSV to upgrade from. |
| `BASE_ACM` | ACM release branch used for the operator and bundle source. |
| `BASE_CATALOG_TAG` | Base ACM catalog tag, such as `latest-5.0`. |
| `ACM_IMG_TAG` | Tag for the resulting custom ACM catalog image. |
| `COMPONENT_BRANCH` | MCO repository and branch to include, in `repository:branch` form. |

The `.env` file is loaded automatically by dev-scripts. Run the build from the
multicluster-observability-operator repository root:

```bash
./dev-scripts/build-acm-bundle.sh
```

### 3. Point upgrade tests at the generated bundle

When using the generated catalog in end-to-end upgrade testing, update the image and
target CSV in `cicd-scripts/upgrade-acm.sh` to match the image and CSV produced by the
build. For example:

```bash
ACM_IMG='quay.io/rh-ee-dbuchana/acm-dev-catalog:managed-logstore-test-5.0-amd64'
TARGET_CSV='advanced-cluster-management.v5.0.1-managedlogstore'
```

Before choosing the target CSV, check which ACM CSV is installed on the cluster. The
current CSV is also the value to use for `FROM_CSV_VERSION` when building an upgrade
bundle:

```bash
NAMESPACE=open-cluster-management
SUB=$(oc -n "$NAMESPACE" get subscriptions.operators.coreos.com -o json |
  jq -er '[.items[] | select(.spec.name == "advanced-cluster-management")] |
    if length == 1 then .[0].metadata.name else error("expected one ACM subscription") end')
oc -n "$NAMESPACE" get subscriptions.operators.coreos.com "$SUB" \
  -o jsonpath='{.status.currentCSV}'
```

Set `TARGET_CSV` to the full CSV name generated by the bundle (the prefix plus
`TARGET_CSV_VERSION`). It may need to change when building for a different ACM version.
This will run as part of cicd. Can also run the upgrade helper against your own cluster:

```bash
./cicd-scripts/upgrade-acm.sh
```

---

## Reclaiming cluster disk space

Long-running dev / collective SNO clusters frequently run into node `DiskPressure` evictions (e.g., MCO metrics collector pods evicted in a loop).

`clean-cluster-disk.sh` automates the remediation of the top space hogs:
1. **Recycles leaking `openshift-console/downloads` pods**: Each container restart leaks ~3.1 GB of oc binaries in `emptyDir` (`/tmp`) without cleaning up.
2. **Purges dead/evicted pods**: Removes lingering `Evicted` and `ContainerStatusUnknown` pods so CRI-O can release writable container layers.
3. **Deletes stale completed pods**: Cleans up old `openshift-insights` and `openshift-kube-apiserver` installer/pruner pods holding logs in `/var/log/pods`.
4. **Vacuums systemd journals**: Trims `/var/log/journal` to 500 MB.
5. **Prunes unused CRI-O images**: Runs `crictl rmi --prune` on the nodes via `machine-config-daemon`.

```bash
# Preview what would be cleaned without modifying anything
./dev-scripts/clean-cluster-disk.sh --dry-run

# Run full cleanup and verify node DiskPressure clears
./dev-scripts/clean-cluster-disk.sh
```

---

## Notes

- **SeaweedFS storage is ephemeral** — all Thanos data is lost if the SeaweedFS pod restarts.
  This is intentional for dev clusters where persistence is not needed.
- SeaweedFS S3 credentials: `seaweedfsadmin` / `seaweedfsadmin`
- The SeaweedFS Filer UI URL is printed at the end of `setup-observability.sh`
