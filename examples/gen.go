// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package main

/*
How to generate SeaweedFS certs:
1. run `go run gen.go` to re-generate certs.
2. run `oc create secret generic seaweedfs-tls-secret --from-file=ca.crt=./seaweedfs-tls/certs/ca.crt --from-file=public.crt=./seaweedfs-tls/certs/public.crt --from-file=private.key=./seaweedfs-tls/certs/private.key --dry-run='client' -oyaml --namespace=open-cluster-management-observability > ./seaweedfs-tls/seaweedfs-tls-secret.yaml` to generate seaweedfs-tls-secret.yaml
*/

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"log/slog"
	"math/big"
	"net"
	"os"
	"time"
)

func main() {
	certPath := "./seaweedfs-tls/certs/public.crt"
	privkeyPath := "./seaweedfs-tls/certs/private.key"
	caPath := "./seaweedfs-tls/certs/ca.crt"
	serverName := "seaweedfs"
	caRoot := &x509.Certificate{
		SerialNumber:          big.NewInt(2019),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	cert := &x509.Certificate{
		SerialNumber: big.NewInt(1658),
		DNSNames:     []string{serverName},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
		NotAfter:     time.Now().AddDate(10, 0, 0),
		SubjectKeyId: []byte{1, 2, 3},
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}

	caPrivKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		slog.Error("Failed to generate CA private key", "error", err)
		os.Exit(1)
	}

	certPrivKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		slog.Error("Failed to generate certificate private key", "error", err)
		os.Exit(1)
	}
	// Generate CA cert.
	caBytes, err := x509.CreateCertificate(rand.Reader, caRoot, caRoot, &caPrivKey.PublicKey, caPrivKey)
	if err != nil {
		slog.Error("Failed to create CA certificate", "error", err)
		os.Exit(1)
	}
	caPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: caBytes,
	})
	err = os.WriteFile(caPath, caPEM, 0o600)
	if err != nil {
		slog.Error("Failed to write CA certificate to file", "error", err)
		os.Exit(1)
	}

	// Sign the cert with the CA private key.
	certBytes, err := x509.CreateCertificate(rand.Reader, cert, caRoot, &certPrivKey.PublicKey, caPrivKey)
	if err != nil {
		slog.Error("Failed to create certificate", "error", err)
		os.Exit(1)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certBytes,
	})
	err = os.WriteFile(certPath, certPEM, 0o600)
	if err != nil {
		slog.Error("Failed to write certificate to file", "error", err)
		os.Exit(1)
	}

	certPrivKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(certPrivKey),
	})
	err = os.WriteFile(privkeyPath, certPrivKeyPEM, 0o600)
	if err != nil {
		slog.Error("Failed to write private key to file", "error", err)
		os.Exit(1)
	}

	os.Exit(0)
}
