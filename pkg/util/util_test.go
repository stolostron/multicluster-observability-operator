// Copyright (c) 2020 Red Hat, Inc.

package util

import (
	"testing"
)

func TestRemove(t *testing.T) {
	s := []string{"one", "two", "three"}
	s = Remove(s, "two")
	if len(s) != 2 {
		t.Errorf("the length of string (%v) is not the expected (2)", len(s))
	}
}

func TestGetAnnotation(t *testing.T) {
	tmpAnnotations := map[string]string{
		"repo": "1",
		"test": "2",
	}
	if GetAnnotation(tmpAnnotations, "repo") != "1" {
		t.Errorf("repo (%v) is not the expected (%v)", GetAnnotation(tmpAnnotations, "repo"), "1")
	}
	if GetAnnotation(tmpAnnotations, "failed") != "" {
		t.Errorf("failed (%v) is not the expected (%v)", GetAnnotation(tmpAnnotations, "repo"), "")
	}
}
