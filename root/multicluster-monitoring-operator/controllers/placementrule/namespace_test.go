// Copyright (c) 2020 Red Hat, Inc.

package placementrule

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
