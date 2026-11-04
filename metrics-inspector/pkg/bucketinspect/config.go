// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package bucketinspect

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/go-kit/log"
	"github.com/prometheus/common/model"
	"github.com/thanos-io/objstore"
	objstoreclient "github.com/thanos-io/objstore/client"
)

const (
	// DefaultSyncInterval is the default interval between Thanos bucket inspections.
	DefaultSyncInterval = 15 * time.Minute

	// DefaultRetentionDuration is the default configured retention duration (365d).
	DefaultRetentionDuration = "365d"

	// DefaultRetentionSeconds is 365 days in seconds.
	DefaultRetentionSeconds = 365 * 24 * 60 * 60
)

// RetentionConfig holds the configured Thanos Compactor retention durations in seconds
// per resolution tier ("raw", "5m", "1h"). A value of 0 means retention is disabled (infinite).
type RetentionConfig struct {
	RawSeconds float64
	FiveMinSec float64
	OneHourSec float64
}

// DefaultRetentionConfig returns the default 365d retention configuration across all resolution tiers.
func DefaultRetentionConfig() RetentionConfig {
	return RetentionConfig{
		RawSeconds: DefaultRetentionSeconds,
		FiveMinSec: DefaultRetentionSeconds,
		OneHourSec: DefaultRetentionSeconds,
	}
}

// NewBucketFromFile reads a Thanos object storage YAML file and initializes an objstore.Bucket.
func NewBucketFromFile(path string) (objstore.Bucket, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- Path is controlled by operator CLI flag.
	if err != nil {
		return nil, fmt.Errorf("cannot read object storage config file %q: %w", path, err)
	}
	return NewBucket(data)
}

// NewBucket initializes an upstream Thanos objstore.Bucket from a raw object storage YAML configuration.
func NewBucket(confContentYaml []byte) (objstore.Bucket, error) {
	bkt, err := objstoreclient.NewBucket(log.NewNopLogger(), confContentYaml, "metrics-inspector", nil)
	if err != nil {
		return nil, fmt.Errorf("cannot create object storage bucket client: %w", err)
	}
	return bkt, nil
}

// ParseDurationSeconds parses a Prometheus/Thanos duration string (such as "365d", "14d", "24h", "0s")
// and returns the duration in seconds.
func ParseDurationSeconds(raw string) (float64, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "0" {
		return 0, nil
	}
	d, err := model.ParseDuration(trimmed)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q: %w", raw, err)
	}
	return time.Duration(d).Seconds(), nil
}

// ParseRetentionConfig parses the raw, 5m, and 1h retention duration strings into a RetentionConfig.
func ParseRetentionConfig(raw, fiveMin, oneHour string) (RetentionConfig, error) {
	rawSec, err := ParseDurationSeconds(raw)
	if err != nil {
		return RetentionConfig{}, fmt.Errorf("invalid raw retention duration %q: %w", raw, err)
	}
	fiveMinSec, err := ParseDurationSeconds(fiveMin)
	if err != nil {
		return RetentionConfig{}, fmt.Errorf("invalid 5m retention duration %q: %w", fiveMin, err)
	}
	oneHourSec, err := ParseDurationSeconds(oneHour)
	if err != nil {
		return RetentionConfig{}, fmt.Errorf("invalid 1h retention duration %q: %w", oneHour, err)
	}
	return RetentionConfig{
		RawSeconds: rawSec,
		FiveMinSec: fiveMinSec,
		OneHourSec: oneHourSec,
	}, nil
}
