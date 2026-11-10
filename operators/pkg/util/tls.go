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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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

	baseConfig, unsupportedCiphers := tlsutil.NewTLSConfigFromProfile(*profileSpec)
	if len(unsupportedCiphers) > 0 {
		log.Info("TLS configuration contains unsupported ciphers that will be ignored", "ciphers", unsupportedCiphers)
	}

	// NewTLSConfigFromProfile only applies MinTLSVersion and ciphers; it does not yet handle
	// the TLSSecurityProfile.Groups field. Layer the configured named groups (curve preferences)
	// on top so in-process TLS servers honor them. CurvePreferences applies to both TLS 1.2 and 1.3.
	curves := convertTLSGroups(profileSpec.Groups)
	tlsConfig = func(c *tls.Config) {
		baseConfig(c)
		if len(curves) > 0 {
			c.CurvePreferences = curves
		}
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

// SetTLSGroups appends or overwrites the given TLS named-groups flag on a container args slice
// based on the cluster TLS profile's Groups field. The flag value is the comma-separated list of
// group names (IANA "TLS Supported Groups" registry form, e.g. "X25519,secp256r1"). It is a no-op
// when the profile configures no groups. Only pass a groups flag for binaries that support it.
func SetTLSGroups(ctx context.Context, args []string, tlsGroupsArg string) ([]string, error) {
	tlsProfileSpec, err := GetOrCreateTLSProfileSpec(ctx)
	if err != nil {
		log.Error(err, "unable to get TLS security configuration")
		return nil, err
	}

	if groups := tlsGroupNames(tlsProfileSpec.Groups); len(groups) > 0 {
		args = setArg(args, tlsGroupsArg, strings.Join(groups, ","))
	}
	return args, nil
}

// SetTLSCurvePreferences appends or overwrites a numeric curve-preferences flag on a container args
// slice based on the cluster TLS profile's Groups field. The value is the comma-separated list of
// numeric crypto/tls CurveID values (e.g. "29,23"). This is the format kube-rbac-proxy expects for
// its --tls-curve-preferences flag (as opposed to the IANA group names used by SetTLSGroups). It is
// a no-op when the profile configures no supported groups.
func SetTLSCurvePreferences(ctx context.Context, args []string, tlsCurvesArg string) ([]string, error) {
	tlsProfileSpec, err := GetOrCreateTLSProfileSpec(ctx)
	if err != nil {
		log.Error(err, "unable to get TLS security configuration")
		return nil, err
	}

	curves := convertTLSGroups(tlsProfileSpec.Groups)
	if len(curves) > 0 {
		nums := make([]string, len(curves))
		for i, c := range curves {
			nums[i] = strconv.Itoa(int(c))
		}
		args = setArg(args, tlsCurvesArg, strings.Join(nums, ","))
	}
	return args, nil
}

// tlsGroupToCurveID maps OpenShift TLSGroup names (IANA "TLS Supported Groups" registry names)
// to crypto/tls CurveIDs. Groups with no corresponding crypto/tls constant in the current Go
// runtime are omitted and treated as unsupported.
//
// TODO: once MCO moves to k8s >= 0.36 / the newer library-go, replace this local map with the
// shared library-go helpers crypto.TLSGroupsToCurveIDs / crypto.TLSGroupsToCurvePreferences —
// the same helpers LSO (STOR-3177) and the CSI operators use — instead of re-implementing it.
// Those helpers only exist in library-go revisions that require k8s >= 0.36, which this repo
// deliberately avoids (ACM-49950 pins openshift/api to the k8s-0.35 line).
var tlsGroupToCurveID = map[ocinfrav1.TLSGroup]tls.CurveID{
	ocinfrav1.TLSGroupX25519:             tls.X25519,
	ocinfrav1.TLSGroupSecP256r1:          tls.CurveP256,
	ocinfrav1.TLSGroupSecP384r1:          tls.CurveP384,
	ocinfrav1.TLSGroupSecP521r1:          tls.CurveP521,
	ocinfrav1.TLSGroupX25519MLKEM768:     tls.X25519MLKEM768,
	ocinfrav1.TLSGroupSecP256r1MLKEM768:  tls.SecP256r1MLKEM768,
	ocinfrav1.TLSGroupSecP384r1MLKEM1024: tls.SecP384r1MLKEM1024,
}

// tlsGroupsToCurveIDs converts OpenShift TLSGroup values to crypto/tls CurveIDs (in order),
// returning the recognized curves and any group names this runtime does not support.
func tlsGroupsToCurveIDs(groups []ocinfrav1.TLSGroup) (curves []tls.CurveID, unrecognized []ocinfrav1.TLSGroup) {
	for _, g := range groups {
		if id, ok := tlsGroupToCurveID[g]; ok {
			curves = append(curves, id)
		} else {
			unrecognized = append(unrecognized, g)
		}
	}
	return curves, unrecognized
}

// convertTLSGroups converts OpenShift TLSGroup values to crypto/tls CurveIDs for use as
// tls.Config.CurvePreferences. Groups the current Go runtime does not recognize are logged and
// skipped rather than failing.
func convertTLSGroups(groups []ocinfrav1.TLSGroup) []tls.CurveID {
	curves, unrecognized := tlsGroupsToCurveIDs(groups)
	if len(unrecognized) > 0 {
		log.Info("ignoring TLS groups not supported by this build", "groups", unrecognized)
	}
	return curves
}

// tlsGroupNames returns the supported group names as strings, preserving order. Groups with no
// crypto/tls equivalent are logged and skipped so downstream binaries are not handed a value their
// Go runtime cannot apply.
func tlsGroupNames(groups []ocinfrav1.TLSGroup) []string {
	names := make([]string, 0, len(groups))
	var unrecognized []ocinfrav1.TLSGroup
	for _, g := range groups {
		if _, ok := tlsGroupToCurveID[g]; ok {
			names = append(names, string(g))
			continue
		}
		unrecognized = append(unrecognized, g)
	}
	if len(unrecognized) > 0 {
		log.Info("ignoring TLS groups not supported by this build", "groups", unrecognized)
	}
	return names
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
	// Normalize for consumers (like observatorium-api) that only accept pre-Go 1.22 cipher names.
	for i, c := range ianaCiphers {
		ianaCiphers[i] = normalizeCipherName(c)
	}
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
			filtered = append(filtered, c)
		}
	}
	return filtered
}

