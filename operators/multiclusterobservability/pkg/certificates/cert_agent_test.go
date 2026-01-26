// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package certificates

import (
	"crypto/sha256"
	"fmt"
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
	addon := &v1alpha1.ManagedClusterAddOn{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "observability-controller",
			Namespace: clusterName,
		},
	}
	configs, err := options.Registration.CSRConfigurations(cluster, addon)
	if err != nil {
		t.Fatalf("Failed to get CSR configurations: %v", err)
	}
	expectedCSRs := 2
	if len(configs) != expectedCSRs {
		t.Fatalf("expected %d CSRs, found %d", expectedCSRs, len(configs))
	}

	caHashOrgUnit := fmt.Sprintf("ca-hash-%x", sha256.Sum256(cert))

	kubeAPISignerExpectedRegConfig := v1alpha1.RegistrationConfig{
		SignerName: "kubernetes.io/kube-apiserver-client",
		Subject: v1alpha1.Subject{
			User: "system:open-cluster-management:cluster:test:addon:observability-controller:agent:observability",
			Groups: []string{
				"system:open-cluster-management:cluster:test:addon:observability-controller",
				"system:open-cluster-management:addon:observability-controller",
				"system:authenticated",
			},
			OrganizationUnits: []string{caHashOrgUnit},
		},
	}
	assert.Contains(t, configs, kubeAPISignerExpectedRegConfig)

	obsSignerExpectedRegConfig := v1alpha1.RegistrationConfig{
		SignerName: "open-cluster-management.io/observability-signer",
		Subject: v1alpha1.Subject{
			User:              "managed-cluster-observability",
			Groups:            nil,
			OrganizationUnits: []string{"acm", caHashOrgUnit},
		},
	}
	assert.Contains(t, configs, obsSignerExpectedRegConfig)
}
