// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package util

import (
	"fmt"
	"net/url"
	"path"

	"github.com/prometheus/common/config"
)

const MountPath = "/var/run/secrets/"

type TLSConfigWithSecret struct {
	// Name of the secret which contains the file
	SecretName string `yaml:"secret_name,omitempty" json:"secret_name,omitempty"`
	// The CA cert to use for the targets.
	CAFile    string `yaml:"ca_file,omitempty"     json:"ca_file,omitempty"`
	CAFileKey string `yaml:"ca_file_key,omitempty" json:"ca_file_key,omitempty"`
	// The client cert file for the targets.
	CertFile    string `yaml:"cert_file,omitempty"     json:"cert_file,omitempty"`
	CertFileKey string `yaml:"cert_file_key,omitempty" json:"cert_file_key,omitempty"`
	// The client key file for the targets.
	KeyFile    string `yaml:"key_file,omitempty"     json:"key_file,omitempty"`
	KeyFileKey string `yaml:"key_file_key,omitempty" json:"key_file_key,omitempty"`
	// Used to verify the hostname for the targets.
	ServerName string `yaml:"server_name,omitempty" json:"server_name,omitempty"`
	// Disable target certificate validation.
	InsecureSkipVerify bool `yaml:"insecure_skip_verify" json:"insecure_skip_verify"`
}

type OAuth2WithSecret struct {
	ClientID         string `yaml:"client_id"          json:"client_id"`
	ClientSecret     string `yaml:"client_secret"      json:"client_secret"`
	ClientSecretFile string `yaml:"client_secret_file" json:"client_secret_file"`
	// Name of the secret which contains the file
	SecretName          string            `yaml:"secret_name,omitempty"     json:"secret_name,omitempty"`
	ClientSecretFileKey string            `yaml:"client_secret_file_key"    json:"client_secret_file_key"`
	Scopes              []string          `yaml:"scopes,omitempty"          json:"scopes,omitempty"`
	TokenURL            string            `yaml:"token_url"                 json:"token_url"`
	EndpointParams      map[string]string `yaml:"endpoint_params,omitempty" json:"endpoint_params,omitempty"`

	// TLSConfig is used to connect to the token URL.
	TLSConfig TLSConfigWithSecret `yaml:"tls_config,omitempty"`
}

type BasicAuthWithSecret struct {
	Username     string `yaml:"username"                json:"username"`
	Password     string `yaml:"password,omitempty"      json:"password,omitempty"`
	PasswordFile string `yaml:"password_file,omitempty" json:"password_file,omitempty"`
	// Name of the secret which contains the file
	SecretName      string `yaml:"secret_name,omitempty"       json:"secret_name,omitempty"`
	PasswordFileKey string `yaml:"password_file_key,omitempty" json:"password_file_key,omitempty"`
}

type AuthorizationWithSecret struct {
	Type            string `yaml:"type,omitempty"             json:"type,omitempty"`
	Credentials     string `yaml:"credentials,omitempty"      json:"credentials,omitempty"`
	CredentialsFile string `yaml:"credentials_file,omitempty" json:"credentials_file,omitempty"`
	// Name of the secret which contains the file
	SecretName         string `yaml:"secret_name,omitempty"          json:"secret_name,omitempty"`
	CredentialsFileKey string `yaml:"credentials_file_key,omitempty" json:"credentials_file_key,omitempty"`
}

type HTTPClientConfigWithSecret struct {
	// The HTTP basic authentication credentials for the targets.
	BasicAuth *BasicAuthWithSecret `yaml:"basic_auth,omitempty" json:"basic_auth,omitempty"`
	// The HTTP authorization credentials for the targets.
	Authorization *AuthorizationWithSecret `yaml:"authorization,omitempty" json:"authorization,omitempty"`
	// The OAuth2 client credentials used to fetch a token for the targets.
	OAuth2 *OAuth2WithSecret `yaml:"oauth2,omitempty" json:"oauth2,omitempty"`
	// The bearer token for the targets. Deprecated in favor of
	// Authorization.Credentials.
	BearerToken string `yaml:"bearer_token,omitempty" json:"bearer_token,omitempty"`
	// The bearer token file for the targets. Deprecated in favor of
	// Authorization.CredentialsFile.
	BearerTokenFile string `yaml:"bearer_token_file,omitempty" json:"bearer_token_file,omitempty"`
	// Name of the secret which contains the file
	SecretName         string `yaml:"secret_name,omitempty"           json:"secret_name,omitempty"`
	BearerTokenFileKey string `yaml:"bearer_token_file_key,omitempty" json:"bearer_token_file_key,omitempty"`
	// HTTP proxy server to use to connect to the targets.
	ProxyURL *config.URL `yaml:"proxy_url,omitempty" json:"proxy_url,omitempty"`
	// TLSConfig to use to connect to the targets.
	TLSConfig *TLSConfigWithSecret `yaml:"tls_config,omitempty" json:"tls_config,omitempty"`
	// FollowRedirects specifies whether the client should follow HTTP 3xx redirects.
	// The omitempty flag is not set, because it would be hidden from the
	// marshaled configuration when set to false.
	FollowRedirects bool `yaml:"follow_redirects" json:"follow_redirects"`
}

