// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package placement

import (
	"testing"
)

const (
	name = "test-ns"
)

func TestCreateNameSpace(t *testing.T) {
	spokeNameSpace = name
	namespace := createNameSpace()
	if namespace.Name != name {
		t.Fatal("Wrong namespace created")
	}
}
