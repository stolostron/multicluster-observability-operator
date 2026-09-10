// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package dependencies

import (
	"testing"

	mcoconfig "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestBuildMCOARootCertificateResources(t *testing.T) {
	objs := BuildMCOARootCertificateResources()
	require.Len(t, objs, 3)

	issuer, ok := objs[0].(*unstructured.Unstructured)
	require.True(t, ok, "first object should be an unstructured Issuer")
	require.Equal(t, "Issuer", issuer.GetKind())
	require.Equal(t, mcoconfig.MCOARootCAIssuerName, issuer.GetName())
	require.Equal(t, mcoconfig.CertManagerNamespace, issuer.GetNamespace())

	_, found, err := unstructured.NestedMap(issuer.Object, "spec", "selfSigned")
	require.NoError(t, err)
	require.True(t, found, "Issuer should be self-signed")

	cert, ok := objs[1].(*unstructured.Unstructured)
	require.True(t, ok, "second object should be an unstructured Certificate")
	require.Equal(t, "Certificate", cert.GetKind())
	require.Equal(t, mcoconfig.MCOARootCertificateName, cert.GetName())
	require.Equal(t, mcoconfig.CertManagerNamespace, cert.GetNamespace())

	isCA, found, err := unstructured.NestedBool(cert.Object, "spec", "isCA")
	require.NoError(t, err)
	require.True(t, found)
	require.True(t, isCA)

	secretName, found, err := unstructured.NestedString(cert.Object, "spec", "secretName")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, mcoconfig.MCOARootCertificateName, secretName)

	issuerRefKind, found, err := unstructured.NestedString(cert.Object, "spec", "issuerRef", "kind")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "Issuer", issuerRefKind)

	issuerRefName, found, err := unstructured.NestedString(cert.Object, "spec", "issuerRef", "name")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, mcoconfig.MCOARootCAIssuerName, issuerRefName)

	clusterIssuer, ok := objs[2].(*unstructured.Unstructured)
	require.True(t, ok, "third object should be an unstructured ClusterIssuer")
	require.Equal(t, "ClusterIssuer", clusterIssuer.GetKind())
	require.Equal(t, mcoconfig.MCOARootClusterIssuerName, clusterIssuer.GetName())
	require.Empty(t, clusterIssuer.GetNamespace(), "ClusterIssuer is cluster-scoped")

	caSecretName, found, err := unstructured.NestedString(clusterIssuer.Object, "spec", "ca", "secretName")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, mcoconfig.MCOARootCertificateName, caSecretName)
}

func TestEnsureMCOARootCertificatesInstalled(t *testing.T) {
	scheme := runtime.NewScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	require.NoError(t, EnsureMCOARootCertificatesInstalled(t.Context(), fakeClient))

	issuer := &unstructured.Unstructured{}
	issuer.SetGroupVersionKind(schema.FromAPIVersionAndKind(certManagerGroupVersion, "Issuer"))
	require.NoError(t, fakeClient.Get(t.Context(), types.NamespacedName{
		Name:      mcoconfig.MCOARootCAIssuerName,
		Namespace: mcoconfig.CertManagerNamespace,
	}, issuer))

	// Calling it again should be a no-op (idempotent), not fail with AlreadyExists.
	require.NoError(t, EnsureMCOARootCertificatesInstalled(t.Context(), fakeClient))
}

func TestEnsureMCOARootCertificatesInstalled_PropagatesUnexpectedErrors(t *testing.T) {
	scheme := runtime.NewScheme()

	// erroringClient is defined in loki_operator_test.go (same package) and forces every
	// Create call to fail with a non-AlreadyExists error.
	fakeClient := &erroringClient{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}

	err := EnsureMCOARootCertificatesInstalled(t.Context(), fakeClient)
	require.Error(t, err)
}
