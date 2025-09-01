// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

// Package cache provides a thread-safe, in-memory cache to store the list of projects a user has access to.
// It uses the user's authentication token, which is forwarded by Grafana in an HTTP header, as the cache key.
//
// This custom cache is necessary because determining a user's project access requires making API requests
// to the OpenShift API server with a client authenticated via that specific user's token. This is different from the main operator's
// client, which uses a single service account. As a result, the standard controller-runtime client-side
// caching mechanism cannot be leveraged for this purpose.
//
// The cache has a configurable expiration time for entries and a background process for automatic cleanup
// to avoid repeatedly fetching the same data from the API server.
package cache

import (
	"slices"
	"sync"
	"time"

	"k8s.io/klog"
)

const defaultCleanPeriod = time.Second * 60 * 5

// UserProjectInfo holds the cache of user project lists, keyed by user tokens.
// It manages concurrent access and automatic cleanup of expired entries.
type UserProjectInfo struct {
	mu              sync.RWMutex
	projectInfo     map[string]userProject
	expiredDuration time.Duration
	cleanPeriod     time.Duration
	stopCh          chan struct{}
}

// userProject stores the list of projects for a user at a specific point in time.
type userProject struct {
	UserName    string
	Timestamp   time.Time
	ProjectList []string
}

// NewUserProjectInfo creates and starts a new UserProjectInfo cache.
// It takes an expiration duration for cache entries and a cleaning period for the cleanup goroutine.
func NewUserProjectInfo(expiredDuration, cleanPeriod time.Duration) *UserProjectInfo {
	if cleanPeriod <= 0 {
		cleanPeriod = defaultCleanPeriod
	}
	upi := &UserProjectInfo{
		projectInfo:     map[string]userProject{},
		expiredDuration: expiredDuration,
		cleanPeriod:     cleanPeriod,
		stopCh:          make(chan struct{}),
	}

	go upi.autoCleanExpiredProjectInfo()

	return upi
}

// UpdateUserProject adds or updates a user's project list in the cache.
// The entry is timestamped with the current time.
func (upi *UserProjectInfo) UpdateUserProject(userName string, token string, projects []string) {
	upi.mu.Lock()
	upi.projectInfo[token] = userProject{
		UserName:    userName,
		Timestamp:   time.Now(),
		ProjectList: projects,
	}
	upi.mu.Unlock()
}

// GetUserProjectList retrieves a user's project list from the cache using their token.
// It returns a copy of the project list and a boolean indicating if the entry was found.
// The slice is copied to prevent the caller from modifying the cached data.
func (upi *UserProjectInfo) GetUserProjectList(token string) ([]string, bool) {
	upi.mu.RLock()
	up, ok := upi.projectInfo[token]
	upi.mu.RUnlock()
	if ok {
		return slices.Clone(up.ProjectList), true
	}
	return []string{}, false
}

// GetUserName retrieves a user's name from the cache using their token.
// It returns the username and a boolean indicating if the entry was found.
func (upi *UserProjectInfo) GetUserName(token string) (string, bool) {
	upi.mu.RLock()
	defer upi.mu.RUnlock()
	up, ok := upi.projectInfo[token]
	if !ok {
		return "", false
	}
	return up.UserName, true
}


// Stop terminates the background cleanup goroutine.
func (upi *UserProjectInfo) Stop() {
	close(upi.stopCh)
}

// autoCleanExpiredProjectInfo runs a periodic check to clean expired entries from the cache.
func (upi *UserProjectInfo) autoCleanExpiredProjectInfo() {
	ticker := time.NewTicker(upi.cleanPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			upi.cleanExpiredProjectInfo()
		case <-upi.stopCh:
			klog.Info("stopping user project info auto cleaning")
			return
		}
	}
}

// cleanExpiredProjectInfo iterates through the cache and removes entries that have exceeded their expiration duration.
func (upi *UserProjectInfo) cleanExpiredProjectInfo() {
	var expiredTokens []string
	upi.mu.RLock()
	for token, up := range upi.projectInfo {
		if time.Since(up.Timestamp) >= upi.expiredDuration {
			expiredTokens = append(expiredTokens, token)
		}
	}
	upi.mu.RUnlock()

	if len(expiredTokens) > 0 {
		upi.mu.Lock()
		defer upi.mu.Unlock()
		for _, token := range expiredTokens {
			if up, ok := upi.projectInfo[token]; ok && time.Since(up.Timestamp) >= upi.expiredDuration {
				klog.Infof("clean %v project info", up.UserName)
				delete(upi.projectInfo, token)
			}
		}
	}
}
