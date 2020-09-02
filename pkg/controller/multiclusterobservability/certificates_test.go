// Copyright (c) 2020 Red Hat, Inc.

package multiclusterobservability

import (
	"context"
	"testing"

	certv1alpha1 "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	mcov1beta1 "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/apis/observability/v1beta1"
	mcoconfig "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/config"
)

const (
	secretName = "test-secret"
	token      = "test-token"
	ca         = "test-ca"
)

func TestCreateCertificates(t *testing.T) {
	var (
		name       = "monitoring"
		namespace  = mcoconfig.GetDefaultNamespace()
		testSuffix = "-test"
	)
	mco := &mcov1beta1.MultiClusterObservability{
		TypeMeta:   metav1.TypeMeta{Kind: "MultiClusterObservability"},
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
		Spec:       mcov1beta1.MultiClusterObservabilitySpec{},
	}

	s := scheme.Scheme
	mcov1beta1.SchemeBuilder.AddToScheme(s)
	certv1alpha1.SchemeBuilder.AddToScheme(s)

	c := fake.NewFakeClient()
	err := createObservabilityCertificate(c, s, mco)
	if err != nil {
		t.Fatalf("createObservabilityCertificate: (%v)", err)
	}

	clusterIssuer := &certv1alpha1.ClusterIssuer{
		ObjectMeta: metav1.ObjectMeta{
			Name: clientCAIssuer,
		},
		Spec: certv1alpha1.IssuerSpec{
			IssuerConfig: certv1alpha1.IssuerConfig{
				CA: &certv1alpha1.CAIssuer{
					SecretName: clientCACerts + testSuffix,
				},
			},
		},
	}
	issuer := &certv1alpha1.Issuer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serverCAIssuer,
			Namespace: namespace,
		},
		Spec: certv1alpha1.IssuerSpec{
			IssuerConfig: certv1alpha1.IssuerConfig{
				CA: &certv1alpha1.CAIssuer{
					SecretName: serverCACerts + testSuffix,
				},
			},
		},
	}
	cert := &certv1alpha1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serverCertificate,
			Namespace: namespace,
		},
		Spec: certv1alpha1.CertificateSpec{
			CommonName: serverCertificate + testSuffix,
		},
	}
	c = fake.NewFakeClient(clusterIssuer, issuer, cert)
	err = createObservabilityCertificate(c, s, mco)
	if err != nil {
		t.Fatalf("createObservabilityCertificate: (%v)", err)
	}

	foundCert := &certv1alpha1.Certificate{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: serverCertificate, Namespace: namespace}, foundCert)
	if err != nil {
		t.Fatalf("Failed to get certificate (%s): (%v)", serverCertificate, err)
	}
	if foundCert.Spec.CommonName != serverCertificate {
		t.Fatalf("Failed to update certificate (%s)", serverCertificate)
	}

	foundIssuer := &certv1alpha1.Issuer{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: serverCAIssuer, Namespace: namespace}, foundIssuer)
	if err != nil {
		t.Fatalf("Failed to get issuer (%s): (%v)", serverCAIssuer, err)
	}
	if foundIssuer.Spec.CA.SecretName != serverCACerts {
		t.Fatalf("Failed to update issuer (%s)", serverCAIssuer)
	}

	foundClusterIssuer := &certv1alpha1.ClusterIssuer{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: clientCAIssuer}, foundClusterIssuer)
	if err != nil {
		t.Fatalf("Failed to get issuer (%s): (%v)", clientCAIssuer, err)
	}
	if foundClusterIssuer.Spec.CA.SecretName != clientCACerts {
		t.Fatalf("Failed to update issuer (%s)", clientCAIssuer)
	}
}
