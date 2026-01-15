// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package certificates

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"os"
	"testing"

	certificatesv1 "k8s.io/api/certificates/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func init() {
	os.Setenv("TEST", "true")
}

func createCSR() []byte {
	keys, _ := rsa.GenerateKey(rand.Reader, 2048)

	csrTemplate := x509.CertificateRequest{
		Subject: pkix.Name{
			Country: []string{"US"},
		},
		SignatureAlgorithm: x509.SHA512WithRSA,
	}
	csrCertificate, _ := x509.CreateCertificateRequest(rand.Reader, &csrTemplate, keys)
	csr := pem.EncodeToMemory(&pem.Block{
		Type: "CERTIFICATE REQUEST", Bytes: csrCertificate,
	})
	return csr
}

func TestSign(t *testing.T) {
	csr := &certificatesv1.CertificateSigningRequest{
		Spec: certificatesv1.CertificateSigningRequestSpec{
			Request: createCSR(),
			Usages:  []certificatesv1.KeyUsage{certificatesv1.UsageCertSign, certificatesv1.UsageClientAuth},
		},
	}

	c, err := getClient(nil)
	if err != nil {
		t.Fatal("Failed to get client", err)
	}

	res, err := Sign(c, csr)
	if err != nil {
		t.Fatal("Failed to sign CSR", err)
	}
	if res == nil {
		t.Fatal("Failed to sign CSR, nil result")
	}
}

func getClient(s *runtime.Scheme) (client.Client, error) {
	if os.Getenv("TEST") != "" {
		c := fake.NewClientBuilder().Build()
		return c, nil
	}
	config, err := clientcmd.BuildConfigFromFlags("", "")
	if err != nil {
		return nil, errors.New("failed to create the kube config")
	}
	options := client.Options{}
	if s != nil {
		options = client.Options{Scheme: s}
	}
	c, err := client.New(config, options)
	if err != nil {
		return nil, errors.New("failed to create the kube client")
	}
	return c, nil
}
