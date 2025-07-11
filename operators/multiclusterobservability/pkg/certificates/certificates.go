// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package certificates

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"time"

	certificatesv1 "k8s.io/api/certificates/v1"

	operatorconfig "github.com/stolostron/multicluster-observability-operator/operators/pkg/config"

	"golang.org/x/exp/slices"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	mcoutil "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/util"
)

const (
	serverCACertifcateCN        = "observability-server-ca-certificate"
	serverCACerts               = config.ServerCACerts
	serverCertificateCN         = config.ServerCertCN
	serverCerts                 = config.ServerCerts
	hubMetricsCollectorMtlsCert = operatorconfig.HubMetricsCollectorMtlsCert

	clientCACertificateCN = "observability-client-ca-certificate"
	clientCACerts         = config.ClientCACerts
	grafanaCertificateCN  = config.GrafanaCN
	grafanaCerts          = config.GrafanaCerts
)

var (
	log               = logf.Log.WithName("controller_certificates")
	serialNumberLimit = new(big.Int).Lsh(big.NewInt(1), 128)
)

func CreateObservabilityCerts(
	c client.Client,
	scheme *runtime.Scheme,
	mco *mcov1beta2.MultiClusterObservability,
	ingressCtlCrdExists bool,
) error {

	config.SetCertDuration(mco.Annotations)

	err, serverCrtUpdated := createCASecret(c, scheme, mco, false, serverCACerts, serverCACertifcateCN)
	if err != nil {
		return err
	}
	err, clientCrtUpdated := createCASecret(c, scheme, mco, false, clientCACerts, clientCACertificateCN)
	if err != nil {
		return err
	}
	hosts, err := getHosts(c, ingressCtlCrdExists)
	if err != nil {
		return err
	}
	err = createCertSecret(c, scheme, mco, serverCrtUpdated, serverCerts, true, serverCertificateCN, nil, hosts, nil)
	if err != nil {
		return err
	}
	err = createCertSecret(c, scheme, mco, clientCrtUpdated, grafanaCerts, false, grafanaCertificateCN, nil, nil, nil)
	if err != nil {
		return err
	}

	return nil
}

func createCASecret(c client.Client,
	scheme *runtime.Scheme, mco *mcov1beta2.MultiClusterObservability,
	isRenew bool, name string, cn string) (error, bool) {
	if isRenew {
		log.Info("To renew CA certificates", "name", name)
	}
	caSecret := &corev1.Secret{}
	err := c.Get(context.TODO(), types.NamespacedName{Namespace: config.GetDefaultNamespace(), Name: name}, caSecret)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			log.Error(err, "Failed to check ca secret", "name", name)
			return err, false
		} else {
			key, cert, err := createCACertificate(cn, nil)
			if err != nil {
				return err, false
			}
			certPEM, keyPEM := pemEncode(cert, key)
			caSecret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: config.GetDefaultNamespace(),
					Labels: map[string]string{
						config.BackupLabelName: config.BackupLabelValue,
					},
				},
				Data: map[string][]byte{
					"ca.crt":  certPEM.Bytes(),
					"tls.crt": certPEM.Bytes(),
					"tls.key": keyPEM.Bytes(),
				},
			}
			if mco != nil {
				if err := controllerutil.SetControllerReference(mco, caSecret, scheme); err != nil {
					return err, false
				}
			}

			if err := c.Create(context.TODO(), caSecret); err != nil {
				log.Error(err, "Failed to create secret", "name", name)
				return err, false
			} else {
				return nil, true
			}
		}
	} else {
		if !isRenew {
			log.Info("CA secrets already existed", "name", name)
			if err := mcoutil.AddBackupLabelToSecretObj(c, caSecret); err != nil {
				return err, false
			}
		} else {
			block, _ := pem.Decode(caSecret.Data["tls.key"])
			caKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
			if err != nil {
				log.Error(err, "Wrong private key found, create new one", "name", name)
				caKey = nil
			}
			key, cert, err := createCACertificate(cn, caKey)
			if err != nil {
				return err, false
			}
			certPEM, keyPEM := pemEncode(cert, key)
			caSecret.Data["ca.crt"] = certPEM.Bytes()
			caSecret.Data["tls.crt"] = append(certPEM.Bytes(), caSecret.Data["tls.crt"]...)
			caSecret.Data["tls.key"] = keyPEM.Bytes()
			if err := c.Update(context.TODO(), caSecret); err != nil {
				log.Error(err, "Failed to update secret", "name", name)
				return err, false
			} else {
				log.Info("CA certificates renewed", "name", name)
				return nil, true
			}
		}
	}
	return nil, false
}

