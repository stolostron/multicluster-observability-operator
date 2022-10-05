### Goal of the 3 policies
If apply these policies to a managed cluster, it will configure the Prometheus of the managed cluster to forward all it alerts to the alertmanager on the ACM hub. It does so by configuring the cluster-monitoring-config configmap in openshift-monitoring namespace of the managed cluster. This cannot be applied to ACM hub unless [this feature](https://issues.redhat.com/browse/ACM-1756) is delivered.

### Notes
- When the policies are deleted, we do not delete the resources create by the policies. This is just to play it safe because we are dealing with cluster-monitoring-config configmap - a critical item in configuring OCP monitoring.
- we cannot create policy set out of these because these policies need to be created in different namespaces as it accesses resources in those namespaces in the ACM Hub for hub side templating.
- These polices will be supported on ACM 2.6; Not sure if ACM 2.5 will be able to support these levels of hub side and managed cluster side templates. However, it is not neccessary that these resources be delivered through policy either. As long as the right resources - configmap, secrets are delivered with correct content, the function will be served.

### Description

Policy | Description | Additional Info
--- | --- | ---
policy-hub-alert-routing.yaml| This needs to be created in openshift-ingress namespace. This takes the CA certs needed  to complete the router handshake and injects them into a secret in the openshift-monitoring namespace. | Needless to say, if custom certificates are used, appropriate changes need to be made to this policy.
policy-cluster-monitoring-config.yaml| This needs to be created in open-cluster-management-observability namespace. This creates the real cluster-monitoring-config configmap in the openshift-monitoring namespace. This usese both hub side and managed cluster side templating. | The other 2 polices creates secrets referred to by this configmap. What happens if the "cluster" label is changed.
policy-obs-alm-accessor.yaml|This needs to be created in open-cluster-management-observability namespace. This takes the bearer token required by the service account using which one can log into the ACM hub alertmanager and injects them into a secret in the openshift-monitoring namespace.|

