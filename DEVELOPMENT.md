# Development test

This document describes the process for build, running and testing this application locally.

## Building

### Installing operator-sdk

This project is built using [operator-sdk](https://github.com/operator-framework/operator-sdk).
There is a hard dependency on version `1.4.2` at the time of writing.

> [!NOTE]
> Due to this [controller-gen issue](https://github.com/kubernetes-sigs/controller-tools/issues/880)
> the steps below cannot be built using Go 1.22.x.

> [!NOTE]
> For macOS users on arm64 you may need to build the binary from source using the steps below.

Any dependencies that already exist will be skipped.

### Building the bundle

To generate and validate bundle manifests and metadata run:

```shell
make bundle
```

### Building the binary

To build the binary, run:

```shell
make build
```

### Building the image

To build the image, which requires access to Red Hat CI registry, run:

```shell
make docker-build
```

# E2E Testing

> [!NOTE]
> For macOS users, you will need to install `gsed` using `brew install gnu-sed`

## Running in KinD

Firstly, bring up the environment:

```shell
make mco-kind-env
```

This will create a cluster with MCO installed and the operator running.

Then, run the tests:

```shell
make e2e-tests-in-kind
```

By default, if the tests fail, the ACM components will be removed.
We can bypass this behaviour by running:

```shell
SKIP_UNINSTALL_STEP=true make e2e-tests-in-kind
```

Likewise, we can also bypass the environment setup by running:

```shell
SKIP_INSTALL_STEP=true make e2e-tests-in-kind
```

After running the tests, there will be some changes made to your environment.
We can clean up the environment by running:

```shell
rm "examples/mco/e2e/v1beta1/observability.yaml-e" || true
rm "examples/mco/e2e/v1beta2/observability.yaml-e" || true
rm "operators/multiclusterobservability/manifests/base/grafana/deployment.yaml-e" || true
git stash
```
