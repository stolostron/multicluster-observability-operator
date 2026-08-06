// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package util

import (
	"context"
	"crypto/tls"
	"strings"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newTestScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	configv1.AddToScheme(scheme)
	return scheme
}

func newAPIServerWithProfile(profile *configv1.TLSSecurityProfile, adherence configv1.TLSAdherencePolicy) *configv1.APIServer {
	return &configv1.APIServer{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Spec: configv1.APIServerSpec{
			TLSSecurityProfile: profile,
			TLSAdherence:       adherence,
		},
	}
}

func setFakeClient(objs ...client.Object) {
	c := fake.NewClientBuilder().WithScheme(newTestScheme()).WithObjects(objs...).Build()
	tlsClientFunc = func() (client.Client, error) {
		return c, nil
	}
}

func TestGetOrCreateTLSProfileSpec(t *testing.T) {
	strict := configv1.TLSAdherencePolicyStrictAllComponents

	customSpec := configv1.TLSProfileSpec{
		Ciphers:       []string{"ECDHE-ECDSA-CHACHA20-POLY1305"},
		MinTLSVersion: configv1.VersionTLS13,
	}

	intermediateSpec := configv1.TLSProfiles[configv1.TLSProfileIntermediateType]

	tests := []struct {
		name          string
		tlsSecProfile *configv1.TLSSecurityProfile
		adherence     configv1.TLSAdherencePolicy
		wantedSpec    *configv1.TLSProfileSpec
	}{
		{
			name: "strict + intermediate profile",
			tlsSecProfile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileIntermediateType,
			},
			adherence:  strict,
			wantedSpec: configv1.TLSProfiles[configv1.TLSProfileIntermediateType],
		},
		{
			name: "strict + modern profile",
			tlsSecProfile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileModernType,
			},
			adherence:  strict,
			wantedSpec: configv1.TLSProfiles[configv1.TLSProfileModernType],
		},
		{
			name: "strict + old profile",
			tlsSecProfile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileOldType,
			},
			adherence:  strict,
			wantedSpec: configv1.TLSProfiles[configv1.TLSProfileOldType],
		},
		{
			name: "strict + custom profile",
			tlsSecProfile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{
					TLSProfileSpec: customSpec,
				},
			},
			adherence:  strict,
			wantedSpec: &customSpec,
		},
		{
			name:          "strict + nil profile",
			tlsSecProfile: nil,
			adherence:     strict,
			wantedSpec:    intermediateSpec,
		},
		{
			name: "NoOpinion adherence ignores cluster profile",
			tlsSecProfile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileModernType,
			},
			adherence:  configv1.TLSAdherencePolicyNoOpinion,
			wantedSpec: intermediateSpec,
		},
		{
			name: "LegacyAdheringComponentsOnly adherence ignores cluster profile",
			tlsSecProfile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileModernType,
			},
			adherence:  configv1.TLSAdherencePolicyLegacyAdheringComponentsOnly,
			wantedSpec: intermediateSpec,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer ResetTLSState()
			setFakeClient(newAPIServerWithProfile(tt.tlsSecProfile, tt.adherence))
			spec, err := GetOrCreateTLSProfileSpec(context.Background())
			require.NoError(t, err)
			assert.Equal(t, *tt.wantedSpec, *spec)
		})
	}
}

func TestFetchTLSAdherencePolicy(t *testing.T) {
	tests := []struct {
		name       string
		adherence  configv1.TLSAdherencePolicy
		wantPolicy configv1.TLSAdherencePolicy
	}{
		{
			name:       "returns StrictAllComponents",
			adherence:  configv1.TLSAdherencePolicyStrictAllComponents,
			wantPolicy: configv1.TLSAdherencePolicyStrictAllComponents,
		},
		{
			name:       "returns NoOpinion when empty",
			adherence:  configv1.TLSAdherencePolicyNoOpinion,
			wantPolicy: configv1.TLSAdherencePolicyNoOpinion,
		},
		{
			name:       "returns LegacyAdheringComponentsOnly",
			adherence:  configv1.TLSAdherencePolicyLegacyAdheringComponentsOnly,
			wantPolicy: configv1.TLSAdherencePolicyLegacyAdheringComponentsOnly,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer ResetTLSState()
			setFakeClient(newAPIServerWithProfile(nil, tt.adherence))
			policy, err := FetchTLSAdherencePolicy(context.Background())
			require.NoError(t, err)
			assert.Equal(t, tt.wantPolicy, policy)
		})
	}
}

