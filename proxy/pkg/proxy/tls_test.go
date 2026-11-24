// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package proxy

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestTLSCurvePreferences(t *testing.T) {
	cases := []struct {
		name   string
		groups []string
		want   []tls.CurveID
	}{
		{"empty", nil, []tls.CurveID{}},
		{
			"supported",
			[]string{"X25519", "secp256r1", "secp384r1", "secp521r1", "X25519MLKEM768", "SecP256r1MLKEM768", "SecP384r1MLKEM1024"},
			[]tls.CurveID{tls.X25519, tls.CurveP256, tls.CurveP384, tls.CurveP521, tls.X25519MLKEM768, tls.SecP256r1MLKEM768, tls.SecP384r1MLKEM1024},
		},
		{"skips unsupported", []string{"X25519", "bogus"}, []tls.CurveID{tls.X25519}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tlsCurvePreferences(tc.groups)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("tlsCurvePreferences(%v) = %v, want %v", tc.groups, got, tc.want)
			}
		})
	}
}

// Helper to create a self-signed certificate for testing.
func createTestCert(t *testing.T, commonName string) ([]byte, []byte) {
	t.Helper()

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate private key: %v", err)
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: commonName,
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(time.Hour),
		KeyUsage:  x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
			x509.ExtKeyUsageClientAuth,
		},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("Failed to create certificate: %v", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	if certPEM == nil {
		t.Fatal("Failed to encode certificate to PEM")
	}

	privBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("Unable to marshal private key: %v", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privBytes})
	if keyPEM == nil {
		t.Fatal("Failed to encode key to PEM")
	}

	return certPEM, keyPEM
}

// Helper to write certs to a file.
func writeTestCert(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("Failed to write to %s: %v", path, err)
	}
}

func TestNewTransport_InitialLoadFails(t *testing.T) {
	tmpDir := t.TempDir()
	opts := &TLSOptions{
		CaFile:   filepath.Join(tmpDir, "non-existent-ca.crt"),
		CertFile: filepath.Join(tmpDir, "non-existent-tls.crt"),
		KeyFile:  filepath.Join(tmpDir, "non-existent-tls.key"),
	}

	_, err := NewTransport(opts)
	if err == nil {
		t.Fatal("Expected NewTransport to fail with missing files, but it succeeded")
	}
}

func TestNewTransport_AllUnsupportedGroupsFails(t *testing.T) {
	// Fail closed: a non-empty NamedGroups list with nothing usable must error rather than
	// silently falling back to Go's default curve selection.
	tmpDir := t.TempDir()
	cert, key := createTestCert(t, "server")
	writeTestCert(t, filepath.Join(tmpDir, "ca.crt"), cert)
	writeTestCert(t, filepath.Join(tmpDir, "tls.crt"), cert)
	writeTestCert(t, filepath.Join(tmpDir, "tls.key"), key)

	opts := &TLSOptions{
		CaFile:      filepath.Join(tmpDir, "ca.crt"),
		CertFile:    filepath.Join(tmpDir, "tls.crt"),
		KeyFile:     filepath.Join(tmpDir, "tls.key"),
		NamedGroups: []string{"bogus-group"},
	}

	if _, err := NewTransport(opts); err == nil {
		t.Fatal("Expected NewTransport to fail when all configured TLS groups are unsupported")
	}
}

func TestPollingReloadOnSymlinkChange(t *testing.T) {
	// 1. Setup directories to simulate Kubernetes volume mounts
	baseDir := t.TempDir()
	v1Dir := filepath.Join(baseDir, "v1")
	v2Dir := filepath.Join(baseDir, "v2")
	liveDir := filepath.Join(baseDir, "live") // The dir the transport will point to
	for _, dir := range []string{v1Dir, v2Dir, liveDir} {
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatalf("Failed to create dir %s: %v", dir, err)
		}
	}

	// 2. Create two versions of the certificates
	certV1, keyV1 := createTestCert(t, "server-v1")
	certV2, keyV2 := createTestCert(t, "server-v2")

	// 3. Write v1 certs and create initial symlinks
	writeTestCert(t, filepath.Join(v1Dir, "ca.crt"), certV1)
	writeTestCert(t, filepath.Join(v1Dir, "tls.crt"), certV1)
	writeTestCert(t, filepath.Join(v1Dir, "tls.key"), keyV1)
	writeTestCert(t, filepath.Join(v2Dir, "ca.crt"), certV2) // Write v2 ahead of time
	writeTestCert(t, filepath.Join(v2Dir, "tls.crt"), certV2)
	writeTestCert(t, filepath.Join(v2Dir, "tls.key"), keyV2)

	for _, name := range []string{"ca.crt", "tls.crt", "tls.key"} {
		if err := os.Symlink(filepath.Join(v1Dir, name), filepath.Join(liveDir, name)); err != nil {
			t.Fatalf("Failed to create symlink for %s: %v", name, err)
		}
	}

	// 4. Create and start the transport with a fast polling interval
	opts := &TLSOptions{
		CaFile:          filepath.Join(liveDir, "ca.crt"),
		CertFile:        filepath.Join(liveDir, "tls.crt"),
		KeyFile:         filepath.Join(liveDir, "tls.key"),
		PollingInterval: 50 * time.Millisecond,
	}
	transport, err := NewTransport(opts)
	if err != nil {
		t.Fatalf("Failed to create transport: %v", err)
	}
	defer transport.Close()

	// 5. Verify initial load is v1
	getCommonName := func(tr *reloadingTransport) (string, error) {
		tr.mutex.RLock()
		defer tr.mutex.RUnlock()
		if tr.Transport.TLSClientConfig == nil || len(tr.Transport.TLSClientConfig.Certificates) == 0 {
			return "", fmt.Errorf("transport has no certificates")
		}
		cert, err := x509.ParseCertificate(tr.Transport.TLSClientConfig.Certificates[0].Certificate[0])
		if err != nil {
			return "", err
		}
		return cert.Subject.CommonName, nil
	}

	cn, err := getCommonName(transport)
	if err != nil {
		t.Fatalf("Failed to get initial common name: %v", err)
	}
	if cn != "server-v1" {
		t.Fatalf(`Expected initial common name to be "server-v1", but got %q`, cn)
	}

	// 6. Simulate the Kubernetes secret update by atomically swapping the symlinks
	for _, name := range []string{"ca.crt", "tls.crt", "tls.key"} {
		// Use rename as an atomic operation to replace the symlink
		newLink := filepath.Join(liveDir, name)
		oldLink := newLink + ".old"
		if err := os.Symlink(filepath.Join(v2Dir, name), oldLink); err != nil {
			t.Fatalf("Failed to create new temp symlink for %s: %v", name, err)
		}
		if err := os.Rename(oldLink, newLink); err != nil {
			t.Fatalf("Failed to atomically replace symlink for %s: %v", name, err)
		}
	}

	// 7. Wait for polling to detect the change and verify the new cert
	time.Sleep(150 * time.Millisecond) // Wait for at least two polling cycles

	cn, err = getCommonName(transport)
	if err != nil {
		t.Fatalf("Failed to get reloaded common name: %v", err)
	}
	if cn != "server-v2" {
		t.Fatalf(`Expected reloaded common name to be "server-v2", but got %q`, cn)
	}

	// 8. Test that nothing changes if we check again
	time.Sleep(150 * time.Millisecond)
	cn, err = getCommonName(transport)
	if err != nil {
		t.Fatalf("Failed to get common name after second wait: %v", err)
	}
	if cn != "server-v2" {
		t.Fatalf(`Expected common name to still be "server-v2", but got %q`, cn)
	}
}
