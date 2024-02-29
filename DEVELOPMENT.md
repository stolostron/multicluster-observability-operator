# Development

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

Firstly, remove any existing `operator-sdk` binary from your $PATH.

To build the `operator-sdk` from source, run:

```shell
git clone https://github.com/operator-framework/operator-sdk
cd operator-sdk
git fetch origin --tags
git checkout v1.4.2
make install
```

To install the `operator-sdk` and `kustomize`, run:

```shell
make install-build-deps
```

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
