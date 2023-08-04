// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package placementrule

import (
	"testing"

	operatorconfig "github.com/stolostron/multicluster-observability-operator/operators/pkg/config"
)

const (
	name = "test-ns"
)

func TestGenerateNamespace(t *testing.T) {
	spokeNameSpace = name
	namespace := generateNamespace()
	if namespace.Name != name {
		t.Fatal("Wrong namespace created")
	}

	annotations := namespace.GetAnnotations()
	value, found := annotations[operatorconfig.WorkloadPartitioningNSAnnotationsKey]

	if !found || value != operatorconfig.WorkloadPartitioningNSExpectedValue {
		t.Fatalf("Failed to find annotation %v: %v on namespace: %v)",
			operatorconfig.WorkloadPartitioningNSAnnotationsKey,
			operatorconfig.WorkloadPartitioningNSExpectedValue,
			name,
		)
	}
}
