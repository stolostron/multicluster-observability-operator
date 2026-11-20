// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

import (
	"context"
	"fmt"

	mcoconfig "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	appsv1 "k8s.io/api/apps/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"
)

const (
	clusterLoggingNamespace        = "openshift-logging"
	clusterLoggingSubscriptionName = "cluster-logging"
	clusterLoggingDeploymentName   = "cluster-logging-operator"
	managedLokiStackName           = "mcoa-logging-managed-storage"
)

// CheckCRDEstablished returns nil once the named CustomResourceDefinition exists on the given
// cluster and has its Established condition set to True, or an error describing why it isn't
// ready yet. It's intended to be used from within an Eventually() block, since OLM-driven CRD
// registration (e.g. Loki Operator installing the LokiStack CRD) is asynchronous.
func CheckCRDEstablished(cluster Cluster, crdName string) error {
	apiExtensionsClient := NewKubeClientAPIExtension(cluster.ClusterServerURL, cluster.KubeConfig, cluster.KubeContext)

	crd, err := apiExtensionsClient.ApiextensionsV1().CustomResourceDefinitions().Get(context.TODO(), crdName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get CRD %s on cluster %s: %w", crdName, cluster.Name, err)
	}

	for _, cond := range crd.Status.Conditions {
		if cond.Type == apiextensionsv1.Established && cond.Status == apiextensionsv1.ConditionTrue {
			return nil
		}
	}

	return fmt.Errorf("CRD %s on cluster %s is not Established yet", crdName, cluster.Name)
}

// CheckLokiStackCRDEstablished returns nil once Loki Operator's LokiStack CRD is Established on
// the given cluster, confirming Loki Operator (installed by MCO as a dependency of MCOA platform
// log collection) has completed its OLM install.
func CheckLokiStackCRDEstablished(cluster Cluster) error {
	return CheckCRDEstablished(cluster, mcoconfig.LokiStackCRDName)
}

// CheckLokiOperatorSubscriptionExists returns nil once the Loki Operator Subscription that MCO
// creates (via dependencies.EnsureLokiOperatorInstalled) exists on the given cluster.
func CheckLokiOperatorSubscriptionExists(cluster Cluster) error {
	clientDynamic := GetKubeClientDynamicWithCluster(cluster)

	sub, err := clientDynamic.Resource(NewSubscriptionGVR()).
		Namespace(mcoconfig.LokiOperatorNamespace).
		Get(context.TODO(), mcoconfig.LokiOperatorPackageName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("loki operator subscription %s/%s does not exist yet on cluster %s", mcoconfig.LokiOperatorNamespace, mcoconfig.LokiOperatorPackageName, cluster.Name)
		}
		return fmt.Errorf("failed to get Loki Operator subscription on cluster %s: %w", cluster.Name, err)
	}

	klog.V(1).Infof("Loki Operator subscription %s/%s found on cluster %s", sub.GetNamespace(), sub.GetName(), cluster.Name)
	return nil
}

// CheckClusterLoggingOperatorHealthy waits for OLM to finish installing CLO and for its
// Deployment to become available. A registered CLF CRD alone does not mean CLO can reconcile CLFs.
func CheckClusterLoggingOperatorHealthy(cluster Cluster) error {
	clientDynamic := GetKubeClientDynamicWithCluster(cluster)
	sub, err := clientDynamic.Resource(NewSubscriptionGVR()).
		Namespace(clusterLoggingNamespace).
		Get(context.TODO(), clusterLoggingSubscriptionName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get Cluster Logging Subscription on cluster %s: %w", cluster.Name, err)
	}
	if sub.GetDeletionTimestamp() != nil {
		return fmt.Errorf("Cluster Logging Subscription on cluster %s is deleting", cluster.Name)
	}
	csvName, found, err := unstructured.NestedString(sub.Object, "status", "installedCSV")
	if err != nil {
		return fmt.Errorf("read Cluster Logging Subscription installedCSV on cluster %s: %w", cluster.Name, err)
	}
	if !found || csvName == "" {
		return fmt.Errorf("Cluster Logging Subscription on cluster %s has no installedCSV yet", cluster.Name)
	}
	csv, err := clientDynamic.Resource(NewClusterServiceVersionGVR()).
		Namespace(clusterLoggingNamespace).
		Get(context.TODO(), csvName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get Cluster Logging CSV %s on cluster %s: %w", csvName, cluster.Name, err)
	}
	if err := checkClusterLoggingCSVStatus(csv); err != nil {
		return fmt.Errorf("Cluster Logging Operator on cluster %s: %w", cluster.Name, err)
	}
	deployment, err := GetDeploymentWithCluster(cluster, clusterLoggingDeploymentName, clusterLoggingNamespace)
	if err != nil {
		return fmt.Errorf("get Cluster Logging Deployment on cluster %s: %w", cluster.Name, err)
	}
	if err := checkClusterLoggingDeploymentStatus(deployment); err != nil {
		return fmt.Errorf("Cluster Logging Operator on cluster %s: %w", cluster.Name, err)
	}
	return nil
}

