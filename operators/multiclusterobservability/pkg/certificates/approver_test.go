// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package certificates

import (
	"context"
	"testing"

	certificatesv1 "k8s.io/api/certificates/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
)

const (
	clusterName = "test"
)

func TestApprove(t *testing.T) {
	ctx := context.Background()
	cluster := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: clusterName,
		},
	}
	csr := &certificatesv1.CertificateSigningRequest{
		Spec: certificatesv1.CertificateSigningRequestSpec{
			Username: "system:open-cluster-management:" + clusterName,
		},
	}
	if !approve(ctx, cluster, nil, csr) {
		t.Fatal("csr not approved automatically")
	}
	illCsr := &certificatesv1.CertificateSigningRequest{
		Spec: certificatesv1.CertificateSigningRequestSpec{
			Username: "illegal",
		},
	}
	if approve(ctx, cluster, nil, illCsr) {
		t.Fatal("illegal csr approved automatically")
	}
}
