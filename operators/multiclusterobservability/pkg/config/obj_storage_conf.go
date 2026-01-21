// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package config

import (
	"errors"
	"net/http"
	"strings"

	"github.com/prometheus/common/model"
	"sigs.k8s.io/yaml"
)

// Config is for s3/azure/gcs compatible configuration.
type Config struct {
	// s3 configuration
	Bucket     string     `json:"bucket"`
	Endpoint   string     `json:"endpoint"`
	Insecure   bool       `json:"insecure"`
	AccessKey  string     `json:"access_key"`
	SecretKey  string     `json:"secret_key"`
	HTTPConfig HTTPConfig `json:"http_config"`
	// azure configuration
	// Bucket    string `json:"bucket"`
	StorageAccount    string `json:"storage_account"`
	StorageAccountKey string `json:"storage_account_key"`
	Container         string `json:"container"`
	MaxRetries        int32  `json:"max_retries"`

	// gcs configuration
	// Endpoint  string `json:"endpoint"`
	ServiceAccount string `json:"service_account"`
}

// HTTPConfig stores the http.Transport configuration for the s3 minio client.
type HTTPConfig struct {
	IdleConnTimeout       model.Duration `json:"idle_conn_timeout"`
	ResponseHeaderTimeout model.Duration `json:"response_header_timeout"`
	InsecureSkipVerify    bool           `json:"insecure_skip_verify"`

	TLSHandshakeTimeout   model.Duration `json:"tls_handshake_timeout"`
	ExpectContinueTimeout model.Duration `json:"expect_continue_timeout"`
	MaxIdleConns          int            `json:"max_idle_conns"`
	MaxIdleConnsPerHost   int            `json:"max_idle_conns_per_host"`
	MaxConnsPerHost       int            `json:"max_conns_per_host"`

	// Allow upstream callers to inject a round tripper
	Transport http.RoundTripper `json:"-"`

	TLSConfig TLSConfig `json:"tls_config"`
}

// TLSConfig configures the options for TLS connections.
type TLSConfig struct {
	// The CA cert to use for the targets.
	CAFile string `json:"ca_file"`
	// The client cert file for the targets.
	CertFile string `json:"cert_file"`
	// The client key file for the targets.
	KeyFile string `json:"key_file"`
	// Used to verify the hostname for the targets.
	ServerName string `json:"server_name"`
	// Disable target certificate validation.
	InsecureSkipVerify bool `json:"insecure_skip_verify"`
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
