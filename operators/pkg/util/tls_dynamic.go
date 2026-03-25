// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package util

import (
	"crypto/tls"
)

// GetDynamicTLSConfig returns a TLS configuration function that dynamically reads
// the latest TLS profile without requiring a pod restart.
func GetDynamicTLSConfig() []func(*tls.Config) {
	return []func(*tls.Config){
		func(t *tls.Config) {
			t.GetConfigForClient = func(hi *tls.ClientHelloInfo) (*tls.Config, error) {
				tlsProfileMu.RLock()
				profile := currentProfile
				tlsProfileMu.RUnlock()

				cfg := t.Clone()
				cfg.GetConfigForClient = nil

				// Re-apply the GetSupportedTLSConfig onto the cloned config
				for _, f := range GetSupportedTLSConfig(profile) {
					f(cfg)
				}

				return cfg, nil
			}
		},
	}
}
