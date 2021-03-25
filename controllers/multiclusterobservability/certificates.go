// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package multiclusterobservability

import (
	"context"
	"net"
	"os"
	"reflect"
	"time"

	cert "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	mcov1beta2 "github.com/open-cluster-management/multicluster-observability-operator/api/v1beta2"
	"github.com/open-cluster-management/multicluster-observability-operator/pkg/config"
)

const (
	serverSelfSignIssuer = "observability-server-selfsign-issuer"
	serverCAIssuer       = "observability-server-ca-issuer"
	serverCACertifcate   = "observability-server-ca-certificate"
	serverCACerts        = "observability-server-ca-certs"
	serverCertificate    = "observability-server-certificate"
	serverCerts          = config.ServerCerts

	clientSelfSignIssuer = "observability-client-selfsign-issuer"
	clientCAIssuer       = "observability-client-ca-issuer"
	clientCACertificate  = "observability-client-ca-certificate"
	clientCACerts        = "observability-client-ca-certs"

	grafanaCertificate = "observability-grafana-certificate"
	grafanaSubject     = "grafana"
	grafanaCerts       = config.GrafanaCerts

	managedClusterCertOrg = "acm"
)

var (
	certMgrClusterRsNs = os.Getenv("POD_NAMESPACE") + "-issuer"
)

// GetManagedClusterOrg is used to return managedClusterCertOrg
func GetManagedClusterOrg() string {
	return managedClusterCertOrg
}

// GetGrafanaSubject is used to return grafanaSubject
func GetGrafanaSubject() string {
	return grafanaSubject
}

// GetClientCAIssuer is used to return clientCAIssuer
func GetClientCAIssuer() string {
	return clientCAIssuer
}

// GetClientCACert is used to return clientCACert
func GetClientCACert() string {
	return grafanaCerts
}

// GetServerCerts is used to return serverCerts
func GetServerCerts() string {
	return serverCerts
}

// GetGrafanaCerts is used to return grafanaCerts
func GetGrafanaCerts() string {
	return grafanaCerts
}

// CreateCertificateSpec is used to create a struct of CertificateSpec
func CreateCertificateSpec(secret string,
	isClusterIssuer bool, issuer string, isCA bool,
	commonName string, organizations []string, hosts []string) cert.CertificateSpec {

	spec := cert.CertificateSpec{}
	spec.SecretName = secret
	kind := "Issuer"
	if isClusterIssuer {
		kind = "ClusterIssuer"
	}
	if isCA {
		spec.IsCA = isCA
		spec.Duration = &metav1.Duration{Duration: time.Hour * 24 * 365 * 5}
	} else {
		spec.Duration = &metav1.Duration{Duration: time.Hour * 24 * 365}
	}
	spec.IssuerRef = cert.ObjectReference{
		Kind: kind,
		Name: issuer,
	}
	if commonName != "" {
		spec.CommonName = commonName
	}
	if len(organizations) != 0 {
		spec.Subject = &cert.X509Subject{
			OrganizationalUnits: organizations,
		}
	}
	if len(hosts) != 0 {
		dns := []string{}
		ips := []string{}
		for _, host := range hosts {
			addr := net.ParseIP(host)
			if addr != nil {
				ips = append(ips, host)
			} else {
				dns = append(dns, host)
			}
		}
		if len(dns) != 0 {
			spec.DNSNames = dns
		}
		if len(ips) != 0 {
			spec.IPAddresses = ips
		}
	}
	return spec
}

// CreateCertificate is used to create Certificate resource
func CreateCertificate(client client.Client, scheme *runtime.Scheme,
	mco *mcov1beta2.MultiClusterObservability,
	name string, namespace string,
	spec cert.CertificateSpec) error {

	certificate := &cert.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: spec,
	}

	// Set MultiClusterObservability instance as the owner and controller
	if namespace == config.GetDefaultNamespace() && mco != nil {
		if err := controllerutil.SetControllerReference(mco, certificate, scheme); err != nil {
			return err
		}
	}

	found := &cert.Certificate{}
	err := client.Get(context.TODO(), types.NamespacedName{Name: name, Namespace: namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating Certificate", "name", name)
		err = client.Create(context.TODO(), certificate)
		if err != nil {
			log.Error(err, "Failed to create Certificate", "name", name)
			return err
		}
		return nil
	} else if err != nil {
		log.Error(err, "Failed to check Certificate", "name", name)
		return err
	}

	if !reflect.DeepEqual(found.Spec, certificate.Spec) {
		log.Info("Updating Certificate", "name", name)
		certificate.ObjectMeta.ResourceVersion = found.ObjectMeta.ResourceVersion
		err = client.Update(context.TODO(), certificate)
		if err != nil {
			log.Error(err, "Failed to update Certificate", "name", name)
			return err
		}
		return nil
	}

	log.Info("Certificate already existed/unchanged", "name", name)
	return nil
}

