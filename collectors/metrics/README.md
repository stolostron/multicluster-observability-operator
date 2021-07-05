Metrics Collector
-----------
Metrics Collector implements a client to "scrape" or collect data from OpenShift Promethus
and performs a push fedration to a Thanos instance hosted by Red Hat Advanced Cluster Management for Kubernetes 
hub cluster. This project is based on the [Telemeter project](https://github.com/openshift/telemeter).


Get started
-----------
To execute the unit test suite, run

```
make -f Makefile.prow test-unit
```

To build docker image and push to a docker repository, run

```
docker build -t {REPO}/metrics-collector:latest .
docker push {REPO}/metrics-collector:latest
```
{REPO} is the docker repository


Integration environment
-----------
Prerequisites:
Commands [kind](https://kind.sigs.k8s.io/) and [kubectl](https://kubernetes.io/docs/tasks/tools/#kubectl) are required to setup an integration environment. To install them, run:
```
./test/integration/prereq.sh
```
If the image is pushed to a private repo which requires authentication, need to export the user/password for the docker repository before run setup.sh
```
export DOCKER_USER=<USER>
export DOCKER_PASS=<PASSWORD>
```

To launch a self contained integration environment based on the image built above, run:

```
./test/integration/setup.sh {REPO}/metrics-collector:latest
```

Above command will create a Kind cluster. Then [prometheus](https://prometheus.io/) and [thanos](https://thanos.io/) will be deployed in the cluster. Finally, a deployment of metrics collector will be deployed, which will scrape metrics from prometheus and send metrics to thanos server.


To check/operate on the environment, run:
```
kubectl --kubeconfig $HOME/.kube/kind-config-hub {COMMAND}
```
{COMMAND} is the target kubectl command. e.g. to check the status for the deployed pods in the Kind cluster, run:
```
kubectl --kubeconfig $HOME/.kube/kind-config-hub get pods -n open-cluster-management-monitoring
```

To clean the integration environment, run:
```
./test/integration/clean.sh
```