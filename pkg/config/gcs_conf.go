// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package config

import (
	"errors"
	"strings"

	"gopkg.in/yaml.v2"
)

func validateGCS(conf Config) error {

	if conf.Bucket == "" {
		return errors.New("no bucket as gcs bucket name in config file")
	}

	if conf.ServiceAccount == "" {
		return errors.New("no service_account as google application credentials in config file")
	}

	return nil
}

// IsValidGCSConf is used to validate GCS configuration
func IsValidGCSConf(data []byte) (bool, error) {
	var objectConfg ObjectStorgeConf
	err := yaml.Unmarshal(data, &objectConfg)
	if err != nil {
		return false, err
	}

	if strings.ToLower(objectConfg.Type) != "gcs" {
		return false, errors.New("invalid type config, only GCS type is supported")
	}

	err = validateGCS(objectConfg.Config)
	if err != nil {
		return false, err
	}

	return true, nil
}
