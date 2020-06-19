// Copyright (c) 2020 Red Hat, Inc.

package placementrule

import (
	"testing"
)

const (
	name = "test-ns"
)

func init() {
	spokeNameSpace = name
}

func TestCreateNameSpace(t *testing.T) {
	namespace := createNameSpace()
	if namespace.Name != name {
		t.Fatal("Wrong namespace created")
	}
}