func TestFetchTLSAdherencePolicy_NotFound(t *testing.T) {
	defer ResetTLSState()
	setFakeClient()

	_, err := FetchTLSAdherencePolicy(context.Background())
	require.Error(t, err, "should return error when APIServer is not found")
}

func TestGetOrCreateTLSProfileSpec_NotFound(t *testing.T) {
	defer ResetTLSState()
	setFakeClient()

	spec, err := GetOrCreateTLSProfileSpec(context.Background())
	require.NoError(t, err, "should fall back to defaults when APIServer is not found")
	assert.Equal(t, *configv1.TLSProfiles[configv1.TLSProfileIntermediateType], *spec)
}

func TestGetOrCreateTLSProfileSpec_Caching(t *testing.T) {
	defer ResetTLSState()
	setFakeClient(newAPIServerWithProfile(
		&configv1.TLSSecurityProfile{Type: configv1.TLSProfileModernType},
		configv1.TLSAdherencePolicyStrictAllComponents,
	))

	spec1, err := GetOrCreateTLSProfileSpec(context.Background())
	require.NoError(t, err)

	tlsClientFunc = func() (client.Client, error) {
		return nil, assert.AnError
	}

	spec2, err := GetOrCreateTLSProfileSpec(context.Background())
	require.NoError(t, err)
	assert.Same(t, spec1, spec2, "should return cached spec")
}

func TestGetOrCreateTLSConfig(t *testing.T) {
	strict := configv1.TLSAdherencePolicyStrictAllComponents

	tests := []struct {
		name             string
		tlsSecProfile    *configv1.TLSSecurityProfile
		adherence        configv1.TLSAdherencePolicy
		wantedMinVersion uint16
		wantCiphers      bool
	}{
		{
			name: "strict + intermediate profile",
			tlsSecProfile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileIntermediateType,
			},
			adherence:        strict,
			wantedMinVersion: tls.VersionTLS12,
			wantCiphers:      true,
		},
		{
			name: "strict + modern profile",
			tlsSecProfile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileModernType,
			},
			adherence:        strict,
			wantedMinVersion: tls.VersionTLS13,
			wantCiphers:      false,
		},
		{
			name: "strict + old profile",
			tlsSecProfile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileOldType,
			},
			adherence:        strict,
			wantedMinVersion: tls.VersionTLS10,
			wantCiphers:      true,
		},
		{
			name:             "nil profile defaults to intermediate",
			tlsSecProfile:    nil,
			adherence:        strict,
			wantedMinVersion: tls.VersionTLS12,
			wantCiphers:      true,
		},
		{
			name: "NoOpinion adherence defaults to intermediate",
			tlsSecProfile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileModernType,
			},
			adherence:        configv1.TLSAdherencePolicyNoOpinion,
			wantedMinVersion: tls.VersionTLS12,
			wantCiphers:      true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer ResetTLSState()
			setFakeClient(newAPIServerWithProfile(tt.tlsSecProfile, tt.adherence))

			tlsCfgFn, err := GetOrCreateTLSConfig(context.Background())
			require.NoError(t, err)
			require.NotNil(t, tlsCfgFn)

			cfg := &tls.Config{}
			tlsCfgFn(cfg)

			assert.Equal(t, tt.wantedMinVersion, cfg.MinVersion)
			if tt.wantCiphers {
				assert.NotEmpty(t, cfg.CipherSuites)
			} else {
				assert.Nil(t, cfg.CipherSuites)
			}
		})
	}
}

