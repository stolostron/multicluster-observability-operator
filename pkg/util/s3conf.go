// Copyright (c) 2020 Red Hat, Inc.

package util

import (
	"errors"

	"gopkg.in/yaml.v2"
)

type Config struct {
	Bucket    string `yaml:"bucket"`
	Endpoint  string `yaml:"endpoint"`
	Insecure  bool   `yaml:"insecure"`
	AccessKey string `yaml:"access_key"`
	SecretKey string `yaml:"secret_key"`
}

// parseConfig unmarshals a buffer into a Config with default HTTPConfig values.
func parseConfig(conf []byte) (Config, error) {
	var config Config
	if err := yaml.UnmarshalStrict(conf, &config); err != nil {
		return Config{}, err
	}

	return config, nil
}

func validate(conf Config) error {

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

func IsValidS3Conf(conf []byte) (bool, error) {
	var confStr interface{}
	err := yaml.UnmarshalStrict(conf, &confStr)
	if err != nil {
		return false, err
	}

	_, ok := confStr.(map[interface{}]interface{})
	if !ok {
		return false, errors.New("invalid config format")
	}

	objType, ok := confStr.(map[interface{}]interface{})["type"].(string)

	if !ok {
		return false, errors.New("invalid config format")
	}

	if objType != "s3" {
		return false, errors.New("invalid type config, only s3 type is supported")
	}

	yamlConf := confStr.(map[interface{}]interface{})["config"]
	rawConf, err := yaml.Marshal(&yamlConf)
	if err != nil {
		return false, err
	}

	s3Conf, err := parseConfig(rawConf)
	if err != nil {
		return false, err
	}

	err = validate(s3Conf)
	if err != nil {
		return false, err
	}

	return true, nil
}
