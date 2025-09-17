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
	"path/filepath"
	"reflect"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	"k8s.io/klog"
)

// TLSOptions holds the paths to the TLS assets and the polling configuration.
type TLSOptions struct {
	CaFile   string
	KeyFile  string
	CertFile string

	// PollingInterval specifies how often to check for certificate changes.
	PollingInterval time.Duration
}

// reloadingTransport wraps http.Transport to allow for safe, concurrent reloading of TLS configuration.
type reloadingTransport struct {
	*http.Transport
	tlsConfig *tls.Config
	opts      *TLSOptions
	mutex     sync.RWMutex

	// Store the real paths to detect symlink changes
	realPaths map[string]string
	closeOnce sync.Once
	closed    chan struct{}
}

// newReloadingTransport creates a new transport that can reload its TLS configuration.
func newReloadingTransport(opts *TLSOptions) (*reloadingTransport, error) {
	transport := &reloadingTransport{
		opts:      opts,
		realPaths: make(map[string]string),
		closed:    make(chan struct{}),
		Transport: &http.Transport{
			Dial: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 300 * time.Second,
			}).Dial,
			TLSHandshakeTimeout:   30 * time.Second,
			ResponseHeaderTimeout: 120 * time.Second,
		},
	}

	// Perform initial load of certs and their real paths.
	if err := transport.reloadTLSConfig(); err != nil {
		return nil, err
	}
	if err := transport.updateRealPaths(); err != nil {
		return nil, err
	}

	transport.TLSClientConfig = transport.tlsConfig

	// Start the background polling goroutine.
	go transport.pollingLoop()

	return transport, nil
}

// Close stops the transport's polling goroutine to prevent leaks.
func (t *reloadingTransport) Close() {
	t.closeOnce.Do(func() {
		close(t.closed)
	})
}

// pollingLoop is the main loop that periodically checks for certificate changes.
func (t *reloadingTransport) pollingLoop() {
	ticker := time.NewTicker(t.opts.PollingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			t.checkForReload()
		case <-t.closed:
			klog.Info("Certificate polling loop shutting down.")
			return
		}
	}
}

// updateRealPaths resolves the symlinks for all certificate files and stores them.
func (t *reloadingTransport) updateRealPaths() error {
	filesToWatch := []string{t.opts.CaFile, t.opts.CertFile, t.opts.KeyFile}
	for _, file := range filesToWatch {
		realPath, err := filepath.EvalSymlinks(file)
		if err != nil {
			return err
		}
		t.realPaths[file] = realPath
	}
	return nil
}

// checkForReload checks if the underlying certificate files have changed and triggers a reload if they have.
func (t *reloadingTransport) checkForReload() {
	// Resiliently resolve symlinks, treating a non-existent file as a change.
	resolve := func(path string) (string, error) {
		realPath, err := filepath.EvalSymlinks(path)
		if err != nil {
			if os.IsNotExist(err) {
				return "", nil // A deleted file is a valid change.
			}
			return "", err // Propagate other errors.
		}
		return realPath, nil
	}

	newRealPaths := make(map[string]string)
	filesToWatch := []string{t.opts.CaFile, t.opts.CertFile, t.opts.KeyFile}
	for _, file := range filesToWatch {
		newPath, err := resolve(file)
		if err != nil {
			klog.Errorf("Error resolving symlink for file %s: %v", file, err)
			return
		}
		newRealPaths[file] = newPath
	}

	if !reflect.DeepEqual(t.realPaths, newRealPaths) {
		klog.Info("Certificate change detected, attempting to reload...")

		b := backoff.NewExponentialBackOff()
		b.MaxElapsedTime = 30 * time.Second
		b.InitialInterval = 200 * time.Millisecond

		reloadOperation := func() error {
			err := t.reloadTLSConfig()
			if err != nil {
				klog.Warningf("Reload callback failed: %v. Retrying...", err)
			}
			return err
		}

		if err := backoff.Retry(reloadOperation, b); err != nil {
			klog.Errorf("Failed to reload TLS config after multiple retries: %v", err)
			return
		}

		klog.Info("Successfully reloaded TLS config")
		// Update the stored paths only after a successful reload.
		t.realPaths = newRealPaths
	}
}

// reloadTLSConfig reloads the TLS configuration from the file paths.
// It performs file I/O before locking the mutex to minimize contention.
func (t *reloadingTransport) reloadTLSConfig() error {
	// Read and parse all certificate files before acquiring the lock.
	caCert, err := os.ReadFile(filepath.Clean(t.opts.CaFile))
	if err != nil {
		return err
	}

	cert, err := tls.LoadX509KeyPair(filepath.Clean(t.opts.CertFile), filepath.Clean(t.opts.KeyFile))
	if err != nil {
		return err
	}

	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	newTLSConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caCertPool,
		MinVersion:   tls.VersionTLS12,
	}

	t.mutex.Lock()
	defer t.mutex.Unlock()

	t.tlsConfig = newTLSConfig
	t.Transport.TLSClientConfig = t.tlsConfig

	return nil
}

// NewTransport returns a closable http.RoundTripper that automatically reloads TLS configuration via polling.
// The caller is responsible for calling Close() on the returned transport to prevent goroutine leaks.
func NewTransport(opts *TLSOptions) (*reloadingTransport, error) {
	if opts.PollingInterval == 0 {
		opts.PollingInterval = 30 * time.Second
	}

	transport, err := newReloadingTransport(opts)
	if err != nil {
		return nil, err
	}
	return transport, nil
}
