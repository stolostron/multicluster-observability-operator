// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

/*
How to generate minio certs:
1. run `go run gen.go` to re-generate certs.
2. run `oc create secret generic minio-tls-secret --from-file=ca.crt=./minio-tls/certs/ca.crt --from-file=public.crt=./minio-tls/certs/public.crt --from-file=private.key=./minio-tls/certs/private.key --dry-run='client' -oyaml --namespace=open-cluster-management-observability > ./minio-tls/minio-tls-secret.yaml` to generate minio-tls-secret.yaml
*/

package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"time"

	"github.com/cloudflare/cfssl/log"
)

func main() {
	certPath := "./minio/certs/public.crt"
	privkeyPath := "./minio/certs/private.key"
	caPath := "./minio/certs/ca.crt"
	serverName := "minio"
	var caRoot = &x509.Certificate{
		SerialNumber:          big.NewInt(2019),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	var cert = &x509.Certificate{
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
		log.Error(err)
		os.Exit(1)
	}

	certPrivKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		log.Error(err)
		os.Exit(1)
	}
	// Generate CA cert.
	caBytes, err := x509.CreateCertificate(rand.Reader, caRoot, caRoot, &caPrivKey.PublicKey, caPrivKey)
	if err != nil {
		log.Error(err)
		os.Exit(1)
	}
	caPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: caBytes,
	})
	err = os.WriteFile(caPath, caPEM, 0600)
	if err != nil {
		log.Error(err)
		os.Exit(1)
	}

	// Sign the cert with the CA private key.
	certBytes, err := x509.CreateCertificate(rand.Reader, cert, caRoot, &certPrivKey.PublicKey, caPrivKey)
	if err != nil {
		log.Error(err)
		os.Exit(1)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certBytes,
	})
	err = os.WriteFile(certPath, certPEM, 0600)
	if err != nil {
		log.Error(err)
		os.Exit(1)
	}

	certPrivKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(certPrivKey),
	})
	err = os.WriteFile(privkeyPath, certPrivKeyPEM, 0600)
	if err != nil {
		log.Error(err)
		os.Exit(1)
	}

	os.Exit(0)
}