func checkClusterLoggingCSVStatus(csv *unstructured.Unstructured) error {
	if csv.GetDeletionTimestamp() != nil {
		return fmt.Errorf("CSV %s is deleting", csv.GetName())
	}
	phase, _, err := unstructured.NestedString(csv.Object, "status", "phase")
	if err != nil {
		return fmt.Errorf("read CSV %s phase: %w", csv.GetName(), err)
	}
	if phase != "Succeeded" {
		reason, _, _ := unstructured.NestedString(csv.Object, "status", "reason")
		message, _, _ := unstructured.NestedString(csv.Object, "status", "message")
		return fmt.Errorf("CSV %s phase is %q (reason: %s, message: %s)", csv.GetName(), phase, reason, message)
	}
	return nil
}

func checkClusterLoggingDeploymentStatus(deployment *appsv1.Deployment) error {
	if deployment.DeletionTimestamp != nil {
		return fmt.Errorf("Deployment %s/%s is deleting", deployment.Namespace, deployment.Name)
	}
	if deployment.Status.ObservedGeneration < deployment.Generation {
		return fmt.Errorf("Deployment %s/%s has not observed generation %d yet (observed: %d)",
			deployment.Namespace, deployment.Name, deployment.Generation, deployment.Status.ObservedGeneration)
	}
	desiredReplicas := int32(1)
	if deployment.Spec.Replicas != nil {
		desiredReplicas = *deployment.Spec.Replicas
	}
	if desiredReplicas == 0 {
		return fmt.Errorf("Deployment %s/%s has zero desired replicas", deployment.Namespace, deployment.Name)
	}
	if deployment.Status.ReadyReplicas < desiredReplicas {
		return FormatDeploymentNotReadyError(deployment, deployment.Namespace, deployment.Name)
	}
	if deployment.Status.AvailableReplicas < desiredReplicas {
		return fmt.Errorf("Deployment %s/%s has %d/%d available replicas", deployment.Namespace, deployment.Name,
			deployment.Status.AvailableReplicas, desiredReplicas)
	}
	return nil
}

// CheckMCOAManagedLokiStackHealthy waits until Loki Operator reports that the
// MCOA-managed LokiStack and its core components are Ready.
func CheckMCOAManagedLokiStackHealthy(cluster Cluster) error {
	clientDynamic := GetKubeClientDynamicWithCluster(cluster)
	stack, err := clientDynamic.Resource(NewLokiStackGVR()).
		Namespace(clusterLoggingNamespace).
		Get(context.TODO(), managedLokiStackName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get managed LokiStack on cluster %s: %w", cluster.Name, err)
	}
	if err := checkManagedLokiStackStatus(stack); err != nil {
		return fmt.Errorf("managed LokiStack on cluster %s: %w", cluster.Name, err)
	}
	return nil
}

