// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package config

import (
	"net/http"
	"strings"

	"errors"

	"github.com/prometheus/common/model"
	"gopkg.in/yaml.v2"
)

// Config is for s3/azure/gcs compatible configuration.
type Config struct {
	// s3 configuration
	Bucket     string     `yaml:"bucket"`
	Endpoint   string     `yaml:"endpoint"`
	Insecure   bool       `yaml:"insecure"`
	AccessKey  string     `yaml:"access_key"`
	SecretKey  string     `yaml:"secret_key"`
	HTTPConfig HTTPConfig `yaml:"http_config"`
	// azure configuration
	// Bucket    string `yaml:"bucket"`
	StorageAccount    string `yaml:"storage_account"`
	StorageAccountKey string `yaml:"storage_account_key"`
	Container         string `yaml:"container"`
	MaxRetries        int32  `yaml:"max_retries"`

	// gcs configuration
	// Endpoint  string `yaml:"endpoint"`
	ServiceAccount string `yaml:"service_account"`
}

// HTTPConfig stores the http.Transport configuration for the s3 minio client.
type HTTPConfig struct {
	IdleConnTimeout       model.Duration `yaml:"idle_conn_timeout"`
	ResponseHeaderTimeout model.Duration `yaml:"response_header_timeout"`
	InsecureSkipVerify    bool           `yaml:"insecure_skip_verify"`

	TLSHandshakeTimeout   model.Duration `yaml:"tls_handshake_timeout"`
	ExpectContinueTimeout model.Duration `yaml:"expect_continue_timeout"`
	MaxIdleConns          int            `yaml:"max_idle_conns"`
	MaxIdleConnsPerHost   int            `yaml:"max_idle_conns_per_host"`
	MaxConnsPerHost       int            `yaml:"max_conns_per_host"`

	// Allow upstream callers to inject a round tripper
	Transport http.RoundTripper `yaml:"-"`

	TLSConfig TLSConfig `yaml:"tls_config"`
}

// TLSConfig configures the options for TLS connections.
type TLSConfig struct {
	// The CA cert to use for the targets.
	CAFile string `yaml:"ca_file"`
	// The client cert file for the targets.
	CertFile string `yaml:"cert_file"`
	// The client key file for the targets.
	KeyFile string `yaml:"key_file"`
	// Used to verify the hostname for the targets.
	ServerName string `yaml:"server_name"`
	// Disable target certificate validation.
	InsecureSkipVerify bool `yaml:"insecure_skip_verify"`
}

// CheckObjStorageConf is used to check/valid the object storage configurations.
func CheckObjStorageConf(data []byte) (bool, error) {
	var objectConfg ObjectStorgeConf
	err := yaml.Unmarshal(data, &objectConfg)
	if err != nil {
		return false, err
	}

	switch strings.ToLower(objectConfg.Type) {
	case "s3":
		return IsValidS3Conf(data)

	case "gcs":
		return IsValidGCSConf(data)

	case "azure":
		return IsValidAzureConf(data)

	default:
		return false, errors.New("invalid object storage type config")
	}
}
