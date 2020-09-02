// Copyright (c) 2020 Red Hat, Inc.

package multiclusterobservability

import (
	"context"
	"reflect"

	cert "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	mcov1beta1 "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/apis/observability/v1beta1"
	"github.com/open-cluster-management/multicluster-monitoring-operator/pkg/config"
)

const (
	certMgrClusterRsNs = "ibm-common-services"

	serverSelfSignIssuer = "observability-server-selfsign-issuer"
	serverCAIssuer       = "observability-server-ca-issuer"
	serverCACertifcate   = "observability-server-ca-certificate"
	serverCACerts        = "observability-server-ca-certs"
	serverCertificate    = "observability-server-certificate"
	serverCerts          = "observability-server-certs"

	clientSelfSignIssuer = "observability-client-selfsign-issuer"
	clientCAIssuer       = "observability-client-ca-issuer"
	clientCACertifcate   = "observability-client-ca-certificate"
	clientCACerts        = "observability-client-ca-certs"

	grafanaCertificate = "observability-grafana-certificate"
	grafanaCerts       = "observability-grafana-certs"
	grafanaSubject     = "grafana"

	managedClusterCertificate = "observability-managed-cluster-certificate"
	managedClusterCerts       = "observability-managed-cluster-certs"
	managedClusterCertOrg     = "acm"
)

// CreateCertificate is used to create Certificate resource
func CreateCertificate(client client.Client, scheme *runtime.Scheme,
	mco *mcov1beta1.MultiClusterObservability,
	name string, namespace string,
	secret string, isClusterIssuer bool, issuer string, isCA bool,
	commonName string, organizations []string, dnsNames []string) error {

	spec := cert.CertificateSpec{}
	spec.SecretName = secret
	kind := "Issuer"
	if isClusterIssuer {
		kind = "ClusterIssuer"
	}
	if isCA {
		spec.IsCA = isCA
	}
	spec.IssuerRef = cert.ObjectReference{
		Kind: kind,
		Name: issuer,
	}
	if commonName != "" {
		spec.CommonName = commonName
	}
	if len(organizations) != 0 {
		spec.Organization = organizations
	}
	if len(dnsNames) != 0 {
		spec.DNSNames = dnsNames
	}

	certificate := &cert.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: spec,
	}

	// Set MultiClusterObservability instance as the owner and controller
	if namespace == config.GetDefaultNamespace() {
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
	issuer := &cert.ClusterIssuer{
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
		err = client.Create(context.TODO(), issuer)
		if err != nil {
			log.Error(err, "Failed to create ClusterIssuer", "name", name)
			return err
		}
		return nil
	} else if err != nil {
		log.Error(err, "Failed to check ClusterIssuer", "name", name)
		return err
	}

	if !reflect.DeepEqual(found.Spec, issuer.Spec) {
		log.Info("Updating ClusterIssuer", "name", name)
		issuer.ObjectMeta.ResourceVersion = found.ObjectMeta.ResourceVersion
		err = client.Update(context.TODO(), issuer)
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
	mco *mcov1beta1.MultiClusterObservability,
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
	mco *mcov1beta1.MultiClusterObservability) error {

	log.Info("Creating server side certificates")
	ns := config.GetDefaultNamespace()

	err := createIssuer(client, scheme, mco,
		serverSelfSignIssuer, ns, "")
	if err != nil {
		return err
	}

	err = CreateCertificate(client, scheme, mco,
		serverCACertifcate, ns,
		serverCACerts, false, serverSelfSignIssuer, true,
		serverCACertifcate, []string{}, []string{})
	if err != nil {
		return err
	}

	err = createIssuer(client, scheme, mco,
		serverCAIssuer, ns, serverCACerts)
	if err != nil {
		return err
	}

	err = CreateCertificate(client, scheme, mco,
		serverCertificate, ns,
		serverCerts, false, serverCAIssuer, false,
		serverCertificate, []string{}, []string{})
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

	err = CreateCertificate(client, scheme, mco,
		clientCACertifcate, certMgrClusterRsNs,
		clientCACerts, false, clientSelfSignIssuer, true,
		clientCACertifcate, []string{}, []string{})
	if err != nil {
		return err
	}

	err = createClusterIssuer(client, clientCAIssuer, clientCACerts)
	if err != nil {
		return err
	}

	log.Info("Creating certificates for grafana")
	err = CreateCertificate(client, scheme, mco,
		grafanaCertificate, config.GetDefaultNamespace(),
		grafanaCerts, true, clientCAIssuer, false,
		grafanaSubject, []string{}, []string{})
	if err != nil {
		return err
	}

	return nil
}
