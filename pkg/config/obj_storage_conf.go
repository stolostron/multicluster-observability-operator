// Copyright (c) 2020 Red Hat, Inc.

package config

import (
	"errors"
	"strings"

	"gopkg.in/yaml.v2"
)

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
		return true, nil

	case "azure":
		return true, nil

	default:
		return false, errors.New("invalid object storage type config")
	}

	return true, nil
}
