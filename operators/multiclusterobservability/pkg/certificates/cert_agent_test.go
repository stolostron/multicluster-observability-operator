// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package certificates

import (
	"context"
	"crypto/sha256"
	"fmt"
	"testing"
	"time"

	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"open-cluster-management.io/addon-framework/pkg/agent"
	addonv1beta1 "open-cluster-management.io/api/addon/v1beta1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestCertAgent(t *testing.T) {
	ctx := context.Background()
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

	obsAgent := &ObservabilityAgent{client: client}
	obsAgent.Manifests(ctx, nil, nil)
	options := obsAgent.GetAgentAddonOptions()
	cluster := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: clusterName,
		},
	}
	addon := &addonv1beta1.ManagedClusterAddOn{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "observability-controller",
			Namespace: clusterName,
		},
	}
	configs, err := options.Registration.Configurations(ctx, cluster, addon)
	if err != nil {
		t.Fatalf("Failed to get CSR configurations: %v", err)
	}
	expectedCSRs := 2
	if len(configs) != expectedCSRs {
		t.Fatalf("expected %d CSRs, found %d", expectedCSRs, len(configs))
	}

	caHashOrgUnit := fmt.Sprintf("ca-hash-%x", sha256.Sum256(cert))

	kubeAPISignerExpectedRegConfig := &agent.KubeClientRegistration{
		User: "system:open-cluster-management:cluster:test:addon:observability-controller:agent:observability",
		Groups: []string{
			"system:open-cluster-management:cluster:test:addon:observability-controller",
			"system:open-cluster-management:addon:observability-controller",
			// TODO(guidonguido): check if system:authenticated group is actually expected
			// "system:authenticated". It has been removed in addon-framework v1.3.0
			caHashOrgUnit,
		},
	}
	assert.Equal(t, configs[0], kubeAPISignerExpectedRegConfig)

	obsSignerExpectedRegConfig := &agent.CustomSignerRegistration{
		SignerName:        "open-cluster-management.io/observability-signer",
		User:              "managed-cluster-observability",
		Groups:            nil,
		OrganizationUnits: []string{"acm", caHashOrgUnit},
	}
	assert.Equal(t, configs[1], obsSignerExpectedRegConfig)
}
