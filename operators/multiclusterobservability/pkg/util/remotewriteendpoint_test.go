// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package util

import (
	"net/url"
	"path"
	"testing"

	"github.com/prometheus/common/config"
)

const (
	basicAuthSName   = "basic_secret"
	basicAuthSKey    = "basic_key"
	AuthSName        = "auth_secret"
	AuthSKey         = "auth_key"
	OAuth2SName      = "oauth2_secret"
	OAuth2SKey       = "oauth2_key"
	BearerTokenSName = "bearertoken_secret"
	BearerTokenSKey  = "bearertoken_key"
	TLSSName         = "tls_secret"
	TLSCAKey         = "tls_ca_key"
	TLSCertKey       = "tls_cert_key"
	TLSKeyKey        = "tls_key_key"
)

func TestTransform(t *testing.T) {
	config := &HTTPClientConfigWithSecret{
		BasicAuth: &BasicAuthWithSecret{
			Username:        "user",
			Password:        "pwd",
			SecretName:      basicAuthSName,
			PasswordFileKey: basicAuthSKey,
		},
		Authorization: &AuthorizationWithSecret{
			SecretName:         AuthSName,
			CredentialsFileKey: AuthSKey,
		},
		OAuth2: &OAuth2WithSecret{
			ClientID:            "client_id",
			SecretName:          OAuth2SName,
			ClientSecretFileKey: OAuth2SKey,
		},
		SecretName:         BearerTokenSName,
		BearerTokenFileKey: BearerTokenSKey,
		TLSConfig: &TLSConfigWithSecret{
			SecretName:  TLSSName,
			CAFileKey:   TLSCAKey,
			CertFileKey: TLSCertKey,
			KeyFileKey:  TLSKeyKey,
		},
	}
	newConfig, names := Transform(*config)

	if newConfig.BasicAuth.PasswordFile != path.Join(MountPath, basicAuthSName, basicAuthSKey) {
		t.Fatalf("Wrong path for BasicAuth.PasswordFile: %s", newConfig.BasicAuth.PasswordFile)
	}

	if newConfig.Authorization.CredentialsFile != path.Join(MountPath, AuthSName, AuthSKey) {
		t.Fatalf("Wrong path for Authorization.CredentialsFile: %s", newConfig.Authorization.CredentialsFile)
	}

	if newConfig.OAuth2.ClientSecretFile != path.Join(MountPath, OAuth2SName, OAuth2SKey) {
		t.Fatalf("Wrong path for OAuth2.ClientSecretFile: %s", newConfig.OAuth2.ClientSecretFile)
	}

	if newConfig.BearerTokenFile != path.Join(MountPath, BearerTokenSName, BearerTokenSKey) {
		t.Fatalf("Wrong path for BearerTokenFile: %s", newConfig.BearerTokenFile)
	}

	if newConfig.TLSConfig.CAFile != path.Join(MountPath, TLSSName, TLSCAKey) {
		t.Fatalf("Wrong path for TLSConfig.CAFile: %s", newConfig.TLSConfig.CAFile)
	}

	if newConfig.TLSConfig.CertFile != path.Join(MountPath, TLSSName, TLSCertKey) {
		t.Fatalf("Wrong path for TLSConfig.CertFile: %s", newConfig.TLSConfig.CertFile)
	}

	if newConfig.TLSConfig.KeyFile != path.Join(MountPath, TLSSName, TLSKeyKey) {
		t.Fatalf("Wrong path for TLSConfig.KeyFile: %s", newConfig.TLSConfig.KeyFile)
	}

	if len(names) != 5 {
		t.Fatalf("Wrong number of mount secrets: expect 5, get %d", len(names))
	}
}

func TestValidateRemoteWriteEndpointWithSecret(t *testing.T) {
	testCases := []struct {
		name     string
		endpoint *RemoteWriteEndpointWithSecret
		wantErr  bool
	}{
		{
			name:     "test missing url",
			endpoint: &RemoteWriteEndpointWithSecret{Name: "valid-name", URL: mustParseURL(t, "")},
			wantErr:  true,
		},
		{
			name:     "test invalid url no scheme",
			endpoint: &RemoteWriteEndpointWithSecret{Name: "valid-name", URL: mustParseURL(t, "invalid-url")},
			wantErr:  true,
		},
		{
			name:     "test valid url invalid scheme",
			endpoint: &RemoteWriteEndpointWithSecret{Name: "valid-name", URL: mustParseURL(t, "httttp://some-valid-host.com:8080/prometheus/api/v1/write")},
			wantErr:  true,
		},
		{
			name:     "test happy path",
			endpoint: &RemoteWriteEndpointWithSecret{Name: "valid-name", URL: mustParseURL(t, "https://example.com")},
			wantErr:  false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.endpoint.Validate()
			if (err != nil) != tc.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tc.wantErr)
				return
			}
		})
	}
}

func mustParseURL(t *testing.T, s string) config.URL {
	u, err := url.Parse(s)
	if err != nil {
		t.Fatal(err.Error())
	}

	return config.URL{URL: u}
}
