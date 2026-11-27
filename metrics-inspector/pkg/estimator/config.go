// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package estimator

import (
	"fmt"
	"os"
	"regexp"
	"slices"
	"time"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/stolostron/multicluster-observability-operator/metrics-inspector/pkg/hll"
	"gopkg.in/yaml.v2"
)

const (
	// DefaultInterval is the default sliding window duration for cardinality estimation streams.
	DefaultInterval = 15 * time.Minute
	// DefaultMaxItemsPerInterval is the default Bloom filter capacity per sliding interval.
	DefaultMaxItemsPerInterval = 15000000

	minInterval     = time.Minute
	maxGroupBy      = 5
	defaultBuckets  = 15
	defaultLimit    = 10000
	defaultMaxItems = DefaultMaxItemsPerInterval
	labelKeyword    = "__label__"
	groupKeyword    = "__group__"
	globalKeyword   = "__global__"
)

// Config represents the top-level YAML configuration for the cardinality estimator.
type Config struct {
	MaxItemsPerInterval int            `yaml:"max_items_per_interval"`
	ChurnExcludeRegex   string         `yaml:"churn_exclude_regex"`
	Streams             []StreamConfig `yaml:"streams"`
}

// StreamConfig configures a single sliding-window cardinality estimation stream.
type StreamConfig struct {
	Interval          time.Duration `yaml:"interval"`
	Filter            string        `yaml:"filter"`
	GroupBy           []string      `yaml:"group_by"`
	GroupLimit        int           `yaml:"group_limit"`
	MinCardinality    uint64        `yaml:"min_cardinality"`
	CalculateChurn    bool          `yaml:"calculate_churn"`
	HLLPrecision      uint8         `yaml:"hll_precision"`
	Buckets           int           `yaml:"buckets"`
	ChurnExcludeRegex string        `yaml:"churn_exclude_regex,omitempty"`

	matchers       []*labels.Matcher
	churnExcludeRe *regexp.Regexp
}

// LoadConfig reads and validates a cardinality estimator YAML configuration file.
func LoadConfig(path string) (*Config, error) {
	return LoadConfigWithInterval(path, DefaultInterval)
}

// LoadConfigWithInterval reads and validates a cardinality estimator YAML configuration file,
// applying defaultInterval to any stream that does not specify an explicit interval.
func LoadConfigWithInterval(path string, defaultInterval time.Duration) (*Config, error) {
	return LoadConfigWithDefaults(path, defaultInterval, DefaultMaxItemsPerInterval)
}

// ConfigDefaults specifies default values applied when omitted in YAML configuration.
type ConfigDefaults struct {
	Interval          time.Duration
	MaxItems          int
	ChurnExcludeRegex string
}

// LoadConfigWithDefaults reads and validates a cardinality estimator YAML configuration file,
// applying defaultInterval and defaultMaxItems when omitted in the YAML file.
func LoadConfigWithDefaults(path string, defaultInterval time.Duration, defaultMaxItems int) (*Config, error) {
	return LoadConfigWithAllDefaults(path, ConfigDefaults{
		Interval: defaultInterval,
		MaxItems: defaultMaxItems,
	})
}

// LoadConfigWithAllDefaults reads and validates a cardinality estimator YAML configuration file,
// applying defaults when omitted in the YAML file.
func LoadConfigWithAllDefaults(path string, defaults ConfigDefaults) (*Config, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- Configuration file path is controlled by operator CLI flag.
	if err != nil {
		return nil, fmt.Errorf("cannot read config file %q: %w", path, err)
	}
	return ParseConfigWithAllDefaults(data, defaults)
}

// ParseConfig unmarshals and validates a cardinality estimator YAML configuration.
func ParseConfig(data []byte) (*Config, error) {
	return ParseConfigWithInterval(data, DefaultInterval)
}

// ParseConfigWithInterval unmarshals and validates a cardinality estimator YAML configuration,
// applying defaultInterval to any stream that does not specify an explicit interval.
func ParseConfigWithInterval(data []byte, defaultInterval time.Duration) (*Config, error) {
	return ParseConfigWithDefaults(data, defaultInterval, DefaultMaxItemsPerInterval)
}

