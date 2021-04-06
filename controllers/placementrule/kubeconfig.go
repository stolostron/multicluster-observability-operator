// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package placementrule

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/url"
	"strings"

	ocinfrav1 "github.com/openshift/api/config/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	clientv1 "k8s.io/client-go/tools/clientcmd/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/open-cluster-management/multicluster-observability-operator/pkg/config"
)

const (
	kubeConfigName           = "hub-kube-config"
	infrastructureConfigName = "cluster"
	apiserverConfigName      = "cluster"
	openshiftConfigNamespace = "openshift-config"
)

func createKubeConfig(client client.Client, restMapper meta.RESTMapper, namespace string) (*clientv1.Config, error) {
	ca, token, err := getSAToken(client, namespace)
	if err != nil {
		return nil, err
	}

	apiServer, err := config.GetKubeAPIServerAddress(client)
	if err != nil {
		return nil, err
	}
	// if there is customized certs for api server, use the customized cert for kubeconfig
	isCustomCA := false
	if u, err := url.Parse(apiServer); err == nil {
		apiServerCertSecretName, err := getKubeAPIServerSecretName(client, restMapper, u.Hostname())
		if err != nil {
			return nil, err
		}
		if len(apiServerCertSecretName) > 0 {
			apiServerCert, err := getKubeAPIServerCertificate(client, apiServerCertSecretName)
			if err != nil {
				return nil, err
			}
			ca = apiServerCert
			isCustomCA = true
		}
	}

	if !isCustomCA {
		// check if it's roks
		// if it's ocp && it's on ibm cloud, we treat it as roks
		isROKS, err := config.CheckIsIBMCloud(client)
		if err != nil {
			return nil, err
		}
		if isROKS {
			// ROKS should have a certificate that is signed by trusted CA
			if certs, err := getValidCertificatesFromURL(apiServer, nil); err != nil {
				return nil, err
			} else if len(certs) > 0 {
				// simply don't give any certs as the apiserver is using certs signed by known CAs
				ca = nil
			} else {
				log.Info("No additional valid certificate found for APIserver. Skipping.")
			}
		}
	}

	return &clientv1.Config{
		Kind:       "Config",
		APIVersion: "v1",
		Clusters: []clientv1.NamedCluster{
			{
				Name: "default-cluster",
				Cluster: clientv1.Cluster{
					Server:                   apiServer,
					CertificateAuthorityData: ca,
				},
			},
		},
		AuthInfos: []clientv1.NamedAuthInfo{
			{
				Name: "default-user",
				AuthInfo: clientv1.AuthInfo{
					Token: string(token),
				},
			},
		},
		Contexts: []clientv1.NamedContext{
			{
				Name: "default-context",
				Context: clientv1.Context{
					Cluster:   "default-cluster",
					AuthInfo:  "default-user",
					Namespace: namespace,
				},
			},
		},
		CurrentContext: "default-context",
	}, nil
}

func createKubeSecret(client client.Client, restMapper meta.RESTMapper, namespace string) (*corev1.Secret, error) {
	config, err := createKubeConfig(client, restMapper, namespace)
	if err != nil {
		return nil, err
	}
	configYaml, err := yaml.Marshal(config)
	if err != nil {
		return nil, err
	}
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      kubeConfigName,
			Namespace: spokeNameSpace,
		},
		Data: map[string][]byte{
			"kubeconfig": configYaml,
		},
	}, nil
}

func getKubeAPIServerAddress(client client.Client) (string, error) {
	infraConfig := &ocinfrav1.Infrastructure{}

	if err := client.Get(context.TODO(),
		types.NamespacedName{Name: infrastructureConfigName}, infraConfig); err != nil {
		return "", err
	}

	return infraConfig.Status.APIServerURL, nil
}

// getKubeAPIServerSecretName iterate through all namespacedCertificates
// returns the first one which has a name matches the given dnsName
func getKubeAPIServerSecretName(client client.Client, restMapper meta.RESTMapper, dnsName string) (string, error) {

	if restMapper != nil {
		gk := schema.GroupKind{Group: ocinfrav1.GroupVersion.Group, Kind: "APIServer"}
		_, err := restMapper.RESTMapping(gk, ocinfrav1.GroupVersion.Version)
		if err != nil {
			log.Info("the server doesn't have a resource type APIServer", "error", err)
			return "", nil
		}
	}

	apiserver := &ocinfrav1.APIServer{}
	if err := client.Get(
		context.TODO(),
		types.NamespacedName{Name: apiserverConfigName},
		apiserver,
	); err != nil {
		if errors.IsNotFound(err) {
			log.Info("APIServer cluster not found")
			return "", nil
		}
		return "", err
	}
	// iterate through all namedcertificates
	for _, namedCert := range apiserver.Spec.ServingCerts.NamedCertificates {
		for _, name := range namedCert.Names {
			if strings.EqualFold(name, dnsName) {
				return namedCert.ServingCertificate.Name, nil
			}
		}
	}
	return "", nil
}

// getKubeAPIServerCertificate looks for secret in openshift-config namespace, and returns tls.crt
func getKubeAPIServerCertificate(client client.Client, secretName string) ([]byte, error) {
	secret := &corev1.Secret{}
	if err := client.Get(
		context.TODO(),
		types.NamespacedName{Name: secretName, Namespace: openshiftConfigNamespace},
		secret,
	); err != nil {
		log.Error(err, fmt.Sprintf("Failed to get secret %s/%s", openshiftConfigNamespace, secretName))
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	if secret.Type != corev1.SecretTypeTLS {
		return nil, fmt.Errorf(
			"secret %s/%s should have type=kubernetes.io/tls",
			openshiftConfigNamespace,
			secretName,
		)
	}
	res, ok := secret.Data["tls.crt"]
	if !ok {
		return nil, fmt.Errorf(
			"failed to find data[tls.crt] in secret %s/%s",
			openshiftConfigNamespace,
			secretName,
		)
	}
	return res, nil
}

// getValidCertificatesFromURL dial to serverURL and get certificates
// only will return certificates signed by trusted ca and verified (with verifyOptions)
// if certificates are all signed by unauthorized party, will return nil
// rootCAs is for tls handshake verification
func getValidCertificatesFromURL(serverURL string, rootCAs *x509.CertPool) ([]*x509.Certificate, error) {
	u, err := url.Parse(serverURL)
	if err != nil {
		log.Error(err, "failed to parse url: "+serverURL)
		return nil, err
	}
	log.Info("getting certificate of " + u.Hostname() + ":" + u.Port())
	conf := &tls.Config{
		// server should support tls1.2
		MinVersion: tls.VersionTLS12,
		ServerName: u.Hostname(),
	}
	if rootCAs != nil {
		conf.RootCAs = rootCAs
	}

	conn, err := tls.Dial("tcp", u.Hostname()+":"+u.Port(), conf)

	if err != nil {
		log.Error(err, "failed to dial "+serverURL)
		// ignore certificate signed by unknown authority error
		if _, ok := err.(x509.UnknownAuthorityError); ok {
			return nil, nil
		}
		return nil, err
	}
	defer conn.Close()
	certs := conn.ConnectionState().PeerCertificates
	retCerts := []*x509.Certificate{}
	opt := x509.VerifyOptions{Roots: rootCAs}
	// check certificates
	for _, cert := range certs {
		if _, err := cert.Verify(opt); err == nil {
			log.Info("Adding a valid certificate")
			retCerts = append(retCerts, cert)
		} else {
			log.Info("Skipping an invalid certificate")
		}
	}
	return retCerts, nil
}