type RemoteWriteEndpointWithSecret struct {
	Name             string                      `yaml:"name"                         json:"name"`
	URL              config.URL                  `yaml:"url"                          json:"url"`
	HttpClientConfig *HTTPClientConfigWithSecret `yaml:"http_client_config,omitempty" json:"http_client_config,omitempty"`
}

// Validate validates the remote write endpoint
func (res *RemoteWriteEndpointWithSecret) Validate() error {
	if res.URL.String() == "" {
		return fmt.Errorf("url is required for remote write endpoint %s", res.Name)
	}

	u, err := url.ParseRequestURI(res.URL.String())
	if err != nil {
		return fmt.Errorf("url %s is invalid for remote write endpoint %s: %s", res.URL.String(), res.Name, err)
	}

	if u.Scheme == "" || u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("url %s is invalid for remote write endpoint %s: scheme must be http or https", res.URL.String(), res.Name)
	}

	return nil
}

func getMountPath(secretName, key string) string {
	return path.Join(MountPath, secretName, key)
}

func transformBasicAuth(old BasicAuthWithSecret) *BasicAuthWithSecret {
	basicAuth := &BasicAuthWithSecret{
		Username: old.Username,
	}
	if old.Password != "" {
		basicAuth.Password = old.Password
	}
	if old.SecretName != "" {
		basicAuth.PasswordFile = getMountPath(old.SecretName, old.PasswordFileKey)
	}
	return basicAuth
}

func transformTLSConfig(old TLSConfigWithSecret) *TLSConfigWithSecret {
	tlsConfig := &TLSConfigWithSecret{
		InsecureSkipVerify: old.InsecureSkipVerify,
	}
	if old.SecretName != "" {
		tlsConfig.ServerName = old.ServerName
	}
	if old.SecretName != "" {
		if old.CAFileKey != "" {
			tlsConfig.CAFile = getMountPath(old.SecretName, old.CAFileKey)
		}
		if old.CertFileKey != "" {
			tlsConfig.CertFile = getMountPath(old.SecretName, old.CertFileKey)
		}
		if old.KeyFileKey != "" {
			tlsConfig.KeyFile = getMountPath(old.SecretName, old.KeyFileKey)
		}
	}
	return tlsConfig
}

func transformAuthorization(old AuthorizationWithSecret) *AuthorizationWithSecret {
	auth := &AuthorizationWithSecret{}
	if old.Type != "" {
		auth.Type = old.Type
	}
	if old.Credentials != "" {
		auth.Credentials = old.Credentials
	}
	if old.SecretName != "" {
		auth.CredentialsFile = getMountPath(old.SecretName, old.CredentialsFileKey)
	}
	return auth
}

func transformOAuth2(old OAuth2WithSecret) *OAuth2WithSecret {
	oauth2 := &OAuth2WithSecret{
		ClientID:         old.ClientID,
		ClientSecret:     old.ClientSecret,
		ClientSecretFile: old.ClientSecretFileKey,
		TokenURL:         old.TokenURL,
	}
	if old.Scopes != nil {
		oauth2.Scopes = old.Scopes
	}
	if old.EndpointParams != nil {
		oauth2.EndpointParams = old.EndpointParams
	}
	if old.SecretName != "" {
		oauth2.ClientSecretFile = getMountPath(old.SecretName, old.ClientSecretFileKey)
	}

	return oauth2
}

func Transform(oldClientConfig HTTPClientConfigWithSecret) (*HTTPClientConfigWithSecret, []string) {
	sNames := []string{}
	clientConfig := &HTTPClientConfigWithSecret{
		FollowRedirects: oldClientConfig.FollowRedirects,
	}
	if oldClientConfig.BearerToken != "" {
		clientConfig.BearerToken = oldClientConfig.BearerToken
	}
	if oldClientConfig.SecretName != "" {
		clientConfig.BearerTokenFile = getMountPath(oldClientConfig.SecretName, oldClientConfig.BearerTokenFileKey)
		sNames = append(sNames, oldClientConfig.SecretName)
	}
	if oldClientConfig.ProxyURL != nil {
		clientConfig.ProxyURL = oldClientConfig.ProxyURL
	}
	if oldClientConfig.BasicAuth != nil {
		clientConfig.BasicAuth = transformBasicAuth(*oldClientConfig.BasicAuth)
		if oldClientConfig.BasicAuth.SecretName != "" {
			sNames = append(sNames, oldClientConfig.BasicAuth.SecretName)
		}
	}
	if oldClientConfig.TLSConfig != nil && oldClientConfig.TLSConfig.SecretName != "" {
		clientConfig.TLSConfig = transformTLSConfig(*oldClientConfig.TLSConfig)
		if oldClientConfig.TLSConfig.SecretName != "" {
			sNames = append(sNames, oldClientConfig.TLSConfig.SecretName)
		}
	}
	if oldClientConfig.Authorization != nil {
		clientConfig.Authorization = transformAuthorization(*oldClientConfig.Authorization)
		if oldClientConfig.Authorization.SecretName != "" {
			sNames = append(sNames, oldClientConfig.Authorization.SecretName)
		}
	}
	if oldClientConfig.OAuth2 != nil {
		clientConfig.OAuth2 = transformOAuth2(*oldClientConfig.OAuth2)
		if oldClientConfig.OAuth2.SecretName != "" {
			sNames = append(sNames, oldClientConfig.OAuth2.SecretName)
		}
	}
	return clientConfig, sNames
}
