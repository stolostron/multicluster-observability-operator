// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

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
	userProjectInfo = new(UserProjectInfo)
	userProjectInfo.ProjectInfo = map[string]UserProject{}
}

func NewUserProject(userName string, token string, projects []string) UserProject {
	up := UserProject{}
	up.UserName = userName
	up.Timestamp = time.Now().Unix()
	up.Token = token
	up.ProjectList = projects
	return up
}

func deleteUserProject(up UserProject) {
	userProjectInfo.Lock()
	delete(userProjectInfo.ProjectInfo, up.Token)
	userProjectInfo.Unlock()
}

func UpdateUserProject(up UserProject) {
	userProjectInfo.Lock()
	userProjectInfo.ProjectInfo[up.Token] = up
	userProjectInfo.Unlock()
}

func GetUserProjectList(token string) ([]string, bool) {
	userProjectInfo.Lock()
	up, ok := userProjectInfo.ProjectInfo[token]
	userProjectInfo.Unlock()
	if ok {
		return up.ProjectList, true
	}
	return []string{}, false
}

func CleanExpiredProjectInfo(expiredTimeSeconds int64) {
	InitUserProjectInfo()
	ticker := time.NewTicker(time.Duration(time.Second * time.Duration(expiredTimeSeconds)))
	defer ticker.Stop()

	for {
		<-ticker.C
		for _, up := range userProjectInfo.ProjectInfo {
			if time.Now().Unix()-up.Timestamp >= expiredTimeSeconds {
				klog.Infof("clean %v project info", up.UserName)
				deleteUserProject(up)
			}
		}
	}
}
