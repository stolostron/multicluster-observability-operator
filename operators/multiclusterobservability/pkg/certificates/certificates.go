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

	mcov1beta2 "github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	"github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	"github.com/open-cluster-management/multicluster-observability-operator/operators/pkg/util"
)

const (
	serverCACertifcateCN = "observability-server-ca-certificate"
	serverCACerts        = config.ServerCACerts
	serverCertificateCN  = config.ServerCertCN
	serverCerts          = config.ServerCerts

	clientCACertificateCN = "observability-client-ca-certificate"
	clientCACerts         = config.ClientCACerts
	grafanaCertificateCN  = config.GrafanaCN
	grafanaCerts          = config.GrafanaCerts
)

var (
	log               = logf.Log.WithName("controller_certificates")
	serialNumberLimit = new(big.Int).Lsh(big.NewInt(1), 128)
)

func CreateObservabilityCerts(c client.Client, scheme *runtime.Scheme, mco *mcov1beta2.MultiClusterObservability, ingressCtlCrdExists bool) error {

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
		if !errors.IsNotFound(err) {
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
		if !errors.IsNotFound(err) {
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
					if !util.Contains(serverCrt.DNSNames, dnsString) {
						isRenew = true
						break
					}
				}
			}
		}

		if !isRenew {
			log.Info("Certificate secrets already existed", "name", name)
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
	err := c.Get(context.TODO(), types.NamespacedName{Namespace: config.GetDefaultNamespace(), Name: caCertName}, caSecret)
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
	if ingressCtlCrdExists {
		url, err := config.GetObsAPIHost(c, config.GetDefaultNamespace())
		if err != nil {
			log.Error(err, "Failed to get api route address")
			return nil, err
		} else {
			hosts = append(hosts, url)
		}
	}
	return hosts, nil
}