// ParseConfigWithDefaults unmarshals and validates a cardinality estimator YAML configuration,
// applying defaultInterval and defaultMaxItems when omitted in the YAML data.
func ParseConfigWithDefaults(data []byte, defaultInterval time.Duration, defaultMaxItems int) (*Config, error) {
	return ParseConfigWithAllDefaults(data, ConfigDefaults{
		Interval: defaultInterval,
		MaxItems: defaultMaxItems,
	})
}

// ParseConfigWithAllDefaults unmarshals and validates a cardinality estimator YAML configuration,
// applying defaults when omitted in the YAML data.
func ParseConfigWithAllDefaults(data []byte, defaults ConfigDefaults) (*Config, error) {
	var cfg Config
	if err := yaml.UnmarshalStrict(data, &cfg); err != nil {
		return nil, fmt.Errorf("cannot unmarshal estimator config: %w", err)
	}
	if cfg.MaxItemsPerInterval <= 0 && defaults.MaxItems > 0 {
		cfg.MaxItemsPerInterval = defaults.MaxItems
	}
	if cfg.ChurnExcludeRegex == "" && defaults.ChurnExcludeRegex != "" {
		cfg.ChurnExcludeRegex = defaults.ChurnExcludeRegex
	}
	if defaults.Interval > 0 {
		for i := range cfg.Streams {
			if cfg.Streams[i].Interval == 0 {
				cfg.Streams[i].Interval = defaults.Interval
			}
		}
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// ApplyChurnExcludeDefault sets ChurnExcludeRegex to defaultPattern if not already set.
func (c *Config) ApplyChurnExcludeDefault(defaultPattern string) error {
	if c.ChurnExcludeRegex == "" && defaultPattern != "" {
		c.ChurnExcludeRegex = defaultPattern
		return c.Validate()
	}
	return nil
}

// Validate checks and applies defaults to the Config.
func (c *Config) Validate() error {
	var churnExcludeRe *regexp.Regexp
	if c.ChurnExcludeRegex != "" {
		re, err := regexp.Compile(c.ChurnExcludeRegex)
		if err != nil {
			return fmt.Errorf("invalid churn_exclude_regex %q: %w", c.ChurnExcludeRegex, err)
		}
		churnExcludeRe = re
	}

	if c.MaxItemsPerInterval <= 0 {
		c.MaxItemsPerInterval = defaultMaxItems
	}
	if len(c.Streams) == 0 {
		return fmt.Errorf("at least one stream must be configured")
	}
	for i := range c.Streams {
		if c.Streams[i].ChurnExcludeRegex == "" && c.Streams[i].churnExcludeRe == nil {
			c.Streams[i].churnExcludeRe = churnExcludeRe
		}
		if err := c.Streams[i].validate(); err != nil {
			return fmt.Errorf("invalid stream[%d]: %w", i, err)
		}
	}
	return nil
}

// ChurnExcludeRe returns the compiled regex used to exclude metrics from churn calculation.
func (sc *StreamConfig) ChurnExcludeRe() *regexp.Regexp {
	return sc.churnExcludeRe
}

// SetChurnExcludeRegex compiles and sets the churn exclusion regex on the stream.
func (sc *StreamConfig) SetChurnExcludeRegex(pattern string) error {
	if pattern == "" {
		sc.churnExcludeRe = nil
		sc.ChurnExcludeRegex = ""
		return nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("invalid churn_exclude_regex %q: %w", pattern, err)
	}
	sc.churnExcludeRe = re
	sc.ChurnExcludeRegex = pattern
	return nil
}

func (sc *StreamConfig) validate() error {
	if sc.churnExcludeRe == nil && sc.ChurnExcludeRegex != "" {
		re, err := regexp.Compile(sc.ChurnExcludeRegex)
		if err != nil {
			return fmt.Errorf("invalid churn_exclude_regex %q: %w", sc.ChurnExcludeRegex, err)
		}
		sc.churnExcludeRe = re
	}
	if sc.Interval == 0 {
		sc.Interval = DefaultInterval
	}
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
