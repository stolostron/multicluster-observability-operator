// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package config

import (
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v2"
)

func validateS3(conf Config) error {
	if conf.Bucket == "" {
		return errors.New("no s3 bucket in config file")
	}

	// Validate bucket name length according to S3 specification
	// S3 bucket names must be between 3 and 63 characters long
	bucketLen := len(conf.Bucket)
	if bucketLen > 63 {
		return fmt.Errorf("bucket name '%s' is too long (%d characters). S3 bucket names must be 63 characters or less", conf.Bucket, bucketLen)
	}

	if bucketLen < 3 {
		return fmt.Errorf("bucket name '%s' is too short (%d characters). S3 bucket names must be at least 3 characters", conf.Bucket, bucketLen)
	}

	if conf.Endpoint == "" {
		return errors.New("no s3 endpoint in config file")
	}

	return nil
}

// IsValidS3Conf is used to validate s3 configuration.
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
