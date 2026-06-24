// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package main

import (
	"reflect"
	"testing"
)

func TestExecute(t *testing.T) {
	// NOTE: This test mutates package-level global runner variables (mcoaRunner, standardRunner, cleanupRunner).
	// To prevent data races and ensure test isolation, do NOT use t.Parallel() here or in any other
	// tests in this package that modify these globals.

	// Save original runners
	origMcoa := mcoaRunner
	origStandard := standardRunner
	origCleanup := cleanupRunner
	defer func() {
		mcoaRunner = origMcoa
		standardRunner = origStandard
		cleanupRunner = origCleanup
	}()

	tests := []struct {
		name         string
		args         []string
		expectedCmd  string // "mcoa", "standard", "cleanup"
		expectedArgs []string
	}{
		{
			name:         "no args",
			args:         []string{"./endpointmetrics"},
			expectedCmd:  "standard",
			expectedArgs: []string{},
		},
		{
			name:         "standard with no flags",
			args:         []string{"./endpointmetrics", "standard"},
			expectedCmd:  "standard",
			expectedArgs: []string{},
		},
		{
			name:         "standard with flags",
			args:         []string{"./endpointmetrics", "standard", "--metrics-bind-address=:8080"},
			expectedCmd:  "standard",
			expectedArgs: []string{"--metrics-bind-address=:8080"},
		},
		{
			name:         "legacy with no flags",
			args:         []string{"./endpointmetrics", "legacy"},
			expectedCmd:  "standard",
			expectedArgs: []string{},
		},
		{
			name:         "legacy with flags",
			args:         []string{"./endpointmetrics", "legacy", "--metrics-bind-address=:8080"},
			expectedCmd:  "standard",
			expectedArgs: []string{"--metrics-bind-address=:8080"},
		},
		{
			name:         "only flags default to standard",
			args:         []string{"./endpointmetrics", "--metrics-bind-address=:8080"},
			expectedCmd:  "standard",
			expectedArgs: []string{"--metrics-bind-address=:8080"},
		},
		{
			name:         "mcoa with flags",
			args:         []string{"./endpointmetrics", "mcoa", "--hub-id=123"},
			expectedCmd:  "mcoa",
			expectedArgs: []string{"--hub-id=123"},
		},
		{
			name:         "cleanup with flags",
			args:         []string{"./endpointmetrics", "cleanup", "--hub-alertmanager-ca-secret=obs-alertmanager-mtls-ca"},
			expectedCmd:  "cleanup",
			expectedArgs: []string{"--hub-alertmanager-ca-secret=obs-alertmanager-mtls-ca"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calledCmd string
			var calledArgs []string

			mcoaRunner = func(args []string) {
				calledCmd = "mcoa"
				calledArgs = args
			}
			standardRunner = func(args []string) {
				calledCmd = "standard"
				calledArgs = args
			}
			cleanupRunner = func(args []string) {
				calledCmd = "cleanup"
				calledArgs = args
			}

			execute(tt.args)

			if calledCmd != tt.expectedCmd {
				t.Errorf("expected cmd %q, got %q", tt.expectedCmd, calledCmd)
			}
			if !reflect.DeepEqual(calledArgs, tt.expectedArgs) {
				t.Errorf("expected args %v, got %v", tt.expectedArgs, calledArgs)
			}
		})
	}
}
