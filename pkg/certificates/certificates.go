// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package certificates

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	mcov1beta2 "github.com/open-cluster-management/multicluster-observability-operator/api/v1beta2"
	"github.com/open-cluster-management/multicluster-observability-operator/pkg/config"
)

const (
	serverCACertifcateCN = "observability-server-ca-certificate"
	serverCACerts        = config.ServerCACerts
	serverCertificateCN  = config.ServerCertCN
	serverCerts          = config.ServerCerts

	clientCACertificateCN = "observability-client-ca-certificate"
	clientCACerts         = config.ClientCACerts

	grafanaCertificateCN = config.GrafanaCN
	grafanaCerts         = config.GrafanaCerts
)

var (
	log               = logf.Log.WithName("controller_certificates")
	serialNumberLimit = new(big.Int).Lsh(big.NewInt(1), 128)
)

func CreateObservabilityCerts(c client.Client, scheme *runtime.Scheme, mco *mcov1beta2.MultiClusterObservability) error {
	err := createCASecret(c, scheme, mco, serverCACerts, serverCACertifcateCN)
	if err != nil {
		return err
	}
	err = createCASecret(c, scheme, mco, clientCACerts, clientCACertificateCN)
	if err != nil {
		return err
	}

	hosts := []string{config.GetObsAPISvc(mco.GetName())}
	url, err := config.GetObsAPIUrl(c, config.GetDefaultNamespace())
	if err != nil {
		log.Info("Failed to get api route address", "error", err.Error())
	} else {
		hosts = append(hosts, url)
	}
	err = createCertSecret(c, scheme, mco, serverCerts, true, serverCertificateCN, nil, hosts, nil)
	if err != nil {
		return err
	}

	err = createCertSecret(c, scheme, mco, grafanaCerts, false, grafanaCertificateCN, nil, nil, nil)
	if err != nil {
		return err
	}

	return nil
}

func createCASecret(c client.Client,
	scheme *runtime.Scheme, mco *mcov1beta2.MultiClusterObservability,
	name string, cn string) error {
	caSecret := &corev1.Secret{}
	err := c.Get(context.TODO(), types.NamespacedName{Namespace: config.GetDefaultNamespace(), Name: name}, caSecret)
	if err != nil {
		if !errors.IsNotFound(err) {
			log.Error(err, "Failed to check ca secret", "name", name)
			return err
		} else {
			key, cert, err := createCACertificate(cn)
			if err != nil {
				return err
			}
			certPEM := new(bytes.Buffer)
			pem.Encode(certPEM, &pem.Block{
				Type:  "CERTIFICATE",
				Bytes: cert,
			})

			keyPEM := new(bytes.Buffer)
			pem.Encode(keyPEM, &pem.Block{
				Type:  "RSA PRIVATE KEY",
				Bytes: key,
			})
			caSecret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: config.GetDefaultNamespace(),
				},
				Data: map[string][]byte{
					"ca.crt":  certPEM.Bytes(),
					"tls.crt": certPEM.Bytes(),
					"tls.key": keyPEM.Bytes(),
				},
			}
			if err := controllerutil.SetControllerReference(mco, caSecret, scheme); err != nil {
				return err
			}
			if err := c.Create(context.TODO(), caSecret); err != nil {
				log.Error(err, "Failed to create secret", "name", name)
				return err
			}
		}
	} else {
		log.Info("CA secrets already existed", "name", name)
	}
	return nil
}

func createCACertificate(cn string) ([]byte, []byte, error) {
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
		NotAfter:              time.Now().AddDate(5, 0, 0),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		log.Error(err, "Failed to generate private key", "cn", cn)
		return nil, nil, err
	}
	caBytes, err := x509.CreateCertificate(rand.Reader, ca, ca, &caKey.PublicKey, caKey)
	if err != nil {
		log.Error(err, "Failed to create certificate", "cn", cn)
		return nil, nil, err
	}
	caKeyBytes := x509.MarshalPKCS1PrivateKey(caKey)
	return caKeyBytes, caBytes, nil
}

func createCertSecret(c client.Client,
	scheme *runtime.Scheme, mco *mcov1beta2.MultiClusterObservability,
	name string, isServer bool,
	cn string, ou []string, dns []string, ips []net.IP) error {
	crtSecret := &corev1.Secret{}
	err := c.Get(context.TODO(), types.NamespacedName{Namespace: config.GetDefaultNamespace(), Name: name}, crtSecret)
	if err != nil {
		if !errors.IsNotFound(err) {
			log.Error(err, "Failed to check certificate secret", "name", name)
			return err
		} else {
			caSecret, caCert, caKey, err := getCA(c, isServer)
			if err != nil {
				return err
			}
			key, cert, err := createCertificate(isServer, cn, ou, dns, ips, caCert, caKey)
			if err != nil {
				return err
			}
			certPEM := new(bytes.Buffer)
			pem.Encode(certPEM, &pem.Block{
				Type:  "CERTIFICATE",
				Bytes: cert,
			})

			keyPEM := new(bytes.Buffer)
			pem.Encode(keyPEM, &pem.Block{
				Type:  "RSA PRIVATE KEY",
				Bytes: key,
			})
			crtSecret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: config.GetDefaultNamespace(),
				},
				Data: map[string][]byte{
					"ca.crt":  caSecret.Data["tls.crt"],
					"tls.crt": certPEM.Bytes(),
					"tls.key": keyPEM.Bytes(),
				},
			}
			if err := controllerutil.SetControllerReference(mco, crtSecret, scheme); err != nil {
				return err
			}
			err = c.Create(context.TODO(), crtSecret)
			if err != nil {
				log.Error(err, "Failed to create secret", "name", name)
				return err
			}
		}
	} else {
		log.Info("Certificate secrets already existed", "name", name)
	}
	return nil
}

func createCertificate(isServer bool, cn string, ou []string, dns []string, ips []net.IP,
	caCert *x509.Certificate, caKey *rsa.PrivateKey) ([]byte, []byte, error) {
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
		NotAfter:    time.Now().AddDate(1, 0, 0),
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
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		log.Error(err, "Failed to generate private key", "cn", cn)
		return nil, nil, err
	}
	caBytes, err := x509.CreateCertificate(rand.Reader, cert, caCert, &key.PublicKey, caKey)
	if err != nil {
		log.Error(err, "Failed to create certificate", "cn", cn)
		return nil, nil, err
	}
	keyBytes := x509.MarshalPKCS1PrivateKey(key)
	return keyBytes, caBytes, nil
}

func getCA(c client.Client, isServer bool) (*corev1.Secret, *x509.Certificate, *rsa.PrivateKey, error) {
	caCertName := serverCACerts
	if !isServer {
		caCertName = clientCACerts
	}
	caSecret := &corev1.Secret{}
	err := c.Get(context.TODO(), types.NamespacedName{Namespace: config.GetDefaultNamespace(), Name: caCertName}, caSecret)
	if err != nil {
		log.Error(err, "Failed to get ca secret", "name", caCertName)
		return nil, nil, nil, err
	}
	block1, _ := pem.Decode(caSecret.Data["tls.crt"])
	caCert, err := x509.ParseCertificate(block1.Bytes)
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
	return caSecret, caCert, caKey, nil
}