func checkManagedLokiStackStatus(stack *unstructured.Unstructured) error {
	if stack.GetDeletionTimestamp() != nil {
		return fmt.Errorf("LokiStack %s/%s is deleting", stack.GetNamespace(), stack.GetName())
	}
	conditions, found, err := unstructured.NestedSlice(stack.Object, "status", "conditions")
	if err != nil {
		return fmt.Errorf("read LokiStack %s/%s conditions: %w", stack.GetNamespace(), stack.GetName(), err)
	}
	if !found {
		return fmt.Errorf("LokiStack %s/%s has no status conditions yet", stack.GetNamespace(), stack.GetName())
	}
	ready := false
	for _, item := range conditions {
		condition, ok := item.(map[string]any)
		if !ok || condition["type"] != "Ready" {
			continue
		}
		if condition["status"] != "True" {
			return fmt.Errorf("LokiStack %s/%s Ready=%v (reason: %v, message: %v)",
				stack.GetNamespace(), stack.GetName(), condition["status"], condition["reason"], condition["message"])
		}
		ready = true
		break
	}
	if !ready {
		return fmt.Errorf("LokiStack %s/%s has no Ready condition yet", stack.GetNamespace(), stack.GetName())
	}

	components, found, err := unstructured.NestedMap(stack.Object, "status", "components")
	if err != nil {
		return fmt.Errorf("read LokiStack %s/%s components: %w", stack.GetNamespace(), stack.GetName(), err)
	}
	if !found {
		return fmt.Errorf("LokiStack %s/%s has no component status yet", stack.GetNamespace(), stack.GetName())
	}
	for name, raw := range components {
		statuses, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("LokiStack %s/%s component %s has invalid status", stack.GetNamespace(), stack.GetName(), name)
		}
		for _, state := range [...]string{"Failed", "Pending", "Running"} {
			pods, _, err := unstructured.NestedStringSlice(statuses, state)
			if err != nil {
				return fmt.Errorf("read LokiStack %s/%s component %s %s pods: %w", stack.GetNamespace(), stack.GetName(), name, state, err)
			}
			if len(pods) > 0 {
				return fmt.Errorf("LokiStack %s/%s component %s has %s pods: %v", stack.GetNamespace(), stack.GetName(), name, state, pods)
			}
		}
	}
	// The ruler is optional, but these components must have Ready pods for this stack.
	for _, name := range [...]string{"compactor", "distributor", "gateway", "indexGateway", "ingester", "querier", "queryFrontend"} {
		statuses, ok := components[name].(map[string]any)
		if !ok {
			return fmt.Errorf("LokiStack %s/%s has no %s component status yet", stack.GetNamespace(), stack.GetName(), name)
		}
		pods, _, err := unstructured.NestedStringSlice(statuses, "Ready")
		if err != nil {
			return fmt.Errorf("read LokiStack %s/%s component %s Ready pods: %w", stack.GetNamespace(), stack.GetName(), name, err)
		}
		if len(pods) == 0 {
			return fmt.Errorf("LokiStack %s/%s component %s has no Ready pods yet", stack.GetNamespace(), stack.GetName(), name)
		}
	}
	return nil
}

// CheckMCOARootCertificateResourcesExist returns nil once the self-signed Issuer, root CA
// Certificate and cluster-scoped ClusterIssuer that MCO provisions for MCOA's logging default
// stack (via dependencies.EnsureMCOARootCertificatesInstalled) all exist on the given cluster.
func CheckMCOARootCertificateResourcesExist(cluster Cluster) error {
	clientDynamic := GetKubeClientDynamicWithCluster(cluster)

	if _, err := clientDynamic.Resource(NewCertManagerIssuerGVR()).
		Namespace(mcoconfig.CertManagerNamespace).
		Get(context.TODO(), mcoconfig.MCOARootCAIssuerName, metav1.GetOptions{}); err != nil {
		return fmt.Errorf("failed to get Issuer %s/%s on cluster %s: %w", mcoconfig.CertManagerNamespace, mcoconfig.MCOARootCAIssuerName, cluster.Name, err)
	}

	if _, err := clientDynamic.Resource(NewCertManagerCertificateGVR()).
		Namespace(mcoconfig.CertManagerNamespace).
		Get(context.TODO(), mcoconfig.MCOARootCertificateName, metav1.GetOptions{}); err != nil {
		return fmt.Errorf("failed to get Certificate %s/%s on cluster %s: %w", mcoconfig.CertManagerNamespace, mcoconfig.MCOARootCertificateName, cluster.Name, err)
	}

	if _, err := clientDynamic.Resource(NewCertManagerClusterIssuerGVR()).
		Get(context.TODO(), mcoconfig.MCOARootClusterIssuerName, metav1.GetOptions{}); err != nil {
		return fmt.Errorf("failed to get ClusterIssuer %s on cluster %s: %w", mcoconfig.MCOARootClusterIssuerName, cluster.Name, err)
	}

	return nil
}

