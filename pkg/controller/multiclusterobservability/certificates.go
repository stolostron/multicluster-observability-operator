// Copyright (c) 2020 Red Hat, Inc.

package multiclusterobservability

import (
	"context"

	cert "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	serverSelfSignIssuer = "multicluster-hub-mcm-server-ca-issuer"
	serverCAIssuer       = "multicloud-ca-issuer"
	clientSelfSignIssuer = ""
	clientCAIssuer       = ""
	apiGatewayCerts      = ""
	grafanaCerts         = ""
	managedClusterCerts  = ""
	certGroup            = ""
)

func createClientIssuers(client client.Client) error {
	selfSignIssuer := &cert.ClusterIssuer{
		ObjectMeta: metav1.ObjectMeta{
			Name: serverSelfSignIssuer,
		},
		Spec: cert.IssuerSpec{
			IssuerConfig: cert.IssuerConfig{
				SelfSigned: &cert.SelfSignedIssuer{},
			},
		},
	}
	foundselfSignIssuer := &cert.ClusterIssuer{}
	err := client.Get(context.TODO(), types.NamespacedName{Name: serverSelfSignIssuer}, foundselfSignIssuer)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating ClusterIssuer", "name", serverSelfSignIssuer)
		err = client.Create(context.TODO(), selfSignIssuer)
		if err != nil {
			log.Error(err, "Failed to create ClusterIssuer", "name", serverSelfSignIssuer)
			return err
		}
		return nil
	} else if err != nil {
		log.Error(err, "Failed to check ClusterIssuer", "name", serverSelfSignIssuer)
		return err
	}
	log.Info("ClusterIssuer already existed", "name", serverSelfSignIssuer)

	caIssuer := &cert.ClusterIssuer{
		ObjectMeta: metav1.ObjectMeta{
			Name: serverCAIssuer,
		},
		Spec: cert.IssuerSpec{
			IssuerConfig: cert.IssuerConfig{
				CA: &cert.CAIssuer{
					SecretName: 
				},
			},
		},
	}

	return nil
}
