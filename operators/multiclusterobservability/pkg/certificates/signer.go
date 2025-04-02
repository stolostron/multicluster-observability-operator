// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package certificates

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/cloudflare/cfssl/config"
	"github.com/cloudflare/cfssl/signer"
	"github.com/cloudflare/cfssl/signer/local"
	certificatesv1 "k8s.io/api/certificates/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

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

func Sign(csr *certificatesv1.CertificateSigningRequest) ([]byte, error) {
	c, err := getClient(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get client: %w", err)
	}
	if os.Getenv("TEST") != "" {
		// Create the CA secret
		err, _ := createCASecret(c, nil, nil, false, clientCACerts, clientCACertificateCN) // creates the
		if err != nil {
			return nil, fmt.Errorf("failed to create CA secret: %w", err)
		}
	}

	caCert, caKey, _, err := getCA(c, false) // gets client CA
	if err != nil {
		return nil, fmt.Errorf("failed to get client CA: %w", err)
	}

	var usages []string
	for _, usage := range csr.Spec.Usages {
		usages = append(usages, string(usage))
	}

	certExpiryDuration := 365 * 24 * time.Hour
	durationUntilExpiry := time.Until(caCert.NotAfter)
	if durationUntilExpiry <= 0 {
		return nil, fmt.Errorf("signer has expired: %s", caCert.NotAfter)
	}
	if durationUntilExpiry < certExpiryDuration {
		certExpiryDuration = durationUntilExpiry
	}

	policy := &config.Signing{
		Default: &config.SigningProfile{
			Usage:        usages,
			Expiry:       certExpiryDuration,
			ExpiryString: certExpiryDuration.String(),
		},
	}
	cfs, err := local.NewSigner(caKey, caCert, signer.DefaultSigAlgo(caKey), policy)
	if err != nil {
		return nil, fmt.Errorf("failed to create new local signer: %w", err)
	}

	signedCert, err := cfs.Sign(signer.SignRequest{
		Request: string(csr.Spec.Request),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to sign the CSR: %w", err)
	}
	return signedCert, nil
}
