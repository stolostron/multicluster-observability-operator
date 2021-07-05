// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package config

import (
	"errors"
	"strings"

	"gopkg.in/yaml.v2"
)

func validateAzure(conf Config) error {

	if conf.StorageAccount == "" {
		return errors.New("no storage_account as azure storage account in config file")
	}

	if conf.StorageAccountKey == "" {
		return errors.New("no storage_account_key as azure storage account key in config file")
	}

	if conf.Container == "" {
		return errors.New("no container as azure container in config file")
	}

	if conf.Endpoint == "" {
		return errors.New("no endpoint as azure endpoint in config file")
	}

	return nil
}

// IsValidAzureConf is used to validate azure configuration
func IsValidAzureConf(data []byte) (bool, error) {
	var objectConfg ObjectStorgeConf
	err := yaml.Unmarshal(data, &objectConfg)
	if err != nil {
		return false, err
	}

	if strings.ToLower(objectConfg.Type) != "azure" {
		return false, errors.New("invalid type config, only azure type is supported")
	}

	err = validateAzure(objectConfg.Config)
	if err != nil {
		return false, err
	}

	return true, nil
}