func createCACertificate(cn string, caKey *rsa.PrivateKey) ([]byte, []byte, error) {
	sn, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		log.Error(err, "failed to generate serial number")
		return nil, nil, err
	}
	ca := &x509.Certificate{
		SerialNumber: sn,
		Subject: pkix.Name{
			Organization: []string{"Red Hat, Inc."},
			Country:      []string{"US"},
			CommonName:   cn,
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(config.GetCertDuration() * 5),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	if caKey == nil {
		caKey, err = rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			log.Error(err, "Failed to generate private key", "cn", cn)
			return nil, nil, err
		}
	}

	caBytes, err := x509.CreateCertificate(rand.Reader, ca, ca, &caKey.PublicKey, caKey)
	if err != nil {
		log.Error(err, "Failed to create certificate", "cn", cn)
		return nil, nil, err
	}
	caKeyBytes := x509.MarshalPKCS1PrivateKey(caKey)
	return caKeyBytes, caBytes, nil
}

// TODO(saswatamcode): Refactor function to remove ou.
//
//nolint:unparam
func createCertSecret(c client.Client,
	scheme *runtime.Scheme, mco *mcov1beta2.MultiClusterObservability,
	isRenew bool, name string, isServer bool,
	cn string, ou []string, dns []string, ips []net.IP) error {
	if isRenew {
		log.Info("To renew certificates", "name", name)
	}
	crtSecret := &corev1.Secret{}
	err := c.Get(context.TODO(), types.NamespacedName{Namespace: config.GetDefaultNamespace(), Name: name}, crtSecret)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			log.Error(err, "Failed to check certificate secret", "name", name)
			return err
		} else {
			caCert, caKey, caCertBytes, err := getCA(c, isServer)
			if err != nil {
				return err
			}
			key, cert, err := createCertificate(isServer, cn, ou, dns, ips, caCert, caKey, nil)
			if err != nil {
				return err
			}
			certPEM, keyPEM := pemEncode(cert, key)
			crtSecret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: config.GetDefaultNamespace(),
					Labels: map[string]string{
						config.BackupLabelName: config.BackupLabelValue,
					},
				},
				Data: map[string][]byte{
					"ca.crt":  caCertBytes,
					"tls.crt": certPEM.Bytes(),
					"tls.key": keyPEM.Bytes(),
				},
			}
			if mco != nil {
				if err := controllerutil.SetControllerReference(mco, crtSecret, scheme); err != nil {
					return err
				}
			}
			err = c.Create(context.TODO(), crtSecret)
			if err != nil {
				log.Error(err, "Failed to create secret", "name", name)
				return err
			}
		}
	} else {
		if crtSecret.Name == serverCerts && !isRenew {
			block, _ := pem.Decode(crtSecret.Data["tls.crt"])
			if block == nil || block.Bytes == nil {
				log.Info("Empty block in server certificate, skip")
			} else {
				serverCrt, err := x509.ParseCertificate(block.Bytes)
				if err != nil {
					log.Error(err, "Failed to parse the server certificate, renew it")
					isRenew = true
				}
				// to handle upgrade scenario in which hosts maybe update
				for _, dnsString := range dns {
					if !slices.Contains(serverCrt.DNSNames, dnsString) {
						isRenew = true
						break
					}
				}
			}
		}

		if !isRenew {
			log.Info("Certificate secrets already existed", "name", name)
			if err := mcoutil.AddBackupLabelToSecretObj(c, crtSecret); err != nil {
				return err
			}
		} else {
			caCert, caKey, caCertBytes, err := getCA(c, isServer)
			if err != nil {
				return err
			}
			block, _ := pem.Decode(crtSecret.Data["tls.key"])
			crtkey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
			if err != nil {
				log.Error(err, "Wrong private key found, create new one", "name", name)
				crtkey = nil
			}
			key, cert, err := createCertificate(isServer, cn, ou, dns, ips, caCert, caKey, crtkey)
			if err != nil {
				return err
			}
			certPEM, keyPEM := pemEncode(cert, key)
			crtSecret.Data["ca.crt"] = caCertBytes
			crtSecret.Data["tls.crt"] = certPEM.Bytes()
			crtSecret.Data["tls.key"] = keyPEM.Bytes()
			if err := c.Update(context.TODO(), crtSecret); err != nil {
				log.Error(err, "Failed to update secret", "name", name)
				return err
			} else {
				log.Info("Certificates renewed", "name", name)
			}
		}
	}
	return nil
}

