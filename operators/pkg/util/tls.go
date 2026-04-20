// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package util

import (
	"crypto/tls"
	"reflect"
	"sync"

	ocinfrav1 "github.com/openshift/api/config/v1"
	"github.com/openshift/library-go/pkg/crypto"
)

var (
	tlsProfileMu   sync.RWMutex
	currentProfile *ocinfrav1.TLSSecurityProfile
)

// SyncBaseTLSConfig updates the cached TLS profile for dynamic configuration.
func SyncBaseTLSConfig(profile *ocinfrav1.TLSSecurityProfile) {
	tlsProfileMu.Lock()
	defer tlsProfileMu.Unlock()

	if !reflect.DeepEqual(currentProfile, profile) {
		log.Info("TLS profile updated", "old", currentProfile, "new", profile)
		currentProfile = profile
	}
}

func normalizeProfile(profile *ocinfrav1.TLSSecurityProfile) *ocinfrav1.TLSSecurityProfile {
	if profile == nil {
		return &ocinfrav1.TLSSecurityProfile{
			Type: ocinfrav1.TLSProfileIntermediateType,
		}
	}
	return profile
}

// GetTLSProfileStringMinVersion returns the string representation of the minimum TLS version for a given profile.
func GetTLSProfileStringMinVersion(profile *ocinfrav1.TLSSecurityProfile) string {
	profile = normalizeProfile(profile)
	switch profile.Type {
	case ocinfrav1.TLSProfileOldType:
		return string(ocinfrav1.TLSProfiles[ocinfrav1.TLSProfileOldType].MinTLSVersion)
	case ocinfrav1.TLSProfileIntermediateType:
		return string(ocinfrav1.TLSProfiles[ocinfrav1.TLSProfileIntermediateType].MinTLSVersion)
	case ocinfrav1.TLSProfileModernType:
		return string(ocinfrav1.TLSProfiles[ocinfrav1.TLSProfileModernType].MinTLSVersion)
	case ocinfrav1.TLSProfileCustomType:
		if profile.Custom != nil {
			return string(profile.Custom.MinTLSVersion)
		}
	}
	return string(ocinfrav1.TLSProfiles[ocinfrav1.TLSProfileIntermediateType].MinTLSVersion)
}

// GetTLSProfileCiphers returns the cipher suites for a given profile in OpenSSL format.
// Note: TLS 1.3 ciphers are not configurable in Go, so this returns nil for the Modern profile.
func GetTLSProfileCiphers(profile *ocinfrav1.TLSSecurityProfile) []string {
	profile = normalizeProfile(profile)
	switch profile.Type {
	case ocinfrav1.TLSProfileOldType:
		return ocinfrav1.TLSProfiles[ocinfrav1.TLSProfileOldType].Ciphers
	case ocinfrav1.TLSProfileIntermediateType:
		return ocinfrav1.TLSProfiles[ocinfrav1.TLSProfileIntermediateType].Ciphers
	case ocinfrav1.TLSProfileModernType:
		// TLS 1.3 ciphers are not configurable in Go
		return nil
	case ocinfrav1.TLSProfileCustomType:
		if profile.Custom != nil {
			return profile.Custom.Ciphers
		}
	}
	return ocinfrav1.TLSProfiles[ocinfrav1.TLSProfileIntermediateType].Ciphers
}

// GetTLSProfileIANACiphers returns the cipher suites for a given profile in IANA format.
func GetTLSProfileIANACiphers(profile *ocinfrav1.TLSSecurityProfile) []string {
	ciphers := GetTLSProfileCiphers(profile)
	if len(ciphers) == 0 {
		return nil
	}
	return crypto.OpenSSLToIANACipherSuites(ciphers)
}

// GetOauthProxyTLSMinVersion maps the configv1.TLSProtocolVersion to the format expected by oauth-proxy (e.g. TLSv1.2).
func GetOauthProxyTLSMinVersion(profile *ocinfrav1.TLSSecurityProfile) string {
	minVersion := GetTLSProfileStringMinVersion(profile)
	switch minVersion {
	case string(ocinfrav1.VersionTLS10):
		return "TLSv1.0"
	case string(ocinfrav1.VersionTLS11):
		return "TLSv1.1"
	case string(ocinfrav1.VersionTLS12):
		return "TLSv1.2"
	case string(ocinfrav1.VersionTLS13):
		return "TLSv1.3"
	default:
		return "TLSv1.2"
	}
}

// GetSupportedTLSConfig returns the default TLS configuration for the given profile.
func GetSupportedTLSConfig(profile *ocinfrav1.TLSSecurityProfile) []func(*tls.Config) {
	minVersion := GetTLSProfileStringMinVersion(profile)
	ciphers := GetTLSProfileCiphers(profile)

	goMinVersion, err := crypto.TLSVersion(minVersion)
	if err != nil {
		goMinVersion = tls.VersionTLS12
	}
	var goCiphers []uint16
	if len(ciphers) > 0 {
		var anyErr bool
		ianaCiphers := crypto.OpenSSLToIANACipherSuites(ciphers)
		for _, cipher := range ianaCiphers {
			goCipher, err := crypto.CipherSuite(cipher)
			if err != nil {
				anyErr = true
			} else {
				goCiphers = append(goCiphers, goCipher)
			}
		}
		if anyErr {
			goCiphers = nil
		}
	}

	return []func(*tls.Config){
		func(t *tls.Config) {
			t.MinVersion = goMinVersion
			if len(goCiphers) > 0 {
				t.CipherSuites = goCiphers
			}
		},
	}
}
