// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package util

import (
	"context"
	"crypto/tls"
	"fmt"
	"strings"

	ocinfrav1 "github.com/openshift/api/config/v1"
	tlsutil "github.com/openshift/controller-runtime-common/pkg/tls"
	libgocrypto "github.com/openshift/library-go/pkg/crypto"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	tlsProfileSpec *ocinfrav1.TLSProfileSpec
	tlsConfig      func(*tls.Config)
	// Override in tests to inject a fake client.
	tlsClientFunc = GetOrCreateOCPConfigCRClient
)

// GetOrCreateTLSProfileSpec retrieves spec.tlsSecurityProfile
// from a OCP Cluster API server: apiservers.config.openshift.io/cluster resource
// and applies it based on the adherence policy.
func GetOrCreateTLSProfileSpec(ctx context.Context) (*ocinfrav1.TLSProfileSpec, error) {
	if tlsProfileSpec != nil {
		return tlsProfileSpec, nil
	}

	c, err := tlsClientFunc()
	if err != nil {
		return nil, fmt.Errorf("unable to create client for API server: %w", err)
	}

	tap, err := tlsutil.FetchAPIServerTLSAdherencePolicy(ctx, c)
	if err != nil {
		log.Error(err, "unable to get TLS adherence policy from API server")
		// Default to empty string if the API server is not available or the field is not set.
		// The controller manager will keep a watch on the API server
		// for the field and trigger a restart if the value changes.
		tap = ""
	}

	defaultSpec := ocinfrav1.TLSProfiles[libgocrypto.DefaultTLSProfileType]

	// If the cluster-wide TLS adherence policy is set to honor the cluster-wide TLS profile,
	// use the cluster-wide TLS profile-based spec.
	if libgocrypto.ShouldHonorClusterTLSProfile(tap) {
		tps, err := tlsutil.FetchAPIServerTLSProfile(ctx, c)
		if err != nil {
			// Default to the default spec if the API server is not available or the field is not set.
			// The controller manager will keep a watch on the API server
			// for the field and trigger a restart if the value changes.
			log.Error(err, "unable to get TLS profile from API server")
			tlsProfileSpec = defaultSpec
		} else {
			tlsProfileSpec = &tps
		}
	} else {
		// If the cluster-wide TLS adherence policy is not set to honor the cluster-wide TLS profile,
		// use the default TLS profile-based spec.
		tlsProfileSpec = defaultSpec
	}

	return tlsProfileSpec, nil
}

// GetOrCreateTLSConfig returns a function that configures a tls.Config
// based on the OCP Cluster API server TLSProfileSpec.
func GetOrCreateTLSConfig(ctx context.Context) (func(*tls.Config), error) {
	if tlsConfig != nil {
		return tlsConfig, nil
	}

	profileSpec, err := GetOrCreateTLSProfileSpec(ctx)
	if err != nil {
		return nil, err
	}

	tlsConfig, unsupportedCiphers := tlsutil.NewTLSConfigFromProfile(*profileSpec)
	if len(unsupportedCiphers) > 0 {
		log.Info("TLS configuration contains unsupported ciphers that will be ignored", "ciphers", unsupportedCiphers)
	}

	return tlsConfig, nil
}

func SetTLSSecurityConfiguration(ctx context.Context, args []string, tlsCipherSuitesArg string, minTLSversionArg string) ([]string, error) {
	tlsProfileSpec, err := GetOrCreateTLSProfileSpec(ctx)
	if err != nil {
		log.Error(err, "unable to get TLS security configuration")
		return nil, err
	}

	ciphers := tlsProfileSpec.Ciphers

	cipherSuites := strings.Join(libgocrypto.OpenSSLToIANACipherSuites(ciphers), ",")
	args = setArg(args, tlsCipherSuitesArg, cipherSuites)
	args = setArg(args, minTLSversionArg, string(tlsProfileSpec.MinTLSVersion))
	return args, nil
}

func SetTLSClientFunc(fn func() (client.Client, error)) {
	tlsClientFunc = fn
}

func ResetTLSState() {
	tlsProfileSpec = nil
	tlsConfig = nil
	tlsClientFunc = GetOrCreateOCPConfigCRClient
}

func setArg(args []string, argName string, argValue string) []string {
	// If flag exists, overwrite in-place. Otherwise append
	found := false
	for i, arg := range args {
		if arg == argName || (argName[len(argName)-1] == '=' && strings.HasPrefix(arg, argName)) {
			args[i] = argName + argValue
			found = true
		}
	}
	if !found {
		args = append(args, argName+argValue)
	}
	return args
}