func createCertificate(isServer bool, cn string, ou []string, dns []string, ips []net.IP,
	caCert *x509.Certificate, caKey *rsa.PrivateKey, key *rsa.PrivateKey) ([]byte, []byte, error) {
	sn, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		log.Error(err, "failed to generate serial number")
		return nil, nil, err
	}

	cert := &x509.Certificate{
		SerialNumber: sn,
		Subject: pkix.Name{
			Organization: []string{"Red Hat, Inc."},
			Country:      []string{"US"},
			CommonName:   cn,
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().Add(config.GetCertDuration()),
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
	}
	if !isServer {
		cert.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	}
	if ou != nil {
		cert.Subject.OrganizationalUnit = ou
	}
	if dns != nil {
		dns = append(dns[:1], dns[0:]...)
		dns[0] = cn
		cert.DNSNames = dns
	} else {
		cert.DNSNames = []string{cn}
	}
	if ips != nil {
		cert.IPAddresses = ips
	}

	if key == nil {
		key, err = rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			log.Error(err, "Failed to generate private key", "cn", cn)
			return nil, nil, err
		}
	}

	caBytes, err := x509.CreateCertificate(rand.Reader, cert, caCert, &key.PublicKey, caKey)
	if err != nil {
		log.Error(err, "Failed to create certificate", "cn", cn)
		return nil, nil, err
	}
	keyBytes := x509.MarshalPKCS1PrivateKey(key)
	return keyBytes, caBytes, nil
}

func getCA(c client.Client, isServer bool) (*x509.Certificate, *rsa.PrivateKey, []byte, error) {
	caCertName := serverCACerts
	if !isServer {
		caCertName = clientCACerts
	}
	caSecret := &corev1.Secret{}
	err := c.Get(
		context.TODO(),
		types.NamespacedName{Namespace: config.GetDefaultNamespace(), Name: caCertName},
		caSecret,
	)
	if err != nil {
		log.Error(err, "Failed to get ca secret", "name", caCertName)
		return nil, nil, nil, err
	}
	block1, rest := pem.Decode(caSecret.Data["tls.crt"])
	caCertBytes := caSecret.Data["tls.crt"][:len(caSecret.Data["tls.crt"])-len(rest)]
	caCerts, err := x509.ParseCertificates(block1.Bytes)
	if err != nil {
		log.Error(err, "Failed to parse ca cert", "name", caCertName)
		return nil, nil, nil, err
	}
	block2, _ := pem.Decode(caSecret.Data["tls.key"])
	caKey, err := x509.ParsePKCS1PrivateKey(block2.Bytes)
	if err != nil {
		log.Error(err, "Failed to parse ca key", "name", caCertName)
		return nil, nil, nil, err
	}
	return caCerts[0], caKey, caCertBytes, nil
}

func removeExpiredCA(c client.Client, name string) {
	caSecret := &corev1.Secret{}
	err := c.Get(context.TODO(), types.NamespacedName{Namespace: config.GetDefaultNamespace(), Name: name}, caSecret)
	if err != nil {
		log.Error(err, "Failed to get ca secret", "name", name)
		return
	}
	data := caSecret.Data["tls.crt"]
	_, restData := pem.Decode(data)
	caSecret.Data["tls.crt"] = data[:len(data)-len(restData)]
	if len(restData) > 0 {
		for {
			var block *pem.Block
			index := len(data) - len(restData)
			block, restData = pem.Decode(restData)
			certs, err := x509.ParseCertificates(block.Bytes)
			removeFlag := false
			if err != nil {
				log.Error(err, "Find wrong cert bytes, needs to remove it", "name", name)
				removeFlag = true
			} else {
				if time.Now().After(certs[0].NotAfter) {
					log.Info("CA certificate expired, needs to remove it", "name", name)
					removeFlag = true
				}
			}
			if !removeFlag {
				caSecret.Data["tls.crt"] = append(caSecret.Data["tls.crt"], data[index:len(data)-len(restData)]...)
			}
			if len(restData) == 0 {
				break
			}
		}
	}
	if len(data) != len(caSecret.Data["tls.crt"]) {
		err = c.Update(context.TODO(), caSecret)
		if err != nil {
			log.Error(err, "Failed to update ca secret to removed expired ca", "name", name)
		} else {
			log.Info("Expired certificates are removed", "name", name)
		}
	}
}

