// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package util

import (
	"strconv"
	"testing"
	"time"
)

func TestGetUserProjectList(t *testing.T) {
	testCaseList := []struct {
		name            string
		token           string
		userProjectInfo *UserProjectInfo
		expected        bool
	}{
		{
			"should has user project",
			"1",
			&UserProjectInfo{
				ProjectInfo: map[string]UserProject{
					"1": UserProject{
						UserName:    "user" + strconv.Itoa(1),
						Timestamp:   time.Now().Unix(),
						Token:       strconv.Itoa(1),
						ProjectList: []string{"p" + strconv.Itoa(1)},
					},
				},
			},
			true,
		},

		{
			"should has not user project",
			"invalid",
			&UserProjectInfo{
				ProjectInfo: map[string]UserProject{
					"1": UserProject{
						UserName:    "user" + strconv.Itoa(1),
						Timestamp:   time.Now().Unix(),
						Token:       strconv.Itoa(1),
						ProjectList: []string{"p" + strconv.Itoa(1)},
					},
				},
			},
			false,
		},
	}

	for _, c := range testCaseList {
		userProjectInfo = c.userProjectInfo
		_, output := GetUserProjectList(c.token)
		if output != c.expected {
			t.Errorf("case (%v) output: (%v) is not the expected: (%v)", c.name, output, c.expected)
		}
	}
}

func TestCleanExpiredProjectInfo(t *testing.T) {
	testCaseList := []struct {
		name            string
		token           string
		userProjectInfo *UserProjectInfo
		expected        bool
	}{
		{
			"user project should expired",
			"1",
			&UserProjectInfo{
				ProjectInfo: map[string]UserProject{
					"1": UserProject{
						UserName:    "user" + strconv.Itoa(1),
						Timestamp:   time.Now().Unix(),
						Token:       strconv.Itoa(1),
						ProjectList: []string{"p" + strconv.Itoa(1)},
					},
				},
			},
			false,
		},

		{
			"user project should not expired",
			"2",
			&UserProjectInfo{
				ProjectInfo: map[string]UserProject{
					"2": UserProject{
						UserName:    "user" + strconv.Itoa(2),
						Timestamp:   time.Now().Unix() + 10,
						Token:       strconv.Itoa(2),
						ProjectList: []string{"p" + strconv.Itoa(2)},
					},
				},
			},
			true,
		},
	}

	go CleanExpiredProjectInfo(1)
	for _, c := range testCaseList {
		userProjectInfo = c.userProjectInfo
		time.Sleep(time.Second * 2)
		_, output := GetUserProjectList(c.token)
		if output != c.expected {
			t.Errorf("case (%v) output: (%v) is not the expected: (%v)", c.name, output, c.expected)
		}
	}
}
