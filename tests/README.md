# observability-e2e-test

[![Build](https://img.shields.io/badge/build-Prow-informational)](https://prow.ci.openshift.org/?repo=stolostron%2F${observability-e2e-test})

This is modeled after: <https://github.com/stolostron/open-cluster-management-e2e>

This is a container which will be called from:

1. Canary Tests
2. Regular Build PRs

The tests in this container will:

1. Create the object store and MCO CR.
2. Wait for the the entire Observability suite (Hub and Addon) installed.
3. Then verify the Observability suite (Hub and Addon) are working as expected including disable/enable addon, grafana verify etc.

The tests can be running both locally and in [Openshift CI(based on Prow)](https://docs.ci.openshift.org/) in the following two kinds of environment:

1. a [KinD](https://kind.sigs.k8s.io/) cluster.
2. an OCP cluster with ACM installed with [deploy repo](https://github.com/stolostron/deploy).

## Run e2e testing automatically

The observability e2e testing can be running automatically in KinD cluster or OCP cluster.

### Run locally in KinD cluster

1. clone this repository and enter its root directory:

```bash
git clone git@github.com:stolostron/multicluster-observability-operator.git && cd multicluster-observability-operator
```

2. Optionally override the observability images to test the corresponding components by exporting the following environment variables before running e2e testing:

| Component Name | Image Environment Variable |
| --- | --- |
| multicluster-observability-operator | MULTICLUSTER_OBSERVABILITY_OPERATOR_IMAGE_REF |
| rbac-query-proxy | RBAC_QUERY_PROXY_IMAGE_REF |
| metrics-collector | METRICS_COLLECTOR_IMAGE_REF |
| endpoint-monitoring-operator | ENDPOINT_MONITORING_OPERATOR_IMAGE_REF |
| grafana-dashboard-loader | GRAFANA_DASHBOARD_LOADER_IMAGE_REF |
| observatorium-operator | OBSERVATORIUM_OPERATOR_IMAGE_REF |

For example, if you want to test `metrics-collector` image from `quay.io/<your_username_in_quay>/metrics-collector:test`, then execute the following command:

```bash
export METRICS_COLLECTOR_IMAGE_REF=quay.io/<your_username_in_quay>/metrics-collector:test
```

> _Note:_ By default, the command will try to install the Observability and its dependencies with images of latest [UPSTREAM snapshot tag](https://quay.io/repository/stolostron/acm-custom-registry?tab=tags).

3. Then simply execute the following command to run e2e testing in a KinD cluster:

```bash
make e2e-tests-in-kind
```

### Run locally in OCP cluster

If you only have an OCP cluster with ACM installed, then you can run observability e2e testing with the following steps.

1. clone this repository and enter its root directory:

```bash
git clone git@github.com:stolostron/multicluster-observability-operator.git && cd multicluster-observability-operator
```

2. export `KUBECONFIG` environment variable to the kubeconfig of the OCP cluster:

```bash
export KUBECONFIG=<kubeconfig-file-of-the-ocp-cluster>
```

3. Optionally override the observability images to test the corresponding components by exporting the following environment variables before running e2e testing:

| Component Name | Image Environment Variable |
| --- | --- |
| multicluster-observability-operator | MULTICLUSTER_OBSERVABILITY_OPERATOR_IMAGE_REF |
| rbac-query-proxy | RBAC_QUERY_PROXY_IMAGE_REF |
| metrics-collector | METRICS_COLLECTOR_IMAGE_REF |
| endpoint-monitoring-operator | ENDPOINT_MONITORING_OPERATOR_IMAGE_REF |
| grafana-dashboard-loader | GRAFANA_DASHBOARD_LOADER_IMAGE_REF |
| observatorium-operator | OBSERVATORIUM_OPERATOR_IMAGE_REF |

4. Then simply execute the following command to run e2e testing:

```bash
make e2e-tests
```

## Run e2e testing manually

If you want to run observability e2e testing manually, make sure you have cluster with ACM installed.

## Running e2e testing manually

1. clone this repository and enter its root directory:

```bash
git clone git@github.com:stolostron/multicluster-observability-operator.git && cd multicluster-observability-operator
```

2. Before running the e2e testing, make sure [ginkgo](https://github.com/onsi/ginkgo) is installed:

```bash
go install github.com/onsi/ginkgo/ginkgo@latest
```

3. Then copy `tests/resources/options.yaml.template` to `tests/resources/options.yaml`, and update values specific to your environment:

```bash
cp tests/resources/options.yaml.template tests/resources/options.yaml
cat tests/resources/options.yaml
options:
  hub:
    name: HUB_CLUSTER_NAME
    baseDomain: BASE_DOMAIN
```

(optional) If there is an imported cluster in the test environment, need to add the cluster info into `options.yaml`:

```bash
cat tests/resources/options.yaml
options:
  hub:
    name: HUB_CLUSTER_NAME
    baseDomain: BASE_DOMAIN
  clusters:
  - name: IMPORT_CLUSTER_NAME
    baseDomain: IMPORT_CLUSTER_BASE_DOMAIN
    kubecontext: IMPORT_CLUSTER_KUBE_CONTEXT
```

4. Then run e2e testing manually by executing the following command:

```bash
export BUCKET=YOUR_S3_BUCKET
export REGION=YOUR_S3_REGION
export AWS_ACCESS_KEY_ID=YOUR_S3_AWS_ACCESS_KEY_ID
export AWS_SECRET_ACCESS_KEY=YOUR_S3_AWS_SECRET_ACCESS_KEY
export KUBECONFIG=~/.kube/config
ginkgo -v tests/pkg/tests/ -- -options=../../resources/options.yaml -v=3
```

(optional) If there is an imported cluster in the test environment, need to set more environment.

```bash
export IMPORT_KUBECONFIG=~/.kube/import-cluster-config
```

## Running e2e testing manually in docker container

1. clone this repository and enter its root directory:

```bash
git clone git@github.com:stolostron/multicluster-observability-operator.git && cd multicluster-observability-operator
```

2. Optionally build docker image for observability e2e testing:

```bash
docker build -t observability-e2e-test:latest -f tests/Dockerfile .
```

3. Then copy `tests/resources/options.yaml.template` to `tests/resources/options.yaml`, and update values specific to your environment:

```bash
cp tests/resources/options.yaml.template tests/resources/options.yaml
cat tests/resources/options.yaml
options:
  hub:
    name: HUB_CLUSTER_NAME
    baseDomain: BASE_DOMAIN
```

(optional)If there is an imported cluster in the test environment, need to add the cluster info into `options.yaml`:

```bash
cat tests/resources/options.yaml
options:
  hub:
    name: HUB_CLUSTER_NAME
    baseDomain: BASE_DOMAIN
  clusters:
  - name: IMPORT_CLUSTER_NAME
    baseDomain: IMPORT_CLUSTER_BASE_DOMAIN
    kubecontext: IMPORT_CLUSTER_KUBE_CONTEXT 
```

4. copy `tests/resources/env.list.template` to `tests/resources/env.list`, and update values specific to your s3 configuration:

```bash
cp tests/resources/env.list.template tests/resources/env.list
cat tests/resources/env.list
BUCKET=YOUR_S3_BUCKET
REGION=YOUR_S3_REGION
AWS_ACCESS_KEY_ID=YOUR_S3_AWS_ACCESS_KEY_ID
AWS_SECRET_ACCESS_KEY=YOUR_S3_AWS_SECRET_ACCESS_KEY
```

5. login to your cluster in which observability is enabled - and make sure that the kubeconfig is located as file `~/.kube/config`:

```bash
kubectl config current-context
admin
```

6. (optional) If there is an imported cluster in the test environment, you need to copy the kubeconfig file into as `~/.kube/` as `import-kubeconfig`:

```bash
cp {IMPORT_CLUSTER_KUBE_CONFIG_PATH} ~/.kube/import-kubeconfig
```

7. start to run e2e testing in docker container with the following command:

```bash
docker run -v ~/.kube/:/opt/.kube -v $(pwd)/tests/results:/results -v $(pwd)/tests/resources:/resources --env-file $(pwd)/tests/resources/env.list observability-e2e-test:latest
```

In Canary environment, this is the container that will be run - and all the volumes etc will passed on while starting the docker container using a helper script.

## Contributing to E2E

### Options.yaml

The values in the options.yaml are optional values read in by E2E. If you do not set an option, the test case that depends on the option should skip the test. The sample values in the option.yaml.template should provide enough context for you fill in with the appropriate values. Further, in the section below, each test should document their test with some detail.

### Skip install and uninstall

For developing and testing purposes, you can set the following env to skip the install and uninstall steps to keep your current MCO instance.

- SKIP_INSTALL_STEP:  if set to `true`, the testing will skip the install step
- SKIP_UNINSTALL_STEP:  if set to `true`, the testing will skip the uninstall step

For example, run the following command will skip the install and uninstall step:

```bash
export SKIP_INSTALL_STEP=true
export SKIP_UNINSTALL_STEP=true
export BUCKET=YOUR_S3_BUCKET
export REGION=YOUR_S3_REGION
export AWS_ACCESS_KEY_ID=YOUR_S3_AWS_ACCESS_KEY_ID
export AWS_SECRET_ACCESS_KEY=YOUR_S3_AWS_SECRET_ACCESS_KEY
export KUBECONFIG=~/.kube/config
ginkgo -v -- -options=resources/options.yaml -v=3
```

### Focus Labels

- Each `It` specification should end with a label which helps automation segregate running of specs.
- The choice of labels is up to the contributor, with the one guideline, that the second label, be `g0-gN`, to indicate the `run level`, with `g0` denoting that this test runs within a few minutes, and `g5` denotes a testcase that will take > 30 minutes to complete. See examples below:

`It("should have not the expected MCO addon pods (addon/g0)", func() {`

Examples:

```bash
  It("should have the expected args in compact pod (reconcile/g0)", func() {
  It("should work in basic mode (reconcile/g0)", func() {
  It("should have not the expected MCO addon pods (addon/g0)", func() {
  It("should have not metric data (addon/g0)", func() {
  It("should be able to access the grafana console (grafana/g0)", func() {
  It("should have metric data in grafana console (grafana/g0)", func() {
    ....
```

- The `--focus` and `--skip` are ginkgo directives that allow you to choose what tests to run, by providing a REGEX express to match. Examples of using the focus:

  - `ginkgo --focus="g0"`
  - `ginkgo --focus="grafana/g0"`
  - `ginkgo --focus="addon"`

- To run with verbose ginkgo logging pass the `--v`
- To run with klog verbosity, pass the `--focus="g0" -- -v=3` where 3 is the log level: 1-3
