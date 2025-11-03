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

func validateGCS(conf Config) error {

	if conf.Bucket == "" {
		return errors.New("no bucket as gcs bucket name in config file")
	}

	// Validate bucket name length according to GCS specification
	// GCS bucket names must be between 3 and 63 characters long
	bucketLen := len(conf.Bucket)
	if bucketLen > 63 {
		return fmt.Errorf("bucket name '%s' is too long (%d characters). GCS bucket names must be 63 characters or less", conf.Bucket, bucketLen)
	}

	if bucketLen < 3 {
		return fmt.Errorf("bucket name '%s' is too short (%d characters). GCS bucket names must be at least 3 characters", conf.Bucket, bucketLen)
	}

	if conf.ServiceAccount == "" {
		return errors.New("no service_account as google application credentials in config file")
	}

	return nil
}

// IsValidGCSConf is used to validate GCS configuration.
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
