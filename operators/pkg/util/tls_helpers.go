// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package util

import (
	"fmt"
	"strings"

	ocinfrav1 "github.com/openshift/api/config/v1"
)

// AppendOauthProxyTLSArgs adds the appropriate --tls-min-version and --tls-cipher-suites flags
// to the provided args slice based on the TLS security profile.
func AppendOauthProxyTLSArgs(args []string, profile *ocinfrav1.TLSSecurityProfile) []string {
	args = append(args, fmt.Sprintf("--tls-min-version=%s", GetOauthProxyTLSMinVersion(profile)))
	if ciphers := GetTLSProfileIANACiphers(profile); len(ciphers) > 0 {
		args = append(args, fmt.Sprintf("--tls-cipher-suites=%s", strings.Join(ciphers, ",")))
	}
	return args
}

// AppendKubeRBACProxyTLSArgs adds the appropriate --tls-min-version and --tls-cipher-suites flags
// to the provided args slice based on the TLS security profile.
func AppendKubeRBACProxyTLSArgs(args []string, profile *ocinfrav1.TLSSecurityProfile) []string {
	args = append(args, fmt.Sprintf("--tls-min-version=%s", GetTLSProfileStringMinVersion(profile)))
	if ciphers := GetTLSProfileIANACiphers(profile); len(ciphers) > 0 {
		args = append(args, fmt.Sprintf("--tls-cipher-suites=%s", strings.Join(ciphers, ",")))
	}
	return args
}
