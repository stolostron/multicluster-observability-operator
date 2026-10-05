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
