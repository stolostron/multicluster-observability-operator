package util

import (
	"encoding/base64"
	"os"
	"testing"
)

func TestRemove(t *testing.T) {
	type testCaseList struct {
		name     string
		list     []string
		s        string
		expected []string
	}

	var testCaseLists = []testCaseList{
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

func TestContains(t *testing.T) {
	testCaseList := []struct {
		name     string
		list     []string
		s        string
		expected bool
	}{
		{"contain sub string", []string{"a", "b"}, "a", true},
		{"shoud contain empty string", []string{""}, "", true},
		{"should not contain sub string", []string{"a", "b"}, "c", false},
		{"shoud not contain empty string", []string{"a", "b"}, "", false},
	}

	for _, c := range testCaseList {
		output := Contains(c.list, c.s)
		if output != c.expected {
			t.Errorf("case (%v) output: (%v) is not the expected: (%v)", c.name, output, c.expected)
		}
	}
}

func TestGetAnnotation(t *testing.T) {
	type testCaseList struct {
		name       string
		annotation map[string]string
		key        string
		expected   string
	}
	//tests whether func returns correct value for a given key
	fullMap := map[string]string{
		"a": "aTest",
		"b": "bTest",
		"c": "cTest",
	}

	//tests whether func returns nil for a nil map
	var nilMap map[string]string

	var testCaseLists = []testCaseList{
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

//TestGeneratePassword
func TestGeneratePassword(t *testing.T) {
	type testCaseList struct {
		name     string
		n        int
		expected error //for both encoding and decoding
	}

	var testCaseLists = []testCaseList{
		{"Length > 0", 4, nil},
		{"Length = 0", 0, nil},
	}

	for _, test := range testCaseLists {
		output, err := GeneratePassword(test.n)

		if err != nil {
			t.Errorf("case (%v) output: (%v) is not the expected: (%v)", test.name, err, test.expected)
		}
		_, err1 := base64.StdEncoding.DecodeString(output)
		if err1 != nil {
			t.Errorf("case (%v) output: (%v) is not the expected: (%v)", test.name, err, test.expected)
		}
	}

}

func setEnvVars(http, https, none string) string {
	os.Setenv("HTTP_PROXY", http)
	os.Setenv("HTTPS_PROXY", https)
	os.Setenv("NO_PROXY", none)

	return ""
}
func TestProxyEnvVarsAreSet(t *testing.T) {
	type testCaseList struct {
		name     string
		http     string
		https    string
		no_proxy string
		expected bool
	}
	var testCaseLists = []testCaseList{
		{"All null str", "", "", "", false},
		{"defined http", "test", "", "", true},
		{"defined https", "", "test", "", true},
		{"defined no_proxy", "", "", "test", true},
		{"All defined", "test", "test", "test", true},
	}

	for _, test := range testCaseLists {
		os.Setenv("HTTP_PROXY", test.http)
		os.Setenv("HTTPS_PROXY", test.https)
		os.Setenv("NO_PROXY", test.no_proxy)

		output := ProxyEnvVarsAreSet()

		if output != test.expected {
			t.Errorf("case {%v), output: (%v) is not expected: (%v)", test.name, output, test.expected)
		}
	}
}

func TestRemoveDuplicates(t *testing.T) {
	type testCaseList struct {
		name     string
		elements []string
		expected []string
	}
	var testCaseLists = []testCaseList{
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
