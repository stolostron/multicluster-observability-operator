// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package util

import (
	"context"
	"crypto/tls"
	"fmt"
	"strconv"
	"strings"

	ocinfrav1 "github.com/openshift/api/config/v1"
	tlsutil "github.com/openshift/controller-runtime-common/pkg/tls"
	libgocrypto "github.com/openshift/library-go/pkg/crypto"
	"k8s.io/apimachinery/pkg/types"
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

	ianaCiphers := filterConfigurableCiphers(libgocrypto.OpenSSLToIANACipherSuites(tlsProfileSpec.Ciphers))
	if len(ianaCiphers) > 0 {
		args = setArg(args, tlsCipherSuitesArg, strings.Join(ianaCiphers, ","))
	}
	args = setArg(args, minTLSversionArg, string(tlsProfileSpec.MinTLSVersion))
	return args, nil
}

// FetchTLSAdherencePolicy retrieves the current TLS adherence policy from the
// OCP APIServer resource. Unlike GetOrCreateTLSProfileSpec, the result is NOT
// cached because the policy can change at runtime and the SecurityProfileWatcher
// callback needs the live value.
func FetchTLSAdherencePolicy(ctx context.Context) (ocinfrav1.TLSAdherencePolicy, error) {
	c, err := tlsClientFunc()
	if err != nil {
		return "", fmt.Errorf("unable to create client for API server: %w", err)
	}

	tap, err := tlsutil.FetchAPIServerTLSAdherencePolicy(ctx, c)
	if err != nil {
		return "", fmt.Errorf("unable to get TLS adherence policy: %w", err)
	}

	return tap, nil
}

func GetTLSSecurityConfiguration(ctx context.Context) (minTLSVersion, cipherSuites string, err error) {
	tlsProfileSpec, err := GetOrCreateTLSProfileSpec(ctx)
	if err != nil {
		log.Error(err, "unable to get TLS security configuration")
		return "", "", err
	}

	ianaCiphers := filterConfigurableCiphers(libgocrypto.OpenSSLToIANACipherSuites(tlsProfileSpec.Ciphers))
	cipherSuites = strings.Join(ianaCiphers, ",")
	minTLSVersion = string(tlsProfileSpec.MinTLSVersion)
	return
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

func filterConfigurableCiphers(ianaCiphers []string) []string {
	configurable := make(map[string]struct{})
	for _, cs := range append(tls.CipherSuites(), tls.InsecureCipherSuites()...) {
		tls13Only := true
		for _, v := range cs.SupportedVersions {
			if v != tls.VersionTLS13 {
				tls13Only = false
				break
			}
		}
		if !tls13Only {
			configurable[cs.Name] = struct{}{}
		}
	}

	var filtered []string
	for _, c := range ianaCiphers {
		if _, ok := configurable[c]; ok {
			filtered = append(filtered, normalizeCipherName(c))
		}
	}
	return filtered
}

// IsOAuthProxyTLSSupported checks whether the cluster's oauth-proxy supports
// --tls-cipher-suites and --tls-min-version flags. These flags were added in
// OCP 5.0; older versions will crash if they receive unrecognized flags.
func IsOAuthProxyTLSSupported(ctx context.Context, c client.Client) bool {
	cv := &ocinfrav1.ClusterVersion{}
	if err := c.Get(ctx, types.NamespacedName{Name: "version"}, cv); err != nil {
		log.Error(err, "unable to get ClusterVersion, skipping oauth-proxy TLS flags")
		return false
	}

	version := ""
	if cv.Status.Desired.Version != "" {
		version = cv.Status.Desired.Version
	} else if len(cv.Status.History) > 0 {
		version = cv.Status.History[0].Version
	}
	if version == "" {
		log.Info("ClusterVersion has no version, skipping oauth-proxy TLS flags")
		return false
	}

	major, _, ok := parseMajorMinor(version)
	if !ok {
		log.Info("unable to parse ClusterVersion", "version", version)
		return false
	}

	return major >= 5
}

func parseMajorMinor(version string) (major, minor int, ok bool) {
	parts := strings.SplitN(version, ".", 3)
	if len(parts) < 2 {
		return 0, 0, false
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, false
	}
	minor, err = strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, false
	}
	return major, minor, true
}

// normalizeCipherName strips the _SHA256 suffix that Go 1.22+ appends to
// CHACHA20_POLY1305 cipher names so they match the shorter form accepted by
// k8s.io/component-base/cli/flag.TLSCipherSuites() across all versions.
func normalizeCipherName(name string) string {
	if strings.HasSuffix(name, "CHACHA20_POLY1305_SHA256") {
		return strings.TrimSuffix(name, "_SHA256")
	}
	return name
}
