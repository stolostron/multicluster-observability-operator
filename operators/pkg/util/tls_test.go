package util

import (
	"crypto/tls"
	"testing"

	ocinfrav1 "github.com/openshift/api/config/v1"
	"github.com/stretchr/testify/assert"
)

func TestGetTLSProfileStringMinVersion(t *testing.T) {
	tests := []struct {
		name     string
		profile  *ocinfrav1.TLSSecurityProfile
		expected string
	}{
		{
			name:     "nil profile",
			profile:  nil,
			expected: string(ocinfrav1.TLSProfiles[ocinfrav1.TLSProfileIntermediateType].MinTLSVersion),
		},
		{
			name: "custom profile with nil custom spec",
			profile: &ocinfrav1.TLSSecurityProfile{
				Type: ocinfrav1.TLSProfileCustomType,
			},
			expected: string(ocinfrav1.TLSProfiles[ocinfrav1.TLSProfileIntermediateType].MinTLSVersion),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, GetTLSProfileStringMinVersion(tt.profile))
		})
	}
}

func TestGetTLSProfileCiphers(t *testing.T) {
	tests := []struct {
		name     string
		profile  *ocinfrav1.TLSSecurityProfile
		expected []string
	}{
		{
			name:     "nil profile",
			profile:  nil,
			expected: ocinfrav1.TLSProfiles[ocinfrav1.TLSProfileIntermediateType].Ciphers,
		},
		{
			name: "custom profile with nil custom spec",
			profile: &ocinfrav1.TLSSecurityProfile{
				Type: ocinfrav1.TLSProfileCustomType,
			},
			expected: ocinfrav1.TLSProfiles[ocinfrav1.TLSProfileIntermediateType].Ciphers,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, GetTLSProfileCiphers(tt.profile))
		})
	}
}

func TestGetSupportedTLSConfig(t *testing.T) {
	tests := []struct {
		name            string
		profile         *ocinfrav1.TLSSecurityProfile
		expectedMinVer  uint16
		expectedCiphers int
	}{
		{
			name: "valid custom profile",
			profile: &ocinfrav1.TLSSecurityProfile{
				Type: ocinfrav1.TLSProfileCustomType,
				Custom: &ocinfrav1.CustomTLSProfile{
					TLSProfileSpec: ocinfrav1.TLSProfileSpec{
						MinTLSVersion: ocinfrav1.VersionTLS11,
						Ciphers:       []string{"AES128-SHA"}, // valid
					},
				},
			},
			expectedMinVer:  tls.VersionTLS11,
			expectedCiphers: 1, // Will map to one IANA cipher
		},
		{
			name: "invalid TLS version falls back to TLS 1.2",
			profile: &ocinfrav1.TLSSecurityProfile{
				Type: ocinfrav1.TLSProfileCustomType,
				Custom: &ocinfrav1.CustomTLSProfile{
					TLSProfileSpec: ocinfrav1.TLSProfileSpec{
						MinTLSVersion: "VersionTLS1.99", // invalid
						Ciphers:       []string{"AES128-SHA"},
					},
				},
			},
			expectedMinVer:  tls.VersionTLS12,
			expectedCiphers: 1,
		},
		{
			name: "invalid ciphers fall back to nil",
			profile: &ocinfrav1.TLSSecurityProfile{
				Type: ocinfrav1.TLSProfileCustomType,
				Custom: &ocinfrav1.CustomTLSProfile{
					TLSProfileSpec: ocinfrav1.TLSProfileSpec{
						MinTLSVersion: ocinfrav1.VersionTLS12,
						Ciphers:       []string{"INVALID-CIPHER"}, // invalid
					},
				},
			},
			expectedMinVer:  tls.VersionTLS12,
			expectedCiphers: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			funcs := GetSupportedTLSConfig(tt.profile)
			assert.Len(t, funcs, 1)

			cfg := &tls.Config{}
			funcs[0](cfg)

			assert.Equal(t, tt.expectedMinVer, cfg.MinVersion)
			assert.Len(t, cfg.CipherSuites, tt.expectedCiphers)
		})
	}
}

func TestGetDynamicTLSConfig(t *testing.T) {
	// Set a known profile
	SyncBaseTLSConfig(&ocinfrav1.TLSSecurityProfile{Type: ocinfrav1.TLSProfileModernType})

	funcs := GetDynamicTLSConfig()
	base := &tls.Config{}
	funcs[0](base)

	// Simulate a handshake
	cfg, err := base.GetConfigForClient(&tls.ClientHelloInfo{})
	assert.NoError(t, err)
	assert.Equal(t, uint16(tls.VersionTLS13), cfg.MinVersion)
	assert.Nil(t, cfg.GetConfigForClient) // recursion guard

	// Update profile and verify it takes effect without re-initialization
	SyncBaseTLSConfig(&ocinfrav1.TLSSecurityProfile{Type: ocinfrav1.TLSProfileIntermediateType})
	cfg2, err := base.GetConfigForClient(&tls.ClientHelloInfo{})
	assert.NoError(t, err)
	assert.Equal(t, uint16(tls.VersionTLS12), cfg2.MinVersion)
}
