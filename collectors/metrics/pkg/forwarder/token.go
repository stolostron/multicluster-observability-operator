// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package forwarder

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/golang-jwt/jwt/v5"
	rlogger "github.com/stolostron/multicluster-observability-operator/collectors/metrics/pkg/logger"
)

const remainingDurationBeforeBackoff = 10 * time.Minute

var (
	ErrEmptyTokenFilePath     = errors.New("token file path is empty")
	ErrEmptyToken             = errors.New("token is empty")
	ErrMissingExpirationClaim = errors.New("missing expiration claim")
)

type TokenFile struct {
	filePath    string
	logger      log.Logger
	readBackoff time.Duration
	token       string
	expiration  time.Time
	tokenMu     sync.RWMutex
}

// NewTokenFile initiates a new TokenFile.
// It reads the token value from the provided filePath and the caller can access this value using the GetToken() method.
// The token value is automatically updated by re-reading the file as the token approaches expiration.
func NewTokenFile(ctx context.Context, logger log.Logger, filePath string, readBackoff time.Duration) (*TokenFile, error) {
	if len(filePath) == 0 {
		return nil, ErrEmptyTokenFilePath
	}

	tf := &TokenFile{
		filePath:    filePath,
		logger:      logger,
		readBackoff: readBackoff,
	}

	// Initiate token value
	if _, err := tf.renewTokenFromFile(); err != nil {
		return nil, err
	}

	go tf.autoRenew(ctx)

	return tf, nil
}

func (t *TokenFile) renewTokenFromFile() (bool, error) {
	rawToken, err := os.ReadFile(t.filePath)
	if err != nil {
		return false, fmt.Errorf("failed to read token file: %w", err)
	}

	token := strings.TrimSpace(string(rawToken))
	if len(token) == 0 {
		return false, ErrEmptyToken
	}

	exp, err := parseTokenExpiration(token)
	if err != nil {
		return false, fmt.Errorf("failed to parse token expiration time: %w", err)
	}

	t.tokenMu.Lock()
	defer t.tokenMu.Unlock()

	if t.token == token {
		return false, nil
	}

	t.token = token
	t.expiration = exp

	return true, nil
}

func (t *TokenFile) GetToken() string {
	t.tokenMu.RLock()
	defer t.tokenMu.RUnlock()
	return t.token
}

// autoRenew automatically re-read the token file to update its value when it approaches the expiration time.
// The objective is to have a simple and robust strategy.
// Most lifetimes are 1y or 1h. Assuming that kubernetes renews the token when it reaches 80% of its lifetime, it is renewed 12 min before exp with 1h lifetime.
// The strategy is to read the token file every backoff duration until success, starting 10 minutes before expiration.
func (t *TokenFile) autoRenew(ctx context.Context) {
	for {
		t.tokenMu.RLock()
		exp := t.expiration
		t.tokenMu.RUnlock()

		waitTime := computeWaitTime(exp, t.readBackoff, remainingDurationBeforeBackoff)
		timer := time.NewTimer(waitTime)
		rlogger.Log(t.logger, rlogger.Info, "msg", "Token renewal triggered", "waitTime", waitTime)
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}

		ok, err := t.renewTokenFromFile()
		if err != nil {
			if waitTime <= t.readBackoff {
				rlogger.Log(t.logger, rlogger.Error, "msg", "Failed to renew token", "error", err, "expiration", t.expiration, "path", t.filePath)
			} else {
				rlogger.Log(t.logger, rlogger.Warn, "msg", "Failed to renew token", "error", err, "expiration", t.expiration, "path", t.filePath)
			}
		}

		if !ok && waitTime <= t.readBackoff {
			rlogger.Log(t.logger, rlogger.Warn, "msg", "Failed to renew token while approaching expiration, same token read from file", "expiration", t.expiration, "path", t.filePath)
		}

		if ok {
			rlogger.Log(t.logger, rlogger.Info, "msg", "Successful Token renewal from file")
		}
	}
}

func parseTokenExpiration(token string) (time.Time, error) {
	parsedToken, _, err := jwt.NewParser().ParseUnverified(token, jwt.MapClaims{})
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to parse JWT: %w", err)
	}

	exp, err := parsedToken.Claims.GetExpirationTime()
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to get expiration time: %w", err)
	}

	if exp == nil {
		return time.Time{}, ErrMissingExpirationClaim
	}

	return exp.Time, nil
}

func computeWaitTime(exiprationTime time.Time, backoff, remainingDurationBeforeBackoff time.Duration) time.Duration {
	timeUntilExp := time.Until(exiprationTime)
	timeToWait := timeUntilExp - remainingDurationBeforeBackoff - backoff

	if timeToWait < backoff {
		timeToWait = backoff
	}

	return timeToWait
}