// normalizeCipherName strips the _SHA256 suffix that Go 1.22+ appends to
// CHACHA20_POLY1305 cipher names. Some consumers (like observatorium-api)
// only accept the pre-Go 1.22 short names.
func normalizeCipherName(name string) string {
	if strings.HasSuffix(name, "CHACHA20_POLY1305_SHA256") {
		return strings.TrimSuffix(name, "_SHA256")
	}
	return name
}

// IsOAuthProxyTLSSupported checks whether the cluster's oauth-proxy supports
// --tls-cipher-suites and --tls-min-version flags. These flags were added in
// OCP 5.0; older versions will crash if they receive unrecognized flags.
// Returns (false, nil) on non-OCP clusters where ClusterVersion does not exist.
// Returns an error for transient failures so the caller can retry.
func IsOAuthProxyTLSSupported(ctx context.Context, c client.Client) (bool, error) {
	cv := &ocinfrav1.ClusterVersion{}
	if err := c.Get(ctx, types.NamespacedName{Name: "version"}, cv); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("unable to get ClusterVersion: %w", err)
	}

	version := ""
	if cv.Status.Desired.Version != "" {
		version = cv.Status.Desired.Version
	} else if len(cv.Status.History) > 0 {
		version = cv.Status.History[0].Version
	}
	if version == "" {
		log.Info("ClusterVersion has no version, skipping oauth-proxy TLS flags")
		return false, nil
	}

	major, _, ok := parseMajorMinor(version)
	if !ok {
		log.Info("unable to parse ClusterVersion", "version", version)
		return false, nil
	}

	return major >= 5, nil
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
