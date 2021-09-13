// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package certificates

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1 "open-cluster-management.io/api/cluster/v1"
)

func TestCertAgent(t *testing.T) {
	agent := &ObservabilityAgent{}
	agent.Manifests(nil, nil)
	options := agent.GetAgentAddonOptions()
	cluster := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: clusterName,
		},
	}
	configs := options.Registration.CSRConfigurations(cluster)
	if len(configs) != 2 {
		t.Fatal("Wrong CSRConfigurations")
	}
}
