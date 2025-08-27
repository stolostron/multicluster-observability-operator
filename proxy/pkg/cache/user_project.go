// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package cache

import (
	"sync"
	"time"

	"k8s.io/klog"
)

const defaultCleanPeriod = time.Second * 60 * 5

type UserProjectInfo struct {
	mu              sync.RWMutex
	projectInfo     map[string]userProject
	expiredDuration time.Duration
	cleanPeriod     time.Duration
	stopCh          chan struct{}
}

type userProject struct {
	UserName    string
	Timestamp   time.Time
	ProjectList []string
}

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

func (upi *UserProjectInfo) UpdateUserProject(userName string, token string, projects []string) {
	upi.mu.Lock()
	upi.projectInfo[token] = userProject{
		UserName:    userName,
		Timestamp:   time.Now(),
		ProjectList: projects,
	}
	upi.mu.Unlock()
}

func (upi *UserProjectInfo) GetUserProjectList(token string) ([]string, bool) {
	upi.mu.RLock()
	up, ok := upi.projectInfo[token]
	upi.mu.RUnlock()
	if ok {
		return up.ProjectList, true
	}
	return []string{}, false
}

func (upi *UserProjectInfo) Stop() {
	close(upi.stopCh)
}

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
