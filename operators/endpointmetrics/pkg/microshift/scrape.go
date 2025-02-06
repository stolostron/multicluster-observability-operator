// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package microshift

import (
	promcfg "github.com/prometheus/prometheus/config"
	"gopkg.in/yaml.v2"
)

type ScrapeConfigs struct {
	ScrapeConfigs []ScrapeConfig `yaml:",inline"`
}

type ScrapeConfig struct {
	promcfg.ScrapeConfig `yaml:",inline"`
	StaticConfigs        []StaticConfig `yaml:"static_configs,omitempty"`
}

type StaticConfig struct {
	Targets []string `yaml:"targets"`
}

func (sc *ScrapeConfigs) MarshalYAML() ([]byte, error) {
	ret, err := yaml.Marshal(sc.ScrapeConfigs)
	if err != nil {
		return nil, err
	}
	return ret, nil
}

func (sc *ScrapeConfigs) UnmarshalYAML(data []byte) error {
	if sc.ScrapeConfigs == nil {
		sc.ScrapeConfigs = []ScrapeConfig{}
	}
	return yaml.Unmarshal(data, &sc.ScrapeConfigs)
}