func createClusterIssuer(client client.Client, name string, ca string) error {
	clusterIssuer := &cert.ClusterIssuer{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: cert.IssuerSpec{
			IssuerConfig: cert.IssuerConfig{
				CA: &cert.CAIssuer{
					SecretName: ca,
				},
			},
		},
	}

	found := &cert.ClusterIssuer{}
	err := client.Get(context.TODO(), types.NamespacedName{Name: name}, found)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating ClusterIssuer", "name", name)
		err = client.Create(context.TODO(), clusterIssuer)
		if err != nil {
			log.Error(err, "Failed to create ClusterIssuer", "name", name)
			return err
		}
		return nil
	} else if err != nil {
		log.Error(err, "Failed to check ClusterIssuer", "name", name)
		return err
	}

	if !reflect.DeepEqual(found.Spec, clusterIssuer.Spec) {
		log.Info("Updating ClusterIssuer", "name", name)
		clusterIssuer.ObjectMeta.ResourceVersion = found.ObjectMeta.ResourceVersion
		err = client.Update(context.TODO(), clusterIssuer)
		if err != nil {
			log.Error(err, "Failed to update ClusterIssuer", "name", name)
			return err
		}
		return nil
	}

	log.Info("ClusterIssuer already existed/unchanged", "name", name)
	return nil
}

func createIssuer(client client.Client, scheme *runtime.Scheme,
	mco *mcov1beta2.MultiClusterObservability,
	name string, namespace string, ca string) error {
	isserConfig := cert.IssuerConfig{}
	if ca != "" {
		isserConfig = cert.IssuerConfig{
			CA: &cert.CAIssuer{
				SecretName: ca,
			},
		}
	} else {
		isserConfig = cert.IssuerConfig{
			SelfSigned: &cert.SelfSignedIssuer{},
		}
	}
	issuer := &cert.Issuer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: cert.IssuerSpec{
			IssuerConfig: isserConfig,
		},
	}

	// Set MultiClusterObservability instance as the owner and controller
	if namespace == config.GetDefaultNamespace() {
		if err := controllerutil.SetControllerReference(mco, issuer, scheme); err != nil {
			return err
		}
	}

	found := &cert.Issuer{}
	err := client.Get(context.TODO(), types.NamespacedName{Name: name, Namespace: namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating Issuer", "name", name)
		err = client.Create(context.TODO(), issuer)
		if err != nil {
			log.Error(err, "Failed to create Issuer", "name", name)
			return err
		}
		return nil
	} else if err != nil {
		log.Error(err, "Failed to check Issuer", "name", name)
		return err
	}

	if !reflect.DeepEqual(found.Spec, issuer.Spec) {
		log.Info("Updating Issuer", "name", name)
		issuer.ObjectMeta.ResourceVersion = found.ObjectMeta.ResourceVersion
		err = client.Update(context.TODO(), issuer)
		if err != nil {
			log.Error(err, "Failed to update Issuer", "name", name)
			return err
		}
		return nil
	}

	log.Info("Issuer already existed/unchanged", "name", name)
	return nil
}

func createNamespace(client client.Client) error {
	ns := &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "Namespace",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: certMgrClusterRsNs,
		},
	}
	found := &corev1.Namespace{}
	err := client.Get(context.TODO(), types.NamespacedName{Name: certMgrClusterRsNs}, found)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating Namespace", "name", certMgrClusterRsNs)
		err = client.Create(context.TODO(), ns)
		if err != nil {
			log.Error(err, "Failed to create Namespace", "name", certMgrClusterRsNs)
			return err
		}
		return nil
	} else if err != nil {
		log.Error(err, "Failed to check Namespace", "name", certMgrClusterRsNs)
		return err
	}
	log.Info("Namespace already existed", "name", certMgrClusterRsNs)
	return nil
}

