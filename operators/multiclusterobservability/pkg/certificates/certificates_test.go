// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package certificates

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"

	routev1 "github.com/openshift/api/route/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	mcov1beta2 "github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	mcoconfig "github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
)

var (
	name      = "observability"
	namespace = mcoconfig.GetDefaultNamespace()
)

func getMco() *mcov1beta2.MultiClusterObservability {
	return &mcov1beta2.MultiClusterObservability{
		TypeMeta:   metav1.TypeMeta{Kind: "MultiClusterObservability"},
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec:       mcov1beta2.MultiClusterObservabilitySpec{},
	}
}

func getExpiredCertSecret() *v1.Secret {
	date := time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC)
	ca := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Country: []string{"US"},
		},
		NotBefore: date,
		NotAfter:  date.AddDate(1, 0, 0),
		IsCA:      true,
		KeyUsage:  x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageCertSign,
	}
	caKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	caBytes, _ := x509.CreateCertificate(rand.Reader, ca, ca, &caKey.PublicKey, caKey)
	certPEM, keyPEM := pemEncode(caBytes, x509.MarshalPKCS1PrivateKey(caKey))
	caSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serverCACerts,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"ca.crt":  caBytes,
			"tls.crt": append(certPEM.Bytes(), certPEM.Bytes()...),
			"tls.key": keyPEM.Bytes(),
		},
	}
	return caSecret
}

func TestCreateCertificates(t *testing.T) {
	route := &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "observatorium-api",
			Namespace: namespace,
		},
		Spec: routev1.RouteSpec{
			Host: "apiServerURL",
		},
	}
	mco := getMco()
	s := scheme.Scheme
	mcov1beta2.SchemeBuilder.AddToScheme(s)
	routev1.AddToScheme(s)

	c := fake.NewFakeClient(route)

	err := CreateObservabilityCerts(c, s, mco, true)
	if err != nil {
		t.Fatalf("CreateObservabilityCerts: (%v)", err)
	}

	err = CreateObservabilityCerts(c, s, mco, true)
	if err != nil {
		t.Fatalf("Rerun CreateObservabilityCerts: (%v)", err)
	}

	err, _ = createCASecret(c, s, mco, true, serverCACerts, serverCACertifcateCN)
	if err != nil {
		t.Fatalf("Failed to renew server ca certificates: (%v)", err)
	}

	err = createCertSecret(c, s, mco, true, grafanaCerts, false, grafanaCertificateCN, nil, nil, nil)
	if err != nil {
		t.Fatalf("Failed to renew server certificates: (%v)", err)
	}
}

func TestRemoveExpiredCA(t *testing.T) {

	caSecret := getExpiredCertSecret()
	oldCertLength := len(caSecret.Data["tls.crt"])
	c := fake.NewFakeClient(caSecret)
	removeExpiredCA(c, serverCACerts)
	c.Get(context.TODO(),
		types.NamespacedName{Name: serverCACerts, Namespace: namespace},
		caSecret)
	if len(caSecret.Data["tls.crt"]) != oldCertLength/2 {
		t.Fatal("Expired certificate not removed correctly")
	}
}
