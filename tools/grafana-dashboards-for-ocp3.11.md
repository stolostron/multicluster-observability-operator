# Import Grafana Dashboards for OCP 3.11 clusters

_Note:_: The grafana dashboards for OCP 3.11 clusters are provided in ACM 2.5 and above out-of-the-box. If you're running ACM < 2.5, you can follow this guide to import the grafana dashboards for OCP 3.11 clusters manually.

## Prequisites

You must meet the following requirements to import OCP 3.11 clusters:

1. `oc` (ver. 4.3+) & `kubectl` (ver. 1.16+) configured to connect to your ACM hub cluster
2. [jq](https://stedolan.github.io/jq/) command-line JSON processor >= 1.6
3. [gojsontoyaml](https://github.com/brancz/gojsontoyaml) command-line tool >=v0.1.0
4. [sed](https://www.gnu.org/software/sed/) command-line tool.

_Note:_ If you're running the steps in this document on MacOS, it is recommended to use GNU sed installed by `brew install gnu-sed`.

## Getting started

1. Login to ACM hub cluster via oc command line.

2. Clone the repository and check out the `multicluster-observability-operator` repository:

```bash
git clone git@github.com:stolostron/multicluster-observability-operator.git
cd multicluster-observability-operator
```

3. Create a configmap that contains the custom metrics allow list for OCP 3.11 clusters with the following commands:

```bash
curl -L https://raw.githubusercontent.com/open-cluster-management/multicluster-observability-operator/main/operators/multiclusterobservability/manifests/base/config/metrics_allowlist.yaml | gojsontoyaml --yamltojson | jq -r '.data."ocp311_metrics_list.yaml"' > /tmp/ocp311_metrics_list.yaml
oc -n open-cluster-management-observability create configmap observability-metrics-custom-allowlist --from-file=metrics_list.yaml=/tmp/ocp311_metrics_list.yaml
```

4. Load the OCP 3.11 dashboards to your ACM.

- For ACM <=2.3, running the following command:

```bash
find ./operators/multiclusterobservability/manifests/base/grafana -name "*-ocp311.yaml" -exec sed -i 's/clusterType=\\"ocp3\\",//g' {} \;
find ./operators/multiclusterobservability/manifests/base/grafana -name "*-ocp311.yaml" -exec sed -i 's/clusterType=\\"ocp3\\"//g' {} \;
find ./operators/multiclusterobservability/manifests/base/grafana -name "*-ocp311.yaml" -exec sed -i '/namespace:/a\ \ labels:' {} \;
find ./operators/multiclusterobservability/manifests/base/grafana -name "*-ocp311.yaml" -exec sed -i '/labels:/a\ \ \ \ grafana-custom-dashboard: "true"' {} \;
find ./operators/multiclusterobservability/manifests/base/grafana -name "*-ocp311.yaml" -exec oc apply -n open-cluster-management-observability -f {} \;
```

- For ACM 2.4, running the following command:

```bash
find ./operators/multiclusterobservability/manifests/base/grafana -name "*-ocp311.yaml" -exec sed -i '/namespace:/a\ \ labels:' {} \;
find ./operators/multiclusterobservability/manifests/base/grafana -name "*-ocp311.yaml" -exec sed -i '/labels:/a\ \ \ \ grafana-custom-dashboard: "true"' {} \;
find ./operators/multiclusterobservability/manifests/base/grafana -name "*-ocp311.yaml" -exec oc apply -n open-cluster-management-observability -f {} \;
```

5. Then open the Grafana console and switch to dashboards page, you should see the dashboards for OCP 3.11 clusters are under `OCP 3.11` folder:

_Note:_ For ACM 2.4, the cluster overview dashboard for OCP 3.11 clusters is located in the `General` folder for legency reasons.

![ocp311-dashboards-example.png](ocp311-dashboards-example.png)
