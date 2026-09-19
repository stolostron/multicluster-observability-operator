// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package config

import (
	"testing"
)

func TestCheckObjStorageConf(t *testing.T) {
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
			name:     "valid s3 conf",
			expected: true,
		},

		{
			conf: []byte(`type: azure
config:
  storage_account: storage_account
  storage_account_key: storage_account_key
  container: container
  endpoint: endpoint
  max_retries: 0`),
			name:     "valid azure conf",
			expected: true,
		},

		{
			conf: []byte(`type: gcs
config:
  bucket: bucket
  service_account: service_account`),
			name:     "valid gcs conf",
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
			conf: []byte(`type: gcs
config:
  bucket: ""
  service_account: service_account`),
			name:     "no bucket",
			expected: false,
		},

		{
			conf: []byte(`type: gcs
config:
  bucket: bucket
  service_account: ""`),
			name:     "no service_account",
			expected: false,
		},

		{
			conf: []byte(`type: azure
config:
  storage_account: ""
  storage_account_key: storage_account_key
  container: container
  endpoint: endpoint
  max_retries: 0`),
			name:     "no storage_account",
			expected: false,
		},

		{
			conf: []byte(`type: azure
config:
  storage_account: storage_account
  storage_account_key: ""
  container: container
  endpoint: endpoint
  max_retries: 0`),
			name:     "no storage_account_key",
			expected: false,
		},

		{
			conf: []byte(`type: azure
config:
  storage_account: storage_account
  storage_account_key: storage_account_key
  container: ""
  endpoint: endpoint
  max_retries: 0`),
			name:     "no container",
			expected: false,
		},

		{
			conf: []byte(`type: azure
config:
  storage_account: storage_account
  storage_account_key: storage_account_key
  container: container
  endpoint: ""
  max_retries: 0`),
			name:     "no endpoint",
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

		// Bucket name length validation tests
		{
			conf: []byte(`type: s3
config:
  bucket: thanos-s3-open-cluster-management-observability-ran-samsung-bos2-lab-a1b2c3d4
  endpoint: endpoint
  insecure: true
  access_key: access_key
  secret_key: secret_key`),
			name:     "s3 bucket name too long (80 chars)",
			expected: false,
		},

		{
			conf: []byte(`type: s3
config:
  bucket: a1234567890123456789012345678901234567890123456789012345678901234
  endpoint: endpoint
  insecure: true
  access_key: access_key
  secret_key: secret_key`),
			name:     "s3 bucket name too long (64 chars)",
			expected: false,
		},

		{
			conf: []byte(`type: s3
config:
  bucket: a12345678901234567890123456789012345678901234567890123456789012
  endpoint: endpoint
  insecure: true
  access_key: access_key
  secret_key: secret_key`),
			name:     "s3 bucket name at limit (63 chars)",
			expected: true,
		},

		{
			conf: []byte(`type: s3
config:
  bucket: ab
  endpoint: endpoint
  insecure: true
  access_key: access_key
  secret_key: secret_key`),
			name:     "s3 bucket name too short (2 chars)",
			expected: false,
		},

		{
			conf: []byte(`type: s3
config:
  bucket: abc
  endpoint: endpoint
  insecure: true
  access_key: access_key
  secret_key: secret_key`),
			name:     "s3 bucket name minimum length (3 chars)",
			expected: true,
		},

		{
			conf: []byte(`type: gcs
config:
  bucket: thanos-s3-open-cluster-management-observability-ran-samsung-bos2-lab-a1b2c3d4
  service_account: service_account`),
			name:     "gcs bucket name too long (80 chars)",
			expected: false,
		},

		{
			conf: []byte(`type: gcs
config:
  bucket: a12345678901234567890123456789012345678901234567890123456789012
  service_account: service_account`),
			name:     "gcs bucket name at limit (63 chars)",
			expected: true,
		},

		{
			conf: []byte(`type: gcs
config:
  bucket: ab
  service_account: service_account`),
			name:     "gcs bucket name too short (2 chars)",
			expected: false,
		},

		{
			conf: []byte(`type: gcs
config:
  bucket: abc
  service_account: service_account`),
			name:     "gcs bucket name minimum length (3 chars)",
			expected: true,
		},
	}

	for _, c := range caseList {
		t.Run(c.name, func(t *testing.T) {
			output, _ := CheckObjStorageConf(c.conf)
			if output != c.expected {
				t.Errorf("case (%v) output (%v) is not the expected (%v)", c.name, output, c.expected)
			}
		})
	}
}
