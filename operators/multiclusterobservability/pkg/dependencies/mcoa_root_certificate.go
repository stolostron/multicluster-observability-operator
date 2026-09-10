// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package dependencies

import (
	"context"
	"fmt"

	mcoconfig "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// certManagerGroupVersion is the API group/version cert-manager registers its CRDs under.
const certManagerGroupVersion = "cert-manager.io/v1"

// BuildMCOARootCertificateResources returns the self-signed Issuer, root CA Certificate and
// ClusterIssuer that MCOA's platform log collection capability needs from cert-manager to
// issue the mTLS certs used for log collection/storage:
//   - a self-signed Issuer that only ever mints the root CA Certificate below;
//   - a root CA Certificate, written to a Secret of the same name;
//   - a ClusterIssuer backed by that Secret, which is what MCOA-rendered Certificates
//     actually request their leaf certs from.
//
// These are built as unstructured objects (rather than importing cert-manager's API package)
// to avoid adding a new dependency to this operator for three object kinds.
func BuildMCOARootCertificateResources() []client.Object {
	issuer := &unstructured.Unstructured{}
	issuer.SetGroupVersionKind(schema.FromAPIVersionAndKind(certManagerGroupVersion, "Issuer"))
	issuer.SetName(mcoconfig.MCOARootCAIssuerName)
	issuer.SetNamespace(mcoconfig.CertManagerNamespace)
	issuer.Object["spec"] = map[string]any{
		"selfSigned": map[string]any{},
	}

	cert := &unstructured.Unstructured{}
	cert.SetGroupVersionKind(schema.FromAPIVersionAndKind(certManagerGroupVersion, "Certificate"))
	cert.SetName(mcoconfig.MCOARootCertificateName)
	cert.SetNamespace(mcoconfig.CertManagerNamespace)
	cert.Object["spec"] = map[string]any{
		"isCA":       true,
		"secretName": mcoconfig.MCOARootCertificateName,
		"commonName": "MCOA Root Certificate",
		"privateKey": map[string]any{
			"algorithm": "RSA",
			"size":      int64(4096),
			"encoding":  "PKCS8",
		},
		"issuerRef": map[string]any{
			"kind": "Issuer",
			"name": mcoconfig.MCOARootCAIssuerName,
		},
	}

	clusterIssuer := &unstructured.Unstructured{}
	clusterIssuer.SetGroupVersionKind(schema.FromAPIVersionAndKind(certManagerGroupVersion, "ClusterIssuer"))
	clusterIssuer.SetName(mcoconfig.MCOARootClusterIssuerName)
	// ClusterIssuer is cluster-scoped: no namespace to set. Its "ca.secretName" is still
	// resolved from CertManagerNamespace, since that's where cert-manager itself runs.
	clusterIssuer.Object["spec"] = map[string]any{
		"ca": map[string]any{
			"secretName": mcoconfig.MCOARootCertificateName,
		},
	}

	return []client.Object{issuer, cert, clusterIssuer}
}

// EnsureMCOARootCertificatesInstalled creates the Issuer, Certificate and ClusterIssuer from
// BuildMCOARootCertificateResources if they don't already exist. It never updates an existing
// object, so it won't fight a manual install or overwrite an already-issued root CA.
//
// Callers must only invoke this once cert-manager's Certificate CRD (and, transitively, its
// Issuer/ClusterIssuer CRDs) are known to be established on the cluster; creating these
// resources any earlier would just fail with a NoKindMatchError.
func EnsureMCOARootCertificatesInstalled(ctx context.Context, c client.Client) error {
	for _, obj := range BuildMCOARootCertificateResources() {
		if err := c.Create(ctx, obj); err != nil && !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create %s %s/%s: %w", obj.GetObjectKind().GroupVersionKind().Kind, obj.GetNamespace(), obj.GetName(), err)
		}
	}
	return nil
}
