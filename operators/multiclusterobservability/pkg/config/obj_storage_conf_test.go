// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

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
