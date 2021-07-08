// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package config

import (
	"errors"
	"strings"

	"gopkg.in/yaml.v2"
)

// Config is for s3/azure/gcs compatiable configuration
type Config struct {
	// s3 configuration
	Bucket    string `yaml:"bucket"`
	Endpoint  string `yaml:"endpoint"`
	Insecure  bool   `yaml:"insecure"`
	AccessKey string `yaml:"access_key"`
	SecretKey string `yaml:"secret_key"`

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

// CheckObjStorageConf is used to check/valid the object storage configurations
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
