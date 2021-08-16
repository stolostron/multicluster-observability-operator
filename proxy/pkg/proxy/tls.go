// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package proxy

import (
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"net"
	"net/http"
	"path"
	"path/filepath"
	"time"

	"k8s.io/klog"
)

const (
	caPath   = "/var/rbac_proxy/ca"
	certPath = "/var/rbac_proxy/certs"
)

func getTLSTransport() (*http.Transport, error) {

	caCertFile := path.Join(caPath, "ca.crt")
	tlsKeyFile := path.Join(certPath, "tls.key")
	tlsCrtFile := path.Join(certPath, "tls.crt")

	// Load Server CA cert
	caCert, err := ioutil.ReadFile(filepath.Clean(caCertFile))
	if err != nil {
		klog.Error("failed to load server ca cert file")
		return nil, err
	}
	// Load client cert signed by Client CA
	cert, err := tls.LoadX509KeyPair(filepath.Clean(tlsCrtFile), filepath.Clean(tlsKeyFile))
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
