// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package multiclusterobservability

import (
	"context"
	"testing"

	certv1alpha1 "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1alpha1"
	routev1 "github.com/openshift/api/route/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	mcov1beta2 "github.com/open-cluster-management/multicluster-observability-operator/api/v1beta2"
	mcoconfig "github.com/open-cluster-management/multicluster-observability-operator/pkg/config"
)

const (
	secretName = "test-secret"
	token      = "test-token"
	ca         = "test-ca"
)

func newTestCert(name string, namespace string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"ca.crt":  []byte("test-ca-crt"),
			"tls.crt": []byte("test-tls-crt"),
			"tls.key": []byte("test-tls-key"),
		},
	}
}

func TestCreateCertificates(t *testing.T) {
	var (
		name       = "monitoring"
		namespace  = mcoconfig.GetDefaultNamespace()
		testSuffix = "-test"
	)
	mco := &mcov1beta2.MultiClusterObservability{
		TypeMeta:   metav1.TypeMeta{Kind: "MultiClusterObservability"},
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
		Spec:       mcov1beta2.MultiClusterObservabilitySpec{},
	}
	route := &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      obsAPIGateway,
			Namespace: namespace,
		},
		Spec: routev1.RouteSpec{
			Host: "apiServerURL",
		},
	}
	s := scheme.Scheme
	mcov1beta2.SchemeBuilder.AddToScheme(s)
	certv1alpha1.SchemeBuilder.AddToScheme(s)
	routev1.AddToScheme(s)

	c := fake.NewFakeClient(route)

	err := createObservabilityCertificate(c, s, mco)
	if err != nil {
		t.Fatalf("createObservabilityCertificate: (%v)", err)
	}

	// Test scenario in which issuer/certificate updated by others
	clusterIssuer := &certv1alpha1.ClusterIssuer{
		ObjectMeta: metav1.ObjectMeta{
			Name: clientCAIssuer,
		},
		Spec: certv1alpha1.IssuerSpec{
			IssuerConfig: certv1alpha1.IssuerConfig{
				CA: &certv1alpha1.CAIssuer{
					SecretName: clientCAIssuer + testSuffix,
				},
			},
		},
	}
	issuer := &certv1alpha1.Issuer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clientSelfSignIssuer,
			Namespace: certMgrClusterRsNs,
		},
		Spec: certv1alpha1.IssuerSpec{
			IssuerConfig: certv1alpha1.IssuerConfig{
				CA: &certv1alpha1.CAIssuer{
					SecretName: clientSelfSignIssuer + testSuffix,
				},
			},
		},
	}
	cert := &certv1alpha1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clientCACertificate,
			Namespace: certMgrClusterRsNs,
		},
		Spec: certv1alpha1.CertificateSpec{
			CommonName: clientCACertificate + testSuffix,
		},
	}
	c = fake.NewFakeClient(route, clusterIssuer, issuer, cert)
	err = createObservabilityCertificate(c, s, mco)
	if err != nil {
		t.Fatalf("createObservabilityCertificate: (%v)", err)
	}

	foundCert := &certv1alpha1.Certificate{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: clientCACertificate, Namespace: certMgrClusterRsNs}, foundCert)
	if err != nil {
		t.Fatalf("Failed to get certificate (%s): (%v)", clientCACertificate, err)
	}
	if foundCert.Spec.CommonName != clientCACertificate {
		t.Fatalf("Failed to update certificate (%s)", clientCACertificate)
	}

	foundIssuer := &certv1alpha1.Issuer{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: clientSelfSignIssuer, Namespace: certMgrClusterRsNs}, foundIssuer)
	if err != nil {
		t.Fatalf("Failed to get issuer (%s): (%v)", clientSelfSignIssuer, err)
	}
	if foundIssuer.Spec.CA != nil {
		t.Fatalf("Failed to update issuer (%s)", clientSelfSignIssuer)
	}

	foundClusterIssuer := &certv1alpha1.ClusterIssuer{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: clientCAIssuer}, foundClusterIssuer)
	if err != nil {
		t.Fatalf("Failed to get issuer (%s): (%v)", clientCAIssuer, err)
	}
	if foundClusterIssuer.Spec.CA.SecretName != clientCACerts {
		t.Fatalf("Failed to update issuer (%s)", clientCAIssuer)
	}

	err = cleanIssuerCert(c)
	if err != nil {
		t.Fatalf("Failed to clean the issuer/certificate")
	}

	// Test clean scenario in which issuer/certificate already removed
	err = createObservabilityCertificate(c, s, mco)
	if err != nil {
		t.Fatalf("Failed to createObservabilityCertificate: (%v)", err)
	}

	err = c.Delete(context.TODO(), clusterIssuer)
	if err != nil {
		t.Fatalf("Failed to delete (%s): (%v)", clientCAIssuer, err)
	}
	err = c.Delete(context.TODO(), issuer)
	if err != nil {
		t.Fatalf("Failed to delete (%s): (%v)", clientSelfSignIssuer, err)
	}
	err = c.Delete(context.TODO(), cert)
	if err != nil {
		t.Fatalf("Failed to delete (%s): (%v)", clientCACertificate, err)
	}

	err = cleanIssuerCert(c)
	if err != nil {
		t.Fatalf("Failed to clean the issuer/certificate")
	}
}
