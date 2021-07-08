// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package config

import (
	"errors"
	"strings"

	"gopkg.in/yaml.v2"
)

func validateS3(conf Config) error {

	if conf.Bucket == "" {
		return errors.New("no s3 bucket in config file")
	}

	if conf.Endpoint == "" {
		return errors.New("no s3 endpoint in config file")
	}

	if conf.AccessKey == "" {
		return errors.New("no s3 access_key in config file")
	}

	if conf.SecretKey == "" {
		return errors.New("no s3 secret_key in config file")
	}

	return nil
}

// IsValidS3Conf is used to validate s3 configuration
func IsValidS3Conf(data []byte) (bool, error) {
	var objectConfg ObjectStorgeConf
	err := yaml.Unmarshal(data, &objectConfg)
	if err != nil {
		return false, err
	}

	if strings.ToLower(objectConfg.Type) != "s3" {
		return false, errors.New("invalid type config, only s3 type is supported")
	}

	err = validateS3(objectConfg.Config)
	if err != nil {
		return false, err
	}

	return true, nil
}
