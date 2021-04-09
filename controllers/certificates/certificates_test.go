// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package certificates

import (
	"testing"

	certv1alpha1 "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1alpha1"
	routev1 "github.com/openshift/api/route/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	mcov1beta2 "github.com/open-cluster-management/multicluster-observability-operator/api/v1beta2"
	mcoconfig "github.com/open-cluster-management/multicluster-observability-operator/pkg/config"
)

func TestCreateCertificates(t *testing.T) {
	/* 	configLog := uzap.NewProductionEncoderConfig()
	   	logfmtEncoder := zaplogfmt.NewEncoder(configLog)
	   	logger := zap.New(zap.UseDevMode(true), zap.WriteTo(os.Stdout), zap.Encoder(logfmtEncoder))
	   	logf.SetLogger(logger) */
	var (
		name      = "observability"
		namespace = mcoconfig.GetDefaultNamespace()
	)
	mco := &mcov1beta2.MultiClusterObservability{
		TypeMeta:   metav1.TypeMeta{Kind: "MultiClusterObservability"},
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
		Spec:       mcov1beta2.MultiClusterObservabilitySpec{},
	}
	route := &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "observatorium-api",
			Namespace: namespace,
		},
		Spec: routev1.RouteSpec{
			Host: "apiServerURL",
		},
	}
	s := scheme.Scheme
	mcov1beta2.SchemeBuilder.AddToScheme(s)
	certv1alpha1.SchemeBuilder.AddToScheme(s)
	routev1.AddToScheme(s)

	c := fake.NewFakeClient(route)

	err := CreateObservabilityCerts(c, s, mco)
	if err != nil {
		t.Fatalf("CreateObservabilityCerts: (%v)", err)
	}

}
