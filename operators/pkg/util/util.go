// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package util

import (
	"net/http"
	"net/http/pprof"
)

func RegisterDebugEndpoint(register func(string, http.Handler) error) error {
	err := register("/debug/", http.Handler(http.DefaultServeMux))
	if err != nil {
		return err
	}
	err = register("/debug/pprof/", http.HandlerFunc(pprof.Index))
	if err != nil {
		return err
	}
	err = register("/debug/pprof/cmdline", http.HandlerFunc(pprof.Cmdline))
	if err != nil {
		return err
	}
	err = register("/debug/pprof/block", http.Handler(pprof.Handler("block")))
	if err != nil {
		return err
	}
	err = register("/debug/pprof/profile", http.HandlerFunc(pprof.Profile))
	if err != nil {
		return err
	}
	err = register("/debug/pprof/symobol", http.HandlerFunc(pprof.Symbol))
	if err != nil {
		return err
	}
	err = register("/debug/pprof/trace", http.HandlerFunc(pprof.Trace))
	if err != nil {
		return err
	}

	return nil
}