func TestGetOrCreateTLSConfig_NotFound(t *testing.T) {
	defer ResetTLSState()
	setFakeClient()

	tlsCfgFn, err := GetOrCreateTLSConfig(context.Background())
	require.NoError(t, err, "should fall back to defaults when APIServer is not found")
	require.NotNil(t, tlsCfgFn)

	cfg := &tls.Config{}
	tlsCfgFn(cfg)
	assert.Equal(t, uint16(tls.VersionTLS12), cfg.MinVersion)
}

func TestGetOrCreateTLSConfig_Caching(t *testing.T) {
	defer ResetTLSState()
	setFakeClient(newAPIServerWithProfile(
		&configv1.TLSSecurityProfile{Type: configv1.TLSProfileIntermediateType},
		configv1.TLSAdherencePolicyStrictAllComponents,
	))

	fn1, err := GetOrCreateTLSConfig(context.Background())
	require.NoError(t, err)

	tlsClientFunc = func() (client.Client, error) {
		return nil, assert.AnError
	}

	fn2, err := GetOrCreateTLSConfig(context.Background())
	require.NoError(t, err)

	cfg1 := &tls.Config{}
	cfg2 := &tls.Config{}
	fn1(cfg1)
	fn2(cfg2)
	assert.Equal(t, cfg1.MinVersion, cfg2.MinVersion)
	assert.Equal(t, cfg1.CipherSuites, cfg2.CipherSuites)
}

func TestSetTLSSecurityConfiguration(t *testing.T) {
	strict := configv1.TLSAdherencePolicyStrictAllComponents

	tests := []struct {
		name          string
		tlsSecProfile *configv1.TLSSecurityProfile
		adherence     configv1.TLSAdherencePolicy
		initialArgs   []string
		wantVersion   string
		wantCiphers   bool
	}{
		{
			name: "strict + intermediate appends TLS args",
			tlsSecProfile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileIntermediateType,
			},
			adherence:   strict,
			initialArgs: []string{"--foo=bar"},
			wantVersion: string(configv1.VersionTLS12),
			wantCiphers: true,
		},
		{
			name: "strict + modern appends TLS 1.3",
			tlsSecProfile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileModernType,
			},
			adherence:   strict,
			initialArgs: []string{"--foo=bar"},
			wantVersion: string(configv1.VersionTLS13),
			wantCiphers: false,
		},
		{
			name: "NoOpinion adherence defaults to intermediate",
			tlsSecProfile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileModernType,
			},
			adherence:   configv1.TLSAdherencePolicyNoOpinion,
			initialArgs: []string{"--foo=bar"},
			wantVersion: string(configv1.VersionTLS12),
			wantCiphers: true,
		},
		{
			name: "overwrites existing TLS args",
			tlsSecProfile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileIntermediateType,
			},
			adherence:   strict,
			initialArgs: []string{"--foo=bar", "--tls-cipher-suites=old", "--tls-min-version=old"},
			wantVersion: string(configv1.VersionTLS12),
			wantCiphers: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer ResetTLSState()
			setFakeClient(newAPIServerWithProfile(tt.tlsSecProfile, tt.adherence))

			result, err := SetTLSSecurityConfiguration(context.Background(), tt.initialArgs, "--tls-cipher-suites=", "--tls-min-version=")
			require.NoError(t, err)

			assert.Contains(t, result[0], "--foo=bar")

			var foundVersion, foundCiphers bool
			for _, arg := range result {
				if strings.Contains(arg, "--tls-min-version=") && len(arg) > len("--tls-min-version=") {
					assert.Equal(t, "--tls-min-version="+tt.wantVersion, arg)
					foundVersion = true
				}
				if strings.Contains(arg, "--tls-cipher-suites=") && len(arg) > len("--tls-cipher-suites=") {
					foundCiphers = true
					if tt.wantCiphers {
						assert.NotEqual(t, "--tls-cipher-suites=", arg, "cipher suites should not be empty")
					}
				}
			}
			assert.True(t, foundVersion, "should contain --tls-min-version flag")
			assert.True(t, foundCiphers, "should contain --tls-cipher-suites flag")
		})
	}
}
