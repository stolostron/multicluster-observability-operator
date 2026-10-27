// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package estimator

import (
	"fmt"
	"os"
	"slices"
	"time"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/stolostron/multicluster-observability-operator/estimators/cardinality/pkg/hll"
	"gopkg.in/yaml.v2"
)

const (
	minInterval     = time.Minute
	maxGroupBy      = 5
	defaultBuckets  = 15
	defaultLimit    = 10000
	defaultMaxItems = 30000000
	labelKeyword    = "__label__"
	groupKeyword    = "__group__"
	globalKeyword   = "__global__"
)

// Config represents the top-level YAML configuration for the cardinality estimator.
type Config struct {
	MaxItemsPerInterval int            `yaml:"max_items_per_interval"`
	Streams             []StreamConfig `yaml:"streams"`
}

// StreamConfig configures a single sliding-window cardinality estimation stream.
type StreamConfig struct {
	Interval       time.Duration `yaml:"interval"`
	Filter         string        `yaml:"filter"`
	GroupBy        []string      `yaml:"group_by"`
	GroupLimit     int           `yaml:"group_limit"`
	MinCardinality uint64        `yaml:"min_cardinality"`
	CalculateChurn bool          `yaml:"calculate_churn"`
	HLLPrecision   uint8         `yaml:"hll_precision"`
	Buckets        int           `yaml:"buckets"`

	matchers []*labels.Matcher
}

// LoadConfig reads and validates a cardinality estimator YAML configuration file.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- Configuration file path is controlled by operator CLI flag.
	if err != nil {
		return nil, fmt.Errorf("cannot read config file %q: %w", path, err)
	}
	return ParseConfig(data)
}

// ParseConfig unmarshals and validates a cardinality estimator YAML configuration.
func ParseConfig(data []byte) (*Config, error) {
	var cfg Config
	if err := yaml.UnmarshalStrict(data, &cfg); err != nil {
		return nil, fmt.Errorf("cannot unmarshal estimator config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// Validate checks and applies defaults to the Config.
func (c *Config) Validate() error {
	if c.MaxItemsPerInterval <= 0 {
		c.MaxItemsPerInterval = defaultMaxItems
	}
	if len(c.Streams) == 0 {
		return fmt.Errorf("at least one stream must be configured")
	}
	for i := range c.Streams {
		if err := c.Streams[i].validate(); err != nil {
			return fmt.Errorf("invalid stream[%d]: %w", i, err)
		}
	}
	return nil
}

func (sc *StreamConfig) validate() error {
	if sc.Interval < minInterval {
		return fmt.Errorf("interval must be >= %s, got %s", minInterval, sc.Interval)
	}
	if len(sc.GroupBy) > maxGroupBy {
		return fmt.Errorf("group_by cannot exceed %d labels, got %d", maxGroupBy, len(sc.GroupBy))
	}

	seen := make(map[string]struct{}, len(sc.GroupBy))
	hasLabel := false
	normalized := make([]string, 0, len(sc.GroupBy))
	for _, g := range sc.GroupBy {
		if g == "" {
			return fmt.Errorf("group_by label cannot be empty")
		}
		if g == groupKeyword || g == globalKeyword {
			return fmt.Errorf("group_by cannot use reserved keyword %q", g)
		}
		if _, exists := seen[g]; exists {
			return fmt.Errorf("duplicate label %q in group_by", g)
		}
		seen[g] = struct{}{}
		if g == labelKeyword {
			hasLabel = true
			continue
		}
		normalized = append(normalized, g)
	}
	if hasLabel {
		normalized = append(normalized, labelKeyword)
	}
	sc.GroupBy = normalized

	if sc.GroupLimit <= 0 {
		sc.GroupLimit = defaultLimit
	}
	if sc.HLLPrecision == 0 {
		sc.HLLPrecision = hll.DefaultPrecision
	}
	if sc.HLLPrecision < hll.MinPrecision || sc.HLLPrecision > hll.MaxPrecision {
		return fmt.Errorf("hll_precision must be in [%d, %d], got %d", hll.MinPrecision, hll.MaxPrecision, sc.HLLPrecision)
	}
	if sc.Buckets <= 0 {
		sc.Buckets = defaultBuckets
	}

	if sc.Filter != "" {
		matchers, err := parser.ParseMetricSelector(sc.Filter)
		if err != nil {
			return fmt.Errorf("invalid filter %q: %w", sc.Filter, err)
		}
		sc.matchers = matchers
	}
	return nil
}

// HasLabelKeyword reports whether any stream in the config groups by __label__.
func (c *Config) HasLabelKeyword() bool {
	for i := range c.Streams {
		if slices.Contains(c.Streams[i].GroupBy, labelKeyword) {
			return true
		}
	}
	return false
}
