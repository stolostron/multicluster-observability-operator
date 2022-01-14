// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package placementrule

import (
	"testing"
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
}
