// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

import (
	"testing"
)

func TestValidateClusterServerURL(t *testing.T) {
	tests := []struct {
		name    string
		rawURL  string
		wantErr bool
	}{
		{
			name:    "valid standard openshift https URL with port 6443",
			rawURL:  "https://api.ci-rosa-50-fi.498a.p1.openshiftapps.com:6443",
			wantErr: false,
		},
		{
			name:    "valid rosa hcp https URL with port 443",
			rawURL:  "https://api.ci-rosa-50-fi.498a.p1.openshiftapps.com:443",
			wantErr: false,
		},
		{
			name:    "valid https URL without explicit port",
			rawURL:  "https://api.ci-rosa-50-fi.498a.p1.openshiftapps.com",
			wantErr: false,
		},
		{
			name:    "valid http URL for local testing",
			rawURL:  "http://localhost:8080",
			wantErr: false,
		},
		{
			name:    "valid IP address server URL",
			rawURL:  "https://127.0.0.1:6443",
			wantErr: false,
		},
		{
			name:    "empty URL",
			rawURL:  "",
			wantErr: true,
		},
		{
			name:    "missing scheme",
			rawURL:  "api.ci-rosa-50-fi.498a.p1.openshiftapps.com:6443",
			wantErr: true,
		},
		{
			name:    "unsupported scheme ftp",
			rawURL:  "ftp://api.ci-rosa-50-fi.498a.p1.openshiftapps.com:6443",
			wantErr: true,
		},
		{
			name:    "missing host",
			rawURL:  "https://:6443",
			wantErr: true,
		},
		{
			name:    "invalid port out of range",
			rawURL:  "https://api.ci-rosa-50-fi.498a.p1.openshiftapps.com:99999",
			wantErr: true,
		},
		{
			name:    "invalid non-numeric port",
			rawURL:  "https://api.ci-rosa-50-fi.498a.p1.openshiftapps.com:port",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateClusterServerURL(tt.rawURL)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateClusterServerURL(%q) error = %v, wantErr %v", tt.rawURL, err, tt.wantErr)
			}
		})
	}
}
