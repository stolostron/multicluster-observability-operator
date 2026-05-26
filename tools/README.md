# How to Design a Grafana Dashboard

## Prerequisites

You must enable the observability service by creating a MultiClusterObservability CustomResource (CR) instance.

## Setup Grafana Developer Instance

Use the script `setup-grafana-dev.sh` to setup your Grafana instance. You need to run this as a `kubeadmin` user.

```bash
$ ./setup-grafana-dev.sh --deploy
secret/grafana-dev-config created
deployment.apps/grafana-dev created
service/grafana-dev created
serviceaccount/grafana-dev created
clusterrolebinding.rbac.authorization.k8s.io/open-cluster-management:grafana-crb-dev created
route.route.openshift.io/grafana-dev created
persistentvolumeclaim/grafana-dev created
oauthclient.oauth.openshift.io/grafana-proxy-client-dev created
deployment.apps/grafana-dev patched
service/grafana-dev patched
route.route.openshift.io/grafana-dev patched
oauthclient.oauth.openshift.io/grafana-proxy-client-dev patched
clusterrolebinding.rbac.authorization.k8s.io/open-cluster-management:grafana-crb-dev patched

Grafana dev URL: grafana-dev-open-cluster-management-observability.apps.<basedomain>.com
```

## Switch User to be Grafana Admin

1. Log in to Grafana developer instance by navigating to the URL:  
   `https://grafana-dev-open-cluster-management-observability.apps.<basedomain>.com`  
   Use the credentials of the user who you want to be Grafana admin to complete the login.

2. Run the `switch-to-grafana-admin.sh` script to assign Grafana admin privileges to the logged-in user.  
 
   For example, if you want `kube:admin` to be Grafana admin, you would run:  
   ```bash
   $ ./switch-to-grafana-admin.sh kube:admin
   User <kube:admin> switched to be Grafana admin
   ```

   If another user, such as `frank`, is logged in, you would run:
   ```bash
   $ ./switch-to-grafana-admin.sh frank
   User frank switched to be Grafana admin
   ```

   Note: If you see an error like the one below, it may be because you did not log in to the Grafana developer instance in step (1) or you provided an invalid username.
   ```bash
   $ ./switch-to-grafana-admin.sh frank
   Failed to fetch user ID, please check your user name
   ```

## Design your Grafana Dashboard

Now, refresh the Grafana console and follow these steps to design your dashboard:

1. Click the **+** icon on the left panel, select **Create Dashboard**, and then click **Add new panel**.
2. In the New Dashboard/Edit Panel view, go to the **Query** tab.
3. Configure your query by selecting `Observatorium` from the data source selector and enter a PromQL query.
4. Click the **Save** icon in the top right corner of your screen to save the dashboard.
5. Add a descriptive name, and then click **Save**.

You can use this script `generate-dashboard-configmap-yaml.sh` to generate a dashboard configmap and save it to local.

```bash
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

```yaml
annotations:
  observability.open-cluster-management.io/dashboard-folder: Custom
```

5. Update Metrics List if Necessary 

When creating a new dashboard (e.g., [example/custom-dashboard.yaml](example/custom-dashboard.yaml)), it may be necessary to add new metrics using a custom allowlist config map if the new dashboard depends on new metrics which are not collected by default. Below is an example of how custom metrics can be added to support the example dashboard.

```bash
oc apply -f observability-metrics-custom-allowlist.yaml
```

## Uninstall Grafana Developer Instance

You can use the following command to uninstall your Grafana instance.

```bash
$ ./setup-grafana-dev.sh --clean
secret "grafana-dev-config" deleted
deployment.apps "grafana-dev" deleted
serviceaccount "grafana-dev" deleted
route.route.openshift.io "grafana-dev" deleted
persistentvolumeclaim "grafana-dev" deleted
oauthclient.oauth.openshift.io "grafana-proxy-client-dev" deleted
clusterrolebinding.rbac.authorization.k8s.io "open-cluster-management:grafana-crb-dev" deleted
```
