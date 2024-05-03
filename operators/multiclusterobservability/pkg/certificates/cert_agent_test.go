// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package certificates

import (
	"crypto/sha256"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"open-cluster-management.io/api/addon/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1 "open-cluster-management.io/api/cluster/v1"

	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
)

func TestCertAgent(t *testing.T) {
	cert, key, err := NewSigningCertKeyPair("testing-mco", 365*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	caSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serverCACerts,
			Namespace: config.GetDefaultNamespace(),
		},
		Data: map[string][]byte{
			"tls.crt": cert,
			"tls.key": key,
		},
	}

	scheme := runtime.NewScheme()
	scheme.AddKnownTypes(corev1.SchemeGroupVersion, &corev1.Secret{})
	client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(caSecret).Build()

	agent := &ObservabilityAgent{client: client}
	agent.Manifests(nil, nil)
	options := agent.GetAgentAddonOptions()
	cluster := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: clusterName,
		},
	}
	configs := options.Registration.CSRConfigurations(cluster)
	expectedCSRs := 2
	if len(configs) != expectedCSRs {
		t.Fatalf("expected %d CSRs, found %d", expectedCSRs, len(configs))
	}

	assert.True(t, slices.ContainsFunc(configs, func(reg v1alpha1.RegistrationConfig) bool {
		return reg.SignerName == "kubernetes.io/kube-apiserver-client"
	}))

	expectedOrganizationUnits := []string{"acm", fmt.Sprintf("ca-hash-%x", sha256.Sum256(cert))}
	expectedRegConfig := v1alpha1.RegistrationConfig{
		SignerName: "open-cluster-management.io/observability-signer",
		Subject: v1alpha1.Subject{
			User:              "managed-cluster-observability",
			Groups:            nil,
			OrganizationUnits: expectedOrganizationUnits,
		},
	}
	assert.Contains(t, configs, expectedRegConfig)
}
