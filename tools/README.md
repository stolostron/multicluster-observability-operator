# How to design a grafana dashboard

## Prerequisites

You must enable the observability service by creating a MultiClusterObservability CustomResource (CR) instance.

## Setup grafana develop instance

Firstly, you should use this script `setup-grafana-dev.sh` to setup your grafana instance.

```
$ ./setup-grafana-dev.sh --deploy
secret/grafana-dev-config created
deployment.apps/grafana-dev created
service/grafana-dev created
ingress.extensions/grafana-dev created
```

## Switch user to be grafana admin

Secondly, you need to ask a user to login `https://$ACM_URL/grafana-dev/` before use this script `switch-to-grafana-admin.sh` to switch the user to be a grafana admin.

```
$ ./switch-to-grafana-admin.sh kube:admin
User <kube:admin> switched to be grafana admin
```

## Design your grafana dashboard

Now, refresh the grafana console and follow these steps to design your dashboard:

1. Click the **+** icon on the left panel, select **Create Dashboard**, and then click **Add new panel**.
2. In the New Dashboard/Edit Panel view, go to the **Query** tab.
3. Configure your query by selecting `Observatorium` from the data source selector and enter a PromQL query.
4. Click the **Save** icon in the top right corner of your screen to save the dashboard.
5. Add a descriptive name, and then click **Save**.

You can use this script `generate-dashboard-configmap-yaml.sh` to generate a dashboard configmap and save it to local.

```
./generate-dashboard-configmap-yaml.sh "Your Dashboard Name"
Save dashboard <your-dashboard-name> to ./your-dashboard-name.yaml
```

If you have not permission to run this script `generate-dashboard-configmap-yaml.sh`, you can following these steps to create a dashboard configmap:

1. Go to a dashboard, click the **Dashboard settings** icon.
2. Click the **JSON Model** icon on the left panel.
3. Copy the dashboard json data and put it in to `$your-dashboard-name` field.
4. Modify `$your-dashboard-name` field.

```yaml
kind: ConfigMap
apiVersion: v1
metadata:
  name: $your-dashboard-name
  namespace: open-cluster-management-observability
  labels:
    grafana-custom-dashboard: "true"
data:
  $your-dashboard-name.json: |
    $your_dashboard_json
```

Note: if your dashboard is not in `General` folder,  you can specify the folder name in `annotations` of this ConfigMap:
```
annotations:
  observability.open-cluster-management.io/dashboard-folder: Custom
```

6. Update metrics allowlist

When you generate a new dashboard like [example/custom-dashboard.yaml](example/custom-dashboard.yaml), there may have no data when you first create it. This is because it depends on some new metrics which don't upload to hub by default. You also need to update custom metrics allowlist, so that new metrics can be uploaded to the server and shown in dashboard. In this example, run the following command to update metrics.
```yaml
oc apply -f observability-metrics-custom-allowlist.yaml
```

## Uninstall grafana develop instance

You can use the following command to uninstall your grafana instance.

```
$ ./setup-grafana-dev.sh --clean
secret "grafana-dev-config" deleted
deployment.apps "grafana-dev" deleted
service "grafana-dev" deleted
ingress.extensions "grafana-dev" deleted
```
