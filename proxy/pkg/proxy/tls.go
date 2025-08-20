// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package proxy

import (
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"time"

	"k8s.io/klog"
)

const (
	caPath   = "/var/rbac_proxy/ca"
	certPath = "/var/rbac_proxy/certs"
)

// TLSOptions holds the paths to the TLS assets.
type TLSOptions struct {
	CaFile   string
	KeyFile  string
	CertFile string
}

func GetTLSTransport() (*http.Transport, error) {
	return getTLSTransportWithOptions(&TLSOptions{
		CaFile:   path.Join(caPath, "ca.crt"),
		KeyFile:  path.Join(certPath, "tls.key"),
		CertFile: path.Join(certPath, "tls.crt"),
	})
}

func getTLSTransportWithOptions(opts *TLSOptions) (*http.Transport, error) {
	// Load Server CA cert
	caCert, err := os.ReadFile(filepath.Clean(opts.CaFile))
	if err != nil {
		klog.Error("failed to load server ca cert file")
		return nil, err
	}
	// Load client cert signed by Client CA
	cert, err := tls.LoadX509KeyPair(filepath.Clean(opts.CertFile), filepath.Clean(opts.KeyFile))
	if err != nil {
		klog.Error("failed to load client cert/key")
		return nil, err
	}

	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)
	// Setup HTTPS client
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caCertPool,
		MinVersion:   tls.VersionTLS12,
	}
	return &http.Transport{
		Dial: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 300 * time.Second,
		}).Dial,
		TLSHandshakeTimeout:   30 * time.Second,
		ResponseHeaderTimeout: 300 * time.Second,
		DisableKeepAlives:     true,
		TLSClientConfig:       tlsConfig,
	}, nil
}