func createObservabilityCertificate(client client.Client, scheme *runtime.Scheme,
	mco *mcov1beta2.MultiClusterObservability) error {

	log.Info("Creating server side certificates")
	ns := config.GetDefaultNamespace()

	err := createIssuer(client, scheme, mco,
		serverSelfSignIssuer, ns, "")
	if err != nil {
		return err
	}

	spec := CreateCertificateSpec(serverCACerts, false, serverSelfSignIssuer, true,
		serverCACertifcate, []string{}, []string{})
	err = CreateCertificate(client, scheme, mco,
		serverCACertifcate, ns, spec)
	if err != nil {
		return err
	}

	err = createIssuer(client, scheme, mco,
		serverCAIssuer, ns, serverCACerts)
	if err != nil {
		return err
	}

	hosts := []string{config.GetObsAPISvc(mco.GetName())}
	url, err := config.GetObsAPIUrl(client, ns)
	if err != nil {
		log.Info("Failed to get api route address", "error", err.Error())
	} else {
		hosts = append(hosts, url)
	}

	spec = CreateCertificateSpec(serverCerts, false, serverCAIssuer, false,
		serverCertificate, []string{}, hosts)
	err = CreateCertificate(client, scheme, mco,
		serverCertificate, ns, spec)
	if err != nil {
		return err
	}

	log.Info("Creating cluster issuer for client")
	err = createNamespace(client)
	if err != nil {
		return err
	}

	err = createIssuer(client, scheme, mco,
		clientSelfSignIssuer, certMgrClusterRsNs, "")
	if err != nil {
		return err
	}

	spec = CreateCertificateSpec(clientCACerts, false, clientSelfSignIssuer, true,
		clientCACertificate, []string{}, []string{})
	err = CreateCertificate(client, scheme, mco,
		clientCACertificate, certMgrClusterRsNs, spec)
	if err != nil {
		return err
	}

	err = createClusterIssuer(client, clientCAIssuer, clientCACerts)
	if err != nil {
		return err
	}

	log.Info("Creating certificates for grafana")
	spec = CreateCertificateSpec(grafanaCerts, true, clientCAIssuer, false,
		grafanaSubject, []string{}, []string{})
	err = CreateCertificate(client, scheme, mco,
		grafanaCertificate, config.GetDefaultNamespace(), spec)
	if err != nil {
		return err
	}

	return nil
}

// only need to clean the issuer/certificate in other namespace
func cleanIssuerCert(client client.Client) error {
	foundClusterIssuer := &cert.ClusterIssuer{}
	err := client.Get(context.TODO(), types.NamespacedName{Name: clientCAIssuer}, foundClusterIssuer)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Issuer doesn't exist", "name", clientCAIssuer)
	} else if err != nil {
		log.Error(err, "Failed to check clusterissuer", "name", clientCAIssuer)
		return err
	} else {
		err = client.Delete(context.TODO(), foundClusterIssuer)
		if err != nil {
			log.Error(err, "Failed to delete clusterissuer", "name", clientCAIssuer)
			return err
		}
	}

	foundIssuer := &cert.Issuer{}
	err = client.Get(context.TODO(),
		types.NamespacedName{Name: clientSelfSignIssuer, Namespace: certMgrClusterRsNs}, foundIssuer)
	if err != nil && errors.IsNotFound(err) {
		log.Info("ClusterIssuer doesn't exist", "name", clientSelfSignIssuer, "namespace", certMgrClusterRsNs)
	} else if err != nil {
		log.Error(err, "Failed to check issuer", "name", clientSelfSignIssuer, "namespace", certMgrClusterRsNs)
		return err
	} else {
		err = client.Delete(context.TODO(), foundIssuer)
		if err != nil {
			log.Error(err, "Failed to delete issuer", "name", clientSelfSignIssuer, "namespace", certMgrClusterRsNs)
			return err
		}
	}

	foundCert := &cert.Certificate{}
	err = client.Get(context.TODO(),
		types.NamespacedName{Name: clientCACertificate, Namespace: certMgrClusterRsNs}, foundCert)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Certificate doesn't exist", "name", clientCACertificate, "namespace", certMgrClusterRsNs)
	} else if err != nil {
		log.Error(err, "Failed to check Certificate", "name", clientCACertificate, "namespace", certMgrClusterRsNs)
		return err
	} else {
		err = client.Delete(context.TODO(), foundCert)
		if err != nil {
			log.Error(err, "Failed to delete Certificate", "name", clientCACertificate, "namespace", certMgrClusterRsNs)
			return err
		}
	}

	return nil
}
