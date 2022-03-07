package util

import (
	"fmt"
	"path"
	"testing"
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

	if newConfig.BasicAuth.PasswordFile != fmt.Sprintf(path.Join(MountPath, basicAuthSName, basicAuthSKey)) {
		t.Fatalf("Wrong path for BasicAuth.PasswordFile: %s", newConfig.BasicAuth.PasswordFile)
	}

	if newConfig.Authorization.CredentialsFile != fmt.Sprintf(path.Join(MountPath, AuthSName, AuthSKey)) {
		t.Fatalf("Wrong path for Authorization.CredentialsFile: %s", newConfig.Authorization.CredentialsFile)
	}

	if newConfig.OAuth2.ClientSecretFile != fmt.Sprintf(path.Join(MountPath, OAuth2SName, OAuth2SKey)) {
		t.Fatalf("Wrong path for OAuth2.ClientSecretFile: %s", newConfig.OAuth2.ClientSecretFile)
	}

	if newConfig.BearerTokenFile != fmt.Sprintf(path.Join(MountPath, BearerTokenSName, BearerTokenSKey)) {
		t.Fatalf("Wrong path for BearerTokenFile: %s", newConfig.BearerTokenFile)
	}

	if newConfig.TLSConfig.CAFile != fmt.Sprintf(path.Join(MountPath, TLSSName, TLSCAKey)) {
		t.Fatalf("Wrong path for TLSConfig.CAFile: %s", newConfig.TLSConfig.CAFile)
	}

	if newConfig.TLSConfig.CertFile != fmt.Sprintf(path.Join(MountPath, TLSSName, TLSCertKey)) {
		t.Fatalf("Wrong path for TLSConfig.CertFile: %s", newConfig.TLSConfig.CertFile)
	}

	if newConfig.TLSConfig.KeyFile != fmt.Sprintf(path.Join(MountPath, TLSSName, TLSKeyKey)) {
		t.Fatalf("Wrong path for TLSConfig.KeyFile: %s", newConfig.TLSConfig.KeyFile)
	}

	if len(names) != 5 {
		t.Fatalf("Wrong number of mount secrets: expect 5, get %d", len(names))
	}
}
