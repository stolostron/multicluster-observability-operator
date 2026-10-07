// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

import (
	"context"
	"fmt"

	mcoconfig "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
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
  # SeaweedFS's S3 gateway listens on 8333 (not MinIO's 9000), on the
  # "seaweedfs" Service in this namespace
  endpoint: http://seaweedfs:8333
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

// SetAddOnDeploymentConfigCustomizedVariable adds or updates a single named CustomizedVariable
// on the MCOA AddOnDeploymentConfig (open-cluster-management-observability/
// multicluster-observability-addon).
//
// This exists for variables the addon-manager consumes (e.g. "platformLogsDefault", which
// selects the managed-log-store use case) that MCO's own renderer doesn't yet set from the MCO
// CR's capabilities spec. It reads the live object, updates only the named list item (by its
// "name" key), and writes the whole object back — the same read-modify-write pattern the
// analytics controller uses to sync right-sizing variables — so it never disturbs any other
// CustomizedVariable entry MCO or the addon-manager itself owns.
//
// The AddOnDeploymentConfig is only rendered once MCOA is enabled (SetMCOACapabilities /
// SetMCOAPlatformLogsCapability), so this should typically be called from inside an Eventually()
// to tolerate the brief window before MCO's controller creates it.
func SetAddOnDeploymentConfigCustomizedVariable(opt TestOptions, name, value string) error {
	clientDynamic := NewKubeClientDynamic(
		opt.HubCluster.ClusterServerURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		aodc, err := clientDynamic.Resource(NewMCOAddOnDeploymentConfigGVR()).
			Namespace(MCO_NAMESPACE).
			Get(context.TODO(), MCOA_CLUSTER_MANAGEMENT_ADDON_NAME, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get AddOnDeploymentConfig %s/%s: %w", MCO_NAMESPACE, MCOA_CLUSTER_MANAGEMENT_ADDON_NAME, err)
		}

		vars, _, err := unstructured.NestedSlice(aodc.Object, "spec", "customizedVariables")
		if err != nil {
			return fmt.Errorf("failed to read customizedVariables from AddOnDeploymentConfig: %w", err)
		}

		found := false
		for i, v := range vars {
			varMap, ok := v.(map[string]any)
			if !ok {
				continue
			}
			if varMap["name"] == name {
				varMap["value"] = value
				vars[i] = varMap
				found = true
				break
			}
		}
		if !found {
			vars = append(vars, map[string]any{"name": name, "value": value})
		}

		if err := unstructured.SetNestedSlice(aodc.Object, vars, "spec", "customizedVariables"); err != nil {
			return fmt.Errorf("failed to set customizedVariables on AddOnDeploymentConfig: %w", err)
		}

		_, err = clientDynamic.Resource(NewMCOAddOnDeploymentConfigGVR()).
			Namespace(MCO_NAMESPACE).
			Update(context.TODO(), aodc, metav1.UpdateOptions{})
		return err
	})
}

// RemoveAddOnDeploymentConfigCustomizedVariable removes a single named CustomizedVariable from
// the MCOA AddOnDeploymentConfig, if present. It's a no-op (returns nil) if the
// AddOnDeploymentConfig doesn't exist or doesn't carry that key, so it's safe to call
// unconditionally during test cleanup.
func RemoveAddOnDeploymentConfigCustomizedVariable(opt TestOptions, name string) error {
	clientDynamic := NewKubeClientDynamic(
		opt.HubCluster.ClusterServerURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		aodc, err := clientDynamic.Resource(NewMCOAddOnDeploymentConfigGVR()).
			Namespace(MCO_NAMESPACE).
			Get(context.TODO(), MCOA_CLUSTER_MANAGEMENT_ADDON_NAME, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return fmt.Errorf("failed to get AddOnDeploymentConfig %s/%s: %w", MCO_NAMESPACE, MCOA_CLUSTER_MANAGEMENT_ADDON_NAME, err)
		}

		vars, found, err := unstructured.NestedSlice(aodc.Object, "spec", "customizedVariables")
		if err != nil {
			return fmt.Errorf("failed to read customizedVariables from AddOnDeploymentConfig: %w", err)
		}
		if !found {
			return nil
		}

		newVars := make([]any, 0, len(vars))
		changed := false
		for _, v := range vars {
			varMap, ok := v.(map[string]any)
			if ok && varMap["name"] == name {
				changed = true
				continue
			}
			newVars = append(newVars, v)
		}
		if !changed {
			return nil
		}

		if err := unstructured.SetNestedSlice(aodc.Object, newVars, "spec", "customizedVariables"); err != nil {
			return fmt.Errorf("failed to set customizedVariables on AddOnDeploymentConfig: %w", err)
		}

		_, err = clientDynamic.Resource(NewMCOAddOnDeploymentConfigGVR()).
			Namespace(MCO_NAMESPACE).
			Update(context.TODO(), aodc, metav1.UpdateOptions{})
		return err
	})
}
