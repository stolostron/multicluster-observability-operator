// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package util

import (
	"sync"
	"time"

	"k8s.io/klog"
)

var userProjectInfo *UserProjectInfo

type UserProjectInfo struct {
	sync.RWMutex
	ProjectInfo map[string]UserProject
}

type UserProject struct {
	UserName    string
	Timestamp   int64
	Token       string
	ProjectList []string
}

func InitUserProjectInfo() {
	if userProjectInfo == nil {
		userProjectInfo = new(UserProjectInfo)
		userProjectInfo.ProjectInfo = map[string]UserProject{}
	}
}

func NewUserProject(userName string, token string, projects []string) UserProject {
	up := UserProject{}
	up.UserName = userName
	up.Timestamp = time.Now().Unix()
	up.Token = token
	up.ProjectList = projects
	return up
}

func UpdateUserProject(up UserProject) {
	userProjectInfo.Lock()
	userProjectInfo.ProjectInfo[up.Token] = up
	userProjectInfo.Unlock()
}

func GetUserProjectList(token string) ([]string, bool) {
	userProjectInfo.Lock()
	defer userProjectInfo.Unlock()
	up, ok := userProjectInfo.ProjectInfo[token]
	if ok {
		return up.ProjectList, true
	}
	return []string{}, false
}

func CleanExpiredProjectInfoJob(expiredTimeSeconds int64) {
	InitUserProjectInfo()
	ticker := time.NewTicker(time.Duration(time.Second * time.Duration(expiredTimeSeconds)))
	defer ticker.Stop()

	for {
		<-ticker.C
		CleanExpiredProjectInfo(expiredTimeSeconds)
	}
}

func CleanExpiredProjectInfo(expiredTimeSeconds int64) {
	userProjectInfo.Lock()
	defer userProjectInfo.Unlock()
	for _, up := range userProjectInfo.ProjectInfo {
		if time.Now().Unix()-up.Timestamp >= expiredTimeSeconds {
			klog.Infof("clean %v project info", up.UserName)
			delete(userProjectInfo.ProjectInfo, up.Token)
		}
	}
}
