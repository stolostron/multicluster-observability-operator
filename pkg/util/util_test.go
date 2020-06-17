// Copyright (c) 2020 Red Hat, Inc.

package util

import (
	"testing"
)

func TestRemove(t *testing.T) {
	s := []string{"one", "two", "three"}
	s = Remove(s, "two")
	if len(s) != 2 {
		t.Errorf("the length of string (%v) is not the expected (3)", len(s))
	}
}
