// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package certificates

import (
	"strings"

	certificatesv1 "k8s.io/api/certificates/v1"

	addonapiv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
)

func approve(cluster *clusterv1.ManagedCluster, addon *addonapiv1alpha1.ManagedClusterAddOn,
	csr *certificatesv1.CertificateSigningRequest) bool {
	if strings.HasPrefix(csr.Spec.Username, "system:open-cluster-management:"+cluster.Name) {
		log.Info("CSR approved")
		return true
	} else {
		log.Info("CSR not approved due to illegal requester", "requester", csr.Spec.Username)
		return false
	}
}
