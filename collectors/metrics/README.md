# Metrics Collector (Legacy)

> **⚠️ WARNING: LEGACY COMPONENT**
> This component is part of the legacy observability architecture and is being deprecated in favor of the **Multi-Cluster Observability Addon (MCOA)**. For new development and the current standard, please refer to the MCOA documentation.

-----------
Metrics Collector implements a client to "scrape" or collect data from OpenShift Prometheus
and performs a push federation to a Thanos instance hosted by Red Hat Advanced Cluster Management for Kubernetes
hub cluster. This project is based on the [Telemeter project](https://github.com/openshift/telemeter).

## Get started

-----------
To execute the unit test suite, run

```bash
make -f Makefile.prow test-unit
```

To build docker image and push to a docker repository, run

```bash
docker build -t {REPO}/metrics-collector:latest .
docker push {REPO}/metrics-collector:latest
```

{REPO} is the docker repository

## Integration environment

-----------
Prerequisites:
Commands [kind](https://kind.sigs.k8s.io/) and [kubectl](https://kubernetes.io/docs/tasks/tools/#kubectl) are required to set up an integration environment. To install them, run:

```bash
make tools
```

If the image is pushed to a private repo which requires authentication,
you will need to export the user/password for the docker repository before run setup.sh

```bash
export DOCKER_USER=<USER>
export DOCKER_PASS=<PASSWORD>
```

To launch a self-contained integration environment based on the image built above, run:

```bash
./test/integration/setup.sh {REPO}/metrics-collector:latest
```

Above command will create a Kind cluster. Then [prometheus](https://prometheus.io/) and [thanos](https://thanos.io/) will be deployed in the cluster.
Finally, a deployment of metrics collector will be deployed,
which will scrape metrics from Prometheus and send metrics to Thanos server.

To check/operate on the environment, run:

```bash
kubectl --kubeconfig $HOME/.kube/kind-config-hub {COMMAND}
```

{COMMAND} is the target kubectl command. e.g. to check the status for the deployed pods in the Kind cluster, run:

```bash
kubectl --kubeconfig $HOME/.kube/kind-config-hub get pods -n open-cluster-management-monitoring
```

To clean the integration environment, run:

```bash
./test/integration/clean.sh
```