func pemEncode(cert []byte, key []byte) (*bytes.Buffer, *bytes.Buffer) {
	certPEM := new(bytes.Buffer)
	err := pem.Encode(certPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert,
	})
	if err != nil {
		log.Error(err, "Failed to encode cert")
	}

	keyPEM := new(bytes.Buffer)
	err = pem.Encode(keyPEM, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: key,
	})
	if err != nil {
		log.Error(err, "Failed to encode key")
	}

	return certPEM, keyPEM
}

func getHosts(c client.Client, ingressCtlCrdExists bool) ([]string, error) {
	hosts := []string{config.GetObsAPISvc(config.GetOperandName(config.Observatorium))}

	customHostURL, err := config.GetObsAPIExternalURL(context.TODO(), c, config.GetDefaultNamespace())
	if err != nil {
		return nil, err
	}
	// The config.GetObsAPIExternalURL call is already doing URL parsing under the hood to ensure it's valid,
	// so we don't need to check the error of url.Parse again.
	customHost := customHostURL.Hostname()
	if customHost != "" {
		hosts = append(hosts, customHost)
	}

	if ingressCtlCrdExists {
		url, err := config.GetObsAPIRouteHost(context.TODO(), c, config.GetDefaultNamespace())
		if err != nil {
			log.Error(err, "Failed to get api route address")
			return nil, err
		}
		// Sometimes these two are the same, so we avoid the duplication.
		if customHost == "" || url != customHost {
			hosts = append(hosts, url)
		}
	}
	return hosts, nil
}

func GenerateKeyAndCSR() ([]byte, []byte, error) {
	keys, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, fmt.Errorf("failed generate private key: %w", err)
	}

	oidOrganization := []int{2, 5, 4, 11} // Object Identifier (OID) for Organization Unit
	oidUser := []int{2, 5, 4, 3}          // Object Identifier (OID) for User

	var csrTemplate = x509.CertificateRequest{
		Subject: pkix.Name{
			Organization: []string{"Red Hat, Inc."},
			Country:      []string{"US"},
			CommonName:   operatorconfig.ClientCACertificateCN,
			ExtraNames: []pkix.AttributeTypeAndValue{
				{Type: oidOrganization, Value: "acm"},
				{Type: oidUser, Value: "managed-cluster-observability"},
			},
		},
		DNSNames:           []string{"observability-controller.addon.open-cluster-management.io"},
		SignatureAlgorithm: x509.SHA512WithRSA,
	}
	csrCertificate, err := x509.CreateCertificateRequest(rand.Reader, &csrTemplate, keys)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create CSR: %w", err)
	}
	csr := pem.EncodeToMemory(&pem.Block{
		Type: "CERTIFICATE REQUEST", Bytes: csrCertificate,
	})

	privateKey := pem.EncodeToMemory(&pem.Block{
		Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(keys),
	})

	return csr, privateKey, nil
}

func CreateUpdateMtlsCertSecretForHubCollector(ctx context.Context, c client.Client) error {
	hubMtlsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      operatorconfig.HubMetricsCollectorMtlsCert,
			Namespace: config.GetDefaultNamespace(),
		},
	}
	updateReason := "None"
	res, err := controllerutil.CreateOrUpdate(ctx, c, hubMtlsSecret, func() error {
		renew := func() error {
			if err := newMtlsCertSecretForHubCollector(hubMtlsSecret); err != nil {
				return fmt.Errorf("failed to create hub mtls secret: %w", err)
			}
			return nil
		}

		// renew if the mTLS secret is empty
		hubMtlsCert := hubMtlsSecret.Data["tls.crt"]
		if len(hubMtlsCert) == 0 {
			updateReason = "Empty hub mTLS cert"
			return renew()
		}

		// renew if mTLS cert is not signed by current CA certificate
		caRef := types.NamespacedName{Namespace: config.GetDefaultNamespace(), Name: config.ClientCACerts}
		caSecret := &corev1.Secret{}
		if err := c.Get(ctx, caRef, caSecret); err != nil {
			return fmt.Errorf("failed to get CA secret: %w", err)
		}

		isSignedByCA, err := childCertIsSignedByCA(caSecret.Data["tls.crt"], hubMtlsCert)
		if err != nil {
			return fmt.Errorf("failed to check if the mtls cert %q is signed by the current CA %q: %w", hubMtlsSecret.Name, caSecret.Name, err)
		}
		if !isSignedByCA {
			updateReason = "mTLS cert is not signed by current CA"
			return renew()
		}

		// renew if the mTLS certificate is approaching end of life
		if mtlsCertShouldBeRenewed(hubMtlsSecret) {
			updateReason = "mTLS cert should be renewed"
			return renew()
		}

		// No change
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to create or update HubMtlsSecret: %w", err)
	}
	if res != controllerutil.OperationResultNone {
		log.Info("updated successfully HubMtlsSecret", "name", hubMtlsSecret.Name, "updateReason", updateReason)
	}

	return nil
}

