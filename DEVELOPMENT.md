# Development

This document covers building, testing, and iterating on the Multi-Cluster Observability
Operator and its companion components.

## Prerequisites

- Go 1.25 (see `go.mod` for the exact version)
- `podman` or `docker` for container image builds
- `kubectl` / `oc` for cluster interaction
- `kustomize` for manifest generation

## Building Go binaries

The root `make build` target compiles the MCO operator binary:

```bash
make build
```

Individual components can be built directly with `go build`:

```bash
# MCO operator
go build -o bin/manager operators/multiclusterobservability/main.go

# Endpoint metrics operator (legacy)
go build -o bin/endpoint-monitoring-operator operators/endpointmetrics/main.go

# Metrics collector (legacy)
go build -o bin/metrics-collector collectors/metrics/cmd/metrics-collector/main.go

# RBAC query proxy
go build -o bin/rbac-query-proxy proxy/cmd/main.go

# Grafana dashboard loader
go build -o bin/grafana-dashboard-loader loaders/dashboards/cmd/main.go
```

## Building container images

Each of the five components built from this repo has a `Dockerfile.dev` that uses only
publicly available base images (`golang:1.25` builder, `ubi9/ubi-minimal` runtime) and
requires no registry authentication.

All builds must be run from the **repository root** because the Go module and shared
packages are at the root level.

The Makefile provides per-component targets and a combined target. By default images are
tagged `quay.io/stolostron/<component>:dev` — override `DEV_REGISTRY` and `DEV_TAG` for
your own fork:

```bash
# Build all five components
CONTAINER_ENGINE=podman DEV_REGISTRY=quay.io/<you> DEV_TAG=my-dev make docker-build-dev-all

# Or build a single component
CONTAINER_ENGINE=podman DEV_REGISTRY=quay.io/<you> make docker-build-dev-mco
```

Each component has three target families — build, push, build-and-push:

| Action | Single component | All components |
|---|---|---|
| Build | `docker-build-dev-<component>` | `docker-build-dev-all` |
| Push | `docker-push-dev-<component>` | `docker-push-dev-all` |
| Build + push | `docker-build-push-dev-<component>` | `docker-build-push-dev-all` |

Where `<component>` is one of: `mco`, `endpoint`, `metrics-collector`, `rbac-proxy`, `dashboard-loader`.

To target a specific architecture (default: `linux/amd64`):

```bash
CONTAINER_ENGINE=podman PLATFORM=linux/arm64 DEV_REGISTRY=quay.io/<you> make docker-build-dev-mco
```

All `Dockerfile.dev` files use BuildKit cache mounts (`--mount=type=cache,sharing=shared`)
for the Go module and build caches. Subsequent builds of the same component are
significantly faster because compiled packages are reused across invocations.

### Pushing images and testing on a cluster

Build, push, then use `dev-scripts/image-override.sh` to deploy on a live cluster:

```bash
CONTAINER_ENGINE=podman DEV_REGISTRY=quay.io/<you> DEV_TAG=my-dev make docker-build-push-dev-mco
MCO_IMAGE=quay.io/<you>/multicluster-observability-operator:my-dev ./dev-scripts/image-override.sh
```

See [`dev-scripts/README.md`](dev-scripts/README.md) for the full image override reference.

### CI images (Red Hat internal)

The `Dockerfile` files (no `.dev` suffix) use `registry.ci.openshift.org/stolostron/builder:go1.25-linux`,
which requires authenticating to the OpenShift CI registry:

```bash
oc registry login --registry registry.ci.openshift.org
```

`Containerfile.operator` files target the internal Brew/Konflux pipeline and use
`brew.registry.redhat.io`, which is only accessible to Red Hat employees with the
appropriate credentials.

## Code quality

```bash
make format          # auto-format Go and shell code
make go-lint         # run golangci-lint (works with uncommitted changes)
make lint            # full check: format + deps + copyright + golangci-lint (requires clean work tree)
make unit-tests      # run all unit tests
make integration-test # run integration tests (requires envtest)
```

## E2E testing in KinD

KinD-based E2E tests run the full stack locally. This is resource-intensive and rarely
needed for routine work — rely on CI for full system validation.

Bring up the environment:

```bash
make mco-kind-env
```

Run the tests:

```bash
make e2e-tests-in-kind
```

Skip install or uninstall phases independently:

```bash
SKIP_INSTALL_STEP=true make e2e-tests-in-kind
SKIP_UNINSTALL_STEP=true make e2e-tests-in-kind
```

## E2E testing on a real OCP cluster

`dev-scripts/run-e2e.sh` auto-discovers managed clusters from the hub, generates
`tests/resources/options.yaml`, and invokes `ginkgo`. MCO must already be deployed.

Requires `ginkgo`:

```bash
go install github.com/onsi/ginkgo/v2/ginkgo@latest
```

```bash
./dev-scripts/run-e2e.sh --focus mcoa
./dev-scripts/run-e2e.sh --focus metrics --skip grafana
```

See [`dev-scripts/README.md`](dev-scripts/README.md) for the full reference.