// CheckClusterLogForwarderExists returns nil once at least one ClusterLogForwarder resource
// exists in the given namespace on the given cluster, confirming the MCOA addon manager has
// reconciled platform log collection onto that cluster.
func CheckClusterLogForwarderExists(cluster Cluster, namespace string) error {
	clientDynamic := GetKubeClientDynamicWithCluster(cluster)

	list, err := clientDynamic.Resource(NewClusterLogForwarderGVR()).Namespace(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list ClusterLogForwarders in namespace %s on cluster %s: %w", namespace, cluster.Name, err)
	}

	if len(list.Items) == 0 {
		return fmt.Errorf("no ClusterLogForwarder found in namespace %s on cluster %s", namespace, cluster.Name)
	}

	names := make([]string, 0, len(list.Items))
	for _, item := range list.Items {
		names = append(names, item.GetName())
	}
	klog.V(1).Infof("Found ClusterLogForwarder(s) %v in namespace %s on cluster %s", names, namespace, cluster.Name)
	return nil
}

// mcoaLoggingObjectStorageSecretYAML is the object storage secret the managed (MCOA-provisioned)
// LokiStack uses as its log store backend. It points at the same SeaweedFS instance the test
// environment already deploys for Thanos's object storage (examples/seaweedfs /
// examples/seaweedfs-tls), under a dedicated bucket so logs and metrics data don't collide.
const mcoaLoggingObjectStorageSecretYAML = `apiVersion: v1
kind: Secret
metadata:
  name: mcoa-logging-managed-storage-objstorage
  namespace: open-cluster-management-observability
type: Opaque
stringData:
  # SeaweedFS access key, set via AWS_ACCESS_KEY_ID on the seaweedfs Deployment
  access_key_id: seaweedfsadmin
  # SeaweedFS secret key, set via AWS_SECRET_ACCESS_KEY on the seaweedfs Deployment
  access_key_secret: seaweedfsadmin
  # Pre-created bucket name (single name or comma-separated list)
  bucketnames: acm-observability-logs
  # The Loki Secret is copied to openshift-logging, so use the Service's
  # namespace-qualified name and the HTTP listener dedicated to this test.
  endpoint: http://seaweedfs.open-cluster-management-observability.svc:8334
  # Still required — SeaweedFS is also a non-AWS S3-compatible store
  forcepathstyle: "true"
`

// CreateMCOALoggingObjectStorageSecret ensures the mcoa-logging-managed-storage-objstorage
// Secret exists on the hub (creating it if absent, updating it in place if it already exists),
// so the managed log store use case has an object storage target to provision the LokiStack
// against. Safe to call repeatedly / idempotently.
func CreateMCOALoggingObjectStorageSecret(opt TestOptions) error {
	return Apply(
		opt.HubCluster.ClusterServerURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext,
		[]byte(mcoaLoggingObjectStorageSecretYAML),
	)
}

// CheckMCOAManagedLoggingAODC verifies that the MCO CR selected the managed
// logging path without leaving the user-defined platform CLF path enabled.
func CheckMCOAManagedLoggingAODC(opt TestOptions) error {
	clientDynamic := NewKubeClientDynamic(
		opt.HubCluster.ClusterServerURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)
	aodc, err := clientDynamic.Resource(NewMCOAddOnDeploymentConfigGVR()).
		Namespace(MCO_NAMESPACE).
		Get(context.TODO(), MCOA_CLUSTER_MANAGEMENT_ADDON_NAME, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get managed logging AddOnDeploymentConfig: %w", err)
	}

	vars, _, err := unstructured.NestedSlice(aodc.Object, "spec", "customizedVariables")
	if err != nil {
		return fmt.Errorf("failed to read managed logging customizedVariables: %w", err)
	}

	managedLoggingFound := false
	for _, item := range vars {
		variable, ok := item.(map[string]any)
		if !ok {
			continue
		}
		switch variable["name"] {
		case "platformLogsDefault":
			managedLoggingFound = variable["value"] == "true"
		case "platformLogsCollection":
			return fmt.Errorf("user-defined platform ClusterLogForwarder is still enabled in the AddOnDeploymentConfig")
		}
	}
	if !managedLoggingFound {
		return fmt.Errorf("managed logging is not enabled in the AddOnDeploymentConfig")
	}
	return nil
}
