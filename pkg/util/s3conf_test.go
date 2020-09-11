// Copyright (c) 2020 Red Hat, Inc.

package util

import (
	"testing"
)

func TestParseConfig(t *testing.T) {
	valid := []byte(`bucket: abcd
insecure: false`)
	cfg, err := parseConfig(valid)

	if err != nil {
		t.Errorf("parsing of bucket failed: got %v, expected nil", err)
	}

	if cfg.Bucket != "abcd" {
		t.Errorf("parsing of bucket failed: got %v, expected %v", cfg.Bucket, "abcd")
	}

	if cfg.Insecure {
		t.Errorf("parsing of insecure failed: got %v, expected %v", cfg.Insecure, false)
	}

	invalid := []byte(`error
insecure: true`)
	cfg, err = parseConfig(invalid)

	if err == nil {
		t.Errorf("parsing of bucket failed: got %v, expected non-nil", err)
	}

	if cfg.Insecure {
		t.Errorf("parsing of insecure failed: got %v, expected %v", cfg.Insecure, false)
	}
}

func TestIsValidS3Conf(t *testing.T) {
	caseList := []struct {
		conf     []byte
		name     string
		expected bool
	}{
		{
			conf: []byte(`type: s3
config:
  bucket: bucket
  endpoint: endpoint
  insecure: true
  access_key: access_key
  secret_key: secret_key`),
			name:     "valid conf",
			expected: true,
		},

		{
			conf: []byte(`type: s3
config:
  bucket: ""
  endpoint: endpoint
  insecure: true
  access_key: access_key
  secret_key: secret_key`),
			name:     "no bucket",
			expected: false,
		},

		{
			conf: []byte(`type: s3
config:
  bucket: bucket
  endpoint: ""
  insecure: true
  access_key: access_key
  secret_key: secret_key`),
			name:     "no endpoint",
			expected: false,
		},

		{
			conf: []byte(`type: s3
config:
  bucket: bucket
  endpoint: endpoint
  insecure: true
  access_key: ""
  secret_key: secret_key`),
			name:     "no access_key",
			expected: false,
		},

		{
			conf: []byte(`type: s3
config:
  bucket: bucket
  endpoint: endpoint
  insecure: true
  access_key: access_key
  secret_key: ""`),
			name:     "no secret_key",
			expected: false,
		},

		{
			conf: []byte(`type: test
config:
  bucket: bucket
  endpoint: endpoint
  insecure: true
  access_key: access_key
  secret_key: ""`),
			name:     "invalid type",
			expected: false,
		},

		{
			conf: []byte(`
config:
  bucket: bucket
  endpoint: endpoint
  insecure: true
  access_key: access_key
  secret_key: secret_key`),
			name:     "invalid conf format",
			expected: false,
		},

		{
			conf:     []byte(``),
			name:     "no conf",
			expected: false,
		},
	}

	for _, c := range caseList {
		t.Run(c.name, func(t *testing.T) {
			output, _ := IsValidS3Conf(c.conf)
			if output != c.expected {
				t.Errorf("case (%v) output (%v) is not the expected (%v)", c.name, output, c.expected)
			}
		})
	}
}
