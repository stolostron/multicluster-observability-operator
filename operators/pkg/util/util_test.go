// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package util

import (
	"encoding/base64"
	"os"
	"testing"
)

// Siddharth's code
func TestRemove(t *testing.T) {
	type testCaseList struct {
		name     string
		list     []string
		s        string
		expected []string
	}

	testCaseLists := []testCaseList{
		{"Should return string with 'test' removed", []string{"test", "test1", "test2"}, "test", []string{"test1", "test2"}},
		{"Should return identical list", []string{"test", "test1", "test2"}, "test3", []string{"test", "test1", "test2"}},
		{"Null String", []string{"", "test", "test1"}, "", []string{"test", "test1"}},
	}

	for _, test := range testCaseLists {
		output := Remove(test.list, test.s)

		for i, str := range output {
			if str != test.expected[i] {
				t.Errorf("case (%v) output: (%v) is not the expected: (%v)", test.name, output, test.expected)
			}
		}
	}
}

// Siddharth's code
func TestGetAnnotation(t *testing.T) {
	type testCaseList struct {
		name       string
		annotation map[string]string
		key        string
		expected   string
	}
	// tests whether func returns correct value for a given key
	fullMap := map[string]string{
		"a": "aTest",
		"b": "bTest",
		"c": "cTest",
	}

	// tests whether func returns nil for a nil map
	var nilMap map[string]string

	testCaseLists := []testCaseList{
		{"Fundamental key-value pair", fullMap, "b", "bTest"},
		{"Nil Map", nilMap, "", ""},
	}

	for _, test := range testCaseLists {
		output := GetAnnotation(test.annotation, test.key)

		if output != test.expected {
			t.Errorf("case (%v) output: (%v) is not the expected: (%v)", test.name, output, test.expected)
		}
	}
}

// func TestGeneratePassword
func TestGeneratePassword(t *testing.T) {
	type testCaseList struct {
		name     string
		n        int
		expected error
	}
	testCaseLists := []testCaseList{
		{"=0", 0, nil},
		{">0", 4, nil},
	}

	for _, test := range testCaseLists {
		output, err := GeneratePassword(test.n)
		_, err1 := base64.StdEncoding.DecodeString(output)

		if err != nil {
			t.Errorf("case (%v) output: (%v) is not the expected: (%v)", test.name, output, test.expected)
		}
		if err1 != nil {
			t.Errorf("case (%v) output: (%v) is not the expected: (%v)", test.name, output, test.expected)
		}

	}
}

func setEnvVars(http, https, no_proxy string) {
	os.Setenv("HTTP_PROXY", http)
	os.Setenv("HTTPS_PROXY", https)
	os.Setenv("NO_PROXY", no_proxy)
}

func TestProxyEnvVarsAreSet(t *testing.T) {
	type testCaseList struct {
		name     string
		http     string
		https    string
		no_proxy string
		expected bool
	}
	testCaseLists := []testCaseList{
		{"set http", "test", "", "", true},
		{"set https", "", "test", "", true},
		{"set no_proxy", "", "", "test", true},
		{"all null str", "", "", "", false},
	}

	for _, test := range testCaseLists {
		setEnvVars(test.http, test.https, test.no_proxy)
		output := ProxyEnvVarsAreSet()

		if output != test.expected {
			t.Errorf("case {%v), output: (%v) is not expected: (%v)", test.name, output, test.expected)
		}
	}
}

// Siddharth's code
func TestRemoveDuplicates(t *testing.T) {
	type testCaseList struct {
		name     string
		elements []string
		expected []string
	}
	testCaseLists := []testCaseList{
		{"One duplicate pair", []string{"a", "b", "a", "c", "d"}, []string{"a", "b", "c", "d"}},
		{"Two duplicate pairs", []string{"a", "b", "c", "d", "c", "a"}, []string{"a", "b", "c", "d"}},
		{"No duplicates", []string{"a", "b", "c", "d"}, []string{"a", "b", "c", "d"}},
	}

	for _, test := range testCaseLists {
		output := RemoveDuplicates(test.elements)

		for i, str := range output {
			if str != test.expected[i] {
				t.Errorf("case {%v), output: (%v) is not expected: (%v)", test.name, output, test.expected)
			}
		}
	}
}