func newMtlsCertSecretForHubCollector(mtlsSecret *corev1.Secret) error {
	csrBytes, privateKeyBytes, err := GenerateKeyAndCSR()
	if err != nil {
		return fmt.Errorf("failed to generate private key and CSR: %w", err)
	}

	csr := &certificatesv1.CertificateSigningRequest{
		Spec: certificatesv1.CertificateSigningRequestSpec{
			Request: csrBytes,
			Usages:  []certificatesv1.KeyUsage{certificatesv1.UsageDigitalSignature, certificatesv1.UsageClientAuth},
		},
	}
	signedClientCert, err := Sign(csr)
	if err != nil {
		return fmt.Errorf("failed to sign CSR: %w", err)
	}

	mtlsSecret.Data = map[string][]byte{
		"tls.crt": signedClientCert,
		"tls.key": privateKeyBytes,
	}

	return nil
}

// childCertIsSignedByCA verifies that the child PEM cert is signed by the CA PEM cert.
// It expects a single certificate in each PEM.
func childCertIsSignedByCA(caPemCert, childPemCert []byte) (bool, error) {
	// Validate inputs
	if len(caPemCert) == 0 {
		return false, errors.New("CA certificate is empty")
	}
	if len(childPemCert) == 0 {
		return false, errors.New("child certificate is empty")
	}

	// Create CA cert pool
	caCertPool := x509.NewCertPool()
	caCerts, err := parsePEM(caPemCert, "CA certificate")
	if err != nil {
		return false, fmt.Errorf("failed to parse CA PEM certificate: %w", err)
	}

	if len(caCerts) != 1 {
		log.Info(fmt.Sprintf("expecting a single certificate for CA, found %d", len(caCerts)))
	}
	caCertPool.AddCert(caCerts[0])

	// Extract leaf
	childCerts, err := parsePEM(childPemCert, "child certificate")
	if err != nil {
		return false, fmt.Errorf("failed to parse child PEM certificate: %w", err)
	}

	if len(childCerts) != 1 {
		return false, fmt.Errorf("expecting a single certificate for child, found %d", len(childCerts))
	}

	// Check child is signed by CA
	_, err = childCerts[0].Verify(x509.VerifyOptions{
		Roots:     caCertPool,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	})
	if err != nil {
		log.Info(fmt.Sprintf("child certificate is not signed by CA: %v", err))
		return false, nil
	}

	return true, nil
}

// parsePEM extracts certificates from PEM.
func parsePEM(pemData []byte, certName string) ([]*x509.Certificate, error) {
	if len(pemData) == 0 {
		return nil, fmt.Errorf("empty PEM data for %s", certName)
	}

	var allCerts []*x509.Certificate

	rest := pemData
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}

		if block.Type != "CERTIFICATE" {
			log.Info("Ingoring non certificate block in pem", "blockType", block.Type, "certName", certName)
			continue
		}

		parsed, err := x509.ParseCertificates(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("failed to parse certificate block: %w", err)
		}
		allCerts = append(allCerts, parsed...)
	}

	if len(allCerts) == 0 {
		return nil, errors.New("no certificates found in PEM")
	}

	return allCerts, nil
}

func mtlsCertShouldBeRenewed(mtlsSecret *corev1.Secret) bool {
	data, ok := mtlsSecret.Data["tls.crt"]
	if !ok || len(data) == 0 {
		log.Info("The certificate is missing, it should be renewed", "secretName", mtlsSecret.Name)
		return true
	}

	block, _ := pem.Decode(data)
	if block == nil {
		log.Error(nil, "Failed to decode the certificate; it should be renewed", "secretName", mtlsSecret.Name)
		return true
	}

	certs, err := x509.ParseCertificates(block.Bytes)
	if err != nil || len(certs) == 0 {
		log.Error(err, "Failed to parse the certificate, it should be renewed", "secretName", mtlsSecret.Name)
		return true
	}

	leafCert := certs[0]

	lifetime := leafCert.NotAfter.Sub(leafCert.NotBefore)
	renewThreshold := lifetime / 5
	renewTime := leafCert.NotAfter.Add(-renewThreshold)

	if time.Now().After(renewTime) {
		log.Info("The certificate expires soon, it should be renewed", "notAfter", leafCert.NotAfter, "secretName", mtlsSecret.Name)
		return true
	}

	return false
}
