// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package certificates

import (
	"errors"
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
		c := fake.NewFakeClient()
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

func sign(csr *certificatesv1.CertificateSigningRequest) []byte {
	c, err := getClient(nil)
	if err != nil {
		log.Error(err, err.Error())
		return nil
	}
	if os.Getenv("TEST") != "" {
		err, _ := createCASecret(c, nil, nil, false, clientCACerts, clientCACertificateCN)
		if err != nil {
			log.Error(err, "Failed to create CA")
		}
	}
	caCert, caKey, _, err := getCA(c, false)
	if err != nil {
		return nil
	}

	var usages []string
	for _, usage := range csr.Spec.Usages {
		usages = append(usages, string(usage))
	}

	certExpiryDuration := 365 * 24 * time.Hour
	durationUntilExpiry := time.Until(caCert.NotAfter)
	if durationUntilExpiry <= 0 {
		log.Error(errors.New("signer has expired"), "the signer has expired", "expired time", caCert.NotAfter)
		return nil
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
		log.Error(err, "Failed to create new local signer")
		return nil
	}

	signedCert, err := cfs.Sign(signer.SignRequest{
		Request: string(csr.Spec.Request),
	})
	if err != nil {
		log.Error(err, "Failed to sign the CSR")
		return nil
	}
	return signedCert
}
