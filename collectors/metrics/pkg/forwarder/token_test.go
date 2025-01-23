// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package forwarder

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
)

func TestTokenFile_Renewal(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	assert.NoError(t, err)

	// Create test token with close expiration time, and save it in a file
	expiresAt := time.Now().Add(3 * time.Second)
	claims := jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(expiresAt),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	tokenStr, err := token.SignedString(privateKey)
	assert.NoError(t, err)
	tmpFile := filepath.Join(t.TempDir(), "token")
	err = os.WriteFile(tmpFile, []byte(tokenStr), 0644)
	assert.NoError(t, err)

	// Create token file with short backoff and wait to trigger failing and finally succesful reads
	backoff := 1 * time.Second
	tf, err := NewTokenFile(context.Background(), log.NewLogfmtLogger(os.Stderr), tmpFile, backoff)
	assert.NoError(t, err)
	assert.Equal(t, tokenStr, tf.GetToken())
	time.Sleep(2 * backoff)

	// Update token file
	expiresAt = time.Now().Add(1 * time.Hour)
	claims = jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(expiresAt),
	}
	newToken := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	newTokenStr, err := newToken.SignedString(privateKey)
	assert.NoError(t, err)
	assert.NotEqual(t, tokenStr, newTokenStr)
	err = os.WriteFile(tmpFile, []byte(newTokenStr), 0644)
	assert.NoError(t, err)

	// Check that the token has been updated
	time.Sleep(2 * backoff)
	assert.Equal(t, newTokenStr, tf.GetToken())
}

func TestTokenFile_ComputeWaitTime(t *testing.T) {
	testCases := map[string]struct {
		backoff        time.Duration
		expiration     time.Time
		waitPercentage int
		minDuration    time.Duration
		expects        time.Duration
	}{
		"no backoff": {
			expiration:     time.Now().Add(100 * time.Minute),
			backoff:        2 * time.Minute,
			minDuration:    10 * time.Minute,
			waitPercentage: 85,
			expects:        85 * time.Minute,
		},
		"below min duration": {
			expiration:     time.Now().Add(30 * time.Minute), // 85% of 30 min is 25m30, remains 4m30s which is below 4*backoff
			backoff:        2 * time.Minute,
			minDuration:    10 * time.Minute,
			waitPercentage: 85,
			expects:        20 * time.Minute,
		},
		"below backoff duration": {
			expiration:     time.Now().Add(10 * time.Minute), // 85% of 10m is 8m30s, remains 1m30s, which is below backoff
			backoff:        2 * time.Minute,
			minDuration:    10 * time.Minute,
			waitPercentage: 85,
			expects:        2 * time.Minute,
		},
		"expired": {
			expiration:     time.Now().Add(-10 * time.Minute),
			backoff:        2 * time.Minute,
			minDuration:    10 * time.Minute,
			waitPercentage: 85,
			expects:        2 * time.Minute,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			res := computeWaitTime(tc.expiration, tc.waitPercentage, tc.backoff, tc.minDuration)
			assert.InEpsilon(t, tc.expects.Seconds(), res.Seconds(), 1, fmt.Sprintf("expected %.1f seconds, got %.1f seconds", tc.expects.Seconds(), res.Seconds()))
		})
	}
}

func TestTokenFile_ParseExpiration(t *testing.T) {
	// Invalid token
	_, err := parseTokenExpiration("aaa.bbb.ccc")
	assert.Error(t, err)

	// No expiration
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	assert.NoError(t, err)
	claims := jwt.RegisteredClaims{
		IssuedAt: jwt.NewNumericDate(time.Now()),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	tokenStr, err := token.SignedString(privateKey)
	assert.NoError(t, err)
	assert.NotEmpty(t, tokenStr)
	_, err = parseTokenExpiration(tokenStr)
	assert.Error(t, err)

	// Valid expiration
	expiresAt := time.Unix(1737557854, 0)
	claims = jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(expiresAt),
	}
	token = jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	tokenStr, err = token.SignedString(privateKey)
	assert.NoError(t, err)
	assert.NotEmpty(t, tokenStr)
	expiration, err := parseTokenExpiration(tokenStr)
	assert.NoError(t, err)
	assert.Equal(t, expiresAt.Unix(), expiration.Unix())
}
