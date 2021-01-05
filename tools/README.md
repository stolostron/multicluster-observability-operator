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

## Uninstall grafana develop instance

You can use the following command to uninstall your grafana instance.

```
$ ./setup-grafana-dev.sh --clean
secret "grafana-dev-config" deleted
deployment.apps "grafana-dev" deleted
service "grafana-dev" deleted
ingress.extensions "grafana-dev" deleted
```

## Swith user to be grafana admin

Secondly, you should use this script `switch-to-grafana-admin.sh` to switch a user to be a grafana admin, and then use this user to manage dashboards, manage folders, manage users, etc.

```
$ ./switch-to-grafana-admin.sh kube:admin
User <kube:admin> switched to be grafana admin
```

## Design your grafana dashboard

Now, you can log in to grafana via access `https://ACM_URL/grafana-dev/`. and following these steps to design your dashboard:

1. Click the **+** icon on the left panel, select **Create Dashboard**, and then click **Add new panel**.
2. In the New Dashboard/Edit Panel view, go to the **Query** tab.
3. Configure your query by selecting `-- Grafana --` from the data source selector. This generates the Random Walk dashboard.
4. Click the **Save** icon in the top right corner of your screen to save the dashboard.
5. Add a descriptive name, and then click **Save**.

You can use this script `generate-dashboard-configmap-yaml.sh` to generate a dashboard configmap and save it to local.

```
./generate-dashboard-configmap-yaml.sh your_dashboard_name
Save dashboard <your_dashboard_name> to ./your_dashboard_name.yaml
```

If you have not permission to run this script `generate-dashboard-configmap-yaml.sh`, you can following these steps to create a dashboard configmap:

1. Go to a dashboard, click the **Dashboard settings** icon.
2. Click the **JSON Model** icon on the left panel.
3. Copy the dashboard json data and put it in to `$your_dashboard_json` field.
4. Modify `$your_dashboard_name` field.

```yaml
kind: ConfigMap
apiVersion: v1
metadata:
  name: $your_dashboard_name
  namespace: open-cluster-management-observability
  labels:
    grafana-custom-dashboard: "true"
data:
  $your_dashboard_name.json: |
    $your_dashboard_json
```