// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package forwarder

import (
	"context"
	stdlog "log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sync"
	"testing"

	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
)

// Base64 encoded CA cert string
var customCA = "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURXVENDQWtHZ0F3SUJBZ0lVWTRHWjZPWk5uTnZySjFjNUk1RjNYZzQrRTFjd0RRWUpLb1pJaHZjTkFRRUwKQlFBd1BERUxNQWtHQTFVRUJoTUNSRVV4RHpBTkJnTlZCQWdNQm1KbGNteHBiakVQTUEwR0ExVUVCd3dHWW1WeQpiR2x1TVFzd0NRWURWUVFLREFKeWFEQWVGdzB5TXpFeU1URXhNelF6TURaYUZ3MHpNekV5TURneE16UXpNRFphCk1Ed3hDekFKQmdOVkJBWVRBa1JGTVE4d0RRWURWUVFJREFaaVpYSnNhVzR4RHpBTkJnTlZCQWNNQm1KbGNteHAKYmpFTE1Ba0dBMVVFQ2d3Q2NtZ3dnZ0VpTUEwR0NTcUdTSWIzRFFFQkFRVUFBNElCRHdBd2dnRUtBb0lCQVFDdwprNEhLV3VBOFptN0JQR2IvZEJjaGtNUFZhWGw0dzJlVHhxRG14OVhYaGVCRFZva0lKZkFGTGZ6a3YwYUd0NWV4ClprenQxc0tQVHk0NEY5ckRKSEg2dWpEODA4U1FPV0p3WFJCakI4Tk1zSjhTTVRCUm5KUE5YNTJ0akdQNjc3UEUKNWpINnc2OW9hMG9tcGVvRDk2eUM2RTZmWU9pbFl0cVF5UFdsT0MzNEQ3TnNXU1gxdnN4cmx3VTBsQXJCbWdQYQpuZURFMnQ1cU1aK1F5TXBhQi80SFh4L2NLYU5XYXJWN3FzV3ZwSE9mOGN2OUNKd1c3VkhWdjJvNUVReVI1MkcrCitOYXE4bTduSVBzaFJSMjBHMjRsR01sVUFaTjFaMkl6VjN3UExUUmZNTXRYdGtIMFVKT3pnZTQvaExSWVJBSzMKTnhZU0xJYmFscWJsa2lUTWxFbEpBZ01CQUFHalV6QlJNQjBHQTFVZERnUVdCQlNJVFZVY2s2Wmg2WTZkY2RxZwo0VHVYRjMxcjFqQWZCZ05WSFNNRUdEQVdnQlNJVFZVY2s2Wmg2WTZkY2RxZzRUdVhGMzFyMWpBUEJnTlZIUk1CCkFmOEVCVEFEQVFIL01BMEdDU3FHU0liM0RRRUJDd1VBQTRJQkFRQ0FKUWFKM2RkYVkvNVMydHU0TnNVeXNiVG8KY3BrL3YyZkxpUkthdmtiZk1kTjBFdkV6K2gwd3FqOUpQdGJjUm5Md2tlQWdmQ3Uzb29zSG4rOXc4SkFaRjJNcwpEM1FucVovaVNNVjVHSDdQTjlIK0h0M1lVQTIwWWh3QkY0RFVXYm5wS0lnL2p4NWdmVTFYZEljK2JpUWJhdHk3CmxUL0hVOVhPRmlqM3VwbWRFakgrQVlJT2QxSFh4M3dsZlFhNHFrdWhHeUMwWXNkeldidWFxaE1tdnJkQksrSDAKUUxPcnAzN3l2OHVwUFVlMXhwTzZTeUg5QjVEeXhEWkVjMXN6WVpSVXdNVzZxc3NkWEZvWGZ0SjYxZmo3S05XagoyamcwZkQ1ZEhFT1RObDFDT3p3Q1lvR1k5ejVWOHNhYy9sSDg3UkxYWXdBcXdvcEdpanM4QXBCeklURm8KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="

func TestNew(t *testing.T) {
	from, err := url.Parse("https://redhat.com")
	if err != nil {
		t.Fatalf("failed to parse `from` URL: %v", err)
	}
	toUpload, err := url.Parse("https://k8s.io")
	if err != nil {
		t.Fatalf("failed to parse `toUpload` URL: %v", err)
	}

	tc := []struct {
		c   Config
		err bool
	}{
		{
			// Empty configuration should error.
			c: Config{
				Logger:       log.NewNopLogger(),
				ToUploadCA:   "../../testdata/tls/ca.crt",
				ToUploadCert: "../../testdata/tls/tls.crt",
				ToUploadKey:  "../../testdata/tls/tls.key",
			},
			err: true,
		},
		{
			// Only providing a `From` should not error.
			c: Config{
				From:         from,
				Logger:       log.NewNopLogger(),
				ToUploadCA:   "../../testdata/tls/ca.crt",
				ToUploadCert: "../../testdata/tls/tls.crt",
				ToUploadKey:  "../../testdata/tls/tls.key",
			},
			err: false,
		},
		{
			// Providing `From` and `ToUpload` should not error.
			c: Config{
				From:         from,
				ToUpload:     toUpload,
				Logger:       log.NewNopLogger(),
				ToUploadCA:   "../../testdata/tls/ca.crt",
				ToUploadCert: "../../testdata/tls/tls.crt",
				ToUploadKey:  "../../testdata/tls/tls.key",
			},
			err: false,
		},
		{
			// Providing an invalid `FromTokenFile` file should error.
			c: Config{
				From:          from,
				FromTokenFile: "/this/path/does/not/exist",
				Logger:        log.NewNopLogger(),
				ToUploadCA:    "../../testdata/tls/ca.crt",
				ToUploadCert:  "../../testdata/tls/tls.crt",
				ToUploadKey:   "../../testdata/tls/tls.key",
			},
			err: true,
		},
		{
			// Providing only `AnonymizeSalt` should not error.
			c: Config{
				From:          from,
				AnonymizeSalt: "1",
				Logger:        log.NewNopLogger(),
				ToUploadCA:    "../../testdata/tls/ca.crt",
				ToUploadCert:  "../../testdata/tls/tls.crt",
				ToUploadKey:   "../../testdata/tls/tls.key",
			},
			err: false,
		},
		{
			// Providing only `AnonymizeLabels` should error.
			c: Config{
				From:            from,
				AnonymizeLabels: []string{"foo"},
				Logger:          log.NewNopLogger(),
				ToUploadCA:      "../../testdata/tls/ca.crt",
				ToUploadCert:    "../../testdata/tls/tls.crt",
				ToUploadKey:     "../../testdata/tls/tls.key",
			},
			err: true,
		},
		{
			// Providing only `AnonymizeSalt` and `AnonymizeLabels should not error.
			c: Config{
				From:            from,
				AnonymizeLabels: []string{"foo"},
				AnonymizeSalt:   "1",
				Logger:          log.NewNopLogger(),
				ToUploadCA:      "../../testdata/tls/ca.crt",
				ToUploadCert:    "../../testdata/tls/tls.crt",
				ToUploadKey:     "../../testdata/tls/tls.key",
			},
			err: false,
		},
		{
			// Providing an invalid `AnonymizeSaltFile` should error.
			c: Config{
				From:              from,
				AnonymizeLabels:   []string{"foo"},
				AnonymizeSaltFile: "/this/path/does/not/exist",
				Logger:            log.NewNopLogger(),
				ToUploadCA:        "../../testdata/tls/ca.crt",
				ToUploadCert:      "../../testdata/tls/tls.crt",
				ToUploadKey:       "../../testdata/tls/tls.key",
			},
			err: true,
		},
		{
			// Providing `AnonymizeSalt` takes preference over an invalid `AnonymizeSaltFile` and should not error.
			c: Config{
				From:              from,
				AnonymizeLabels:   []string{"foo"},
				AnonymizeSalt:     "1",
				AnonymizeSaltFile: "/this/path/does/not/exist",
				Logger:            log.NewNopLogger(),
				ToUploadCA:        "../../testdata/tls/ca.crt",
				ToUploadCert:      "../../testdata/tls/tls.crt",
				ToUploadKey:       "../../testdata/tls/tls.key",
			},
			err: false,
		},
		{
			// Providing an invalid `FromCAFile` should error.
			c: Config{
				From:         from,
				FromCAFile:   "/this/path/does/not/exist",
				Logger:       log.NewNopLogger(),
				ToUploadCA:   "../../testdata/tls/ca.crt",
				ToUploadCert: "../../testdata/tls/tls.crt",
				ToUploadKey:  "../../testdata/tls/tls.key",
			},
			err: true,
		},
		{
			// Providing CustomCA should not error.
			c: Config{
				From:         from,
				ToUpload:     toUpload,
				Logger:       log.NewNopLogger(),
				ToUploadCA:   "../../testdata/tls/ca.crt",
				ToUploadCert: "../../testdata/tls/tls.crt",
				ToUploadKey:  "../../testdata/tls/tls.key",
			},
			err: false,
		},
	}

	for i := range tc {
		tc[i].c.Metrics = NewWorkerMetrics(prometheus.NewRegistry())
		if i == 10 {
			os.Setenv("HTTPS_PROXY_CA_BUNDLE", customCA)
		}
		if _, err := New(tc[i].c); (err != nil) != tc[i].err {
			no := "no"
			if tc[i].err {
				no = "an"
			}
			t.Errorf("test case %d: got '%v', expected %s error", i, err, no)
		}
	}
}

func TestReconfigure(t *testing.T) {
	from, err := url.Parse("https://redhat.com")
	if err != nil {
		t.Fatalf("failed to parse `from` URL: %v", err)
	}
	c := Config{
		From:         from,
		Logger:       log.NewNopLogger(),
		Metrics:      NewWorkerMetrics(prometheus.NewRegistry()),
		ToUploadCA:   "../../testdata/tls/ca.crt",
		ToUploadCert: "../../testdata/tls/tls.crt",
		ToUploadKey:  "../../testdata/tls/tls.key",
	}
	w, err := New(c)
	if err != nil {
		t.Fatalf("failed to create new worker: %v", err)
	}

	from2, err := url.Parse("https://redhat.com")
	if err != nil {
		t.Fatalf("failed to parse `from2` URL: %v", err)
	}

	tc := []struct {
		c   Config
		err bool
	}{
		{
			// Empty configuration should error.
			c: Config{
				Logger:       log.NewNopLogger(),
				ToUploadCA:   "../../testdata/tls/ca.crt",
				ToUploadCert: "../../testdata/tls/tls.crt",
				ToUploadKey:  "../../testdata/tls/tls.key",
			},
			err: true,
		},
		{
			// Configuration with new `From` should not error.
			c: Config{
				From:         from2,
				Logger:       log.NewNopLogger(),
				ToUploadCA:   "../../testdata/tls/ca.crt",
				ToUploadCert: "../../testdata/tls/tls.crt",
				ToUploadKey:  "../../testdata/tls/tls.key",
			},
			err: false,
		},
		{
			// Configuration with new invalid field should error.
			c: Config{
				From:          from,
				FromTokenFile: "/this/path/does/not/exist",
				Logger:        log.NewNopLogger(),
				ToUploadCA:    "../../testdata/tls/ca.crt",
				ToUploadCert:  "../../testdata/tls/tls.crt",
				ToUploadKey:   "../../testdata/tls/tls.key",
			},
			err: true,
		},
	}

	for i := range tc {
		tc[i].c.Metrics = NewWorkerMetrics(prometheus.NewRegistry())
		if err := w.Reconfigure(tc[i].c); (err != nil) != tc[i].err {
			no := "no"
			if tc[i].err {
				no = "an"
			}
			t.Errorf("test case %d: got %q, expected %s error", i, err, no)
		}
	}
}

// TestRun tests the Run method of the Worker type.
// This test will:
// * instantiate a worker
// * configure the worker to make requests against a test server
// * in that test server, reconfigure the worker to make requests against a second test server
// * in the second test server, cancel the worker's context.
// This test will only succeed if the worker is able to be correctly reconfigured and canceled
// such that the Run method returns.
func TestRun(t *testing.T) {
	c := Config{
		// Use a dummy URL.
		From:         &url.URL{},
		FromQuery:    &url.URL{},
		Logger:       log.NewNopLogger(),
		Metrics:      NewWorkerMetrics(prometheus.NewRegistry()),
		ToUploadCA:   "../../testdata/tls/ca.crt",
		ToUploadCert: "../../testdata/tls/tls.crt",
		ToUploadKey:  "../../testdata/tls/tls.key",
	}
	w, err := New(c)
	if err != nil {
		t.Fatalf("failed to create new worker: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	var once sync.Once
	var wg sync.WaitGroup

	wg.Add(1)
	// This is the second test server. We need to define it early so we can use its URL in the
	// handler for the first test server.
	// In this handler, we decrement the wait group and cancel the worker's context.
	ts2 := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		cancel()
		once.Do(wg.Done)
	}))
	defer ts2.Close()

	// This is the first test server.
	// In this handler, we test the Reconfigure method of the worker and point it to the second
	// test server.
	ts1 := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		go func() {
			from, err := url.Parse(ts2.URL)
			if err != nil {
				stdlog.Fatalf("failed to parse second test server URL: %v", err)
			}
			if err := w.Reconfigure(Config{
				From:         from,
				FromQuery:    from,
				Logger:       log.NewNopLogger(),
				Metrics:      NewWorkerMetrics(prometheus.NewRegistry()),
				ToUploadCA:   "../../testdata/tls/ca.crt",
				ToUploadCert: "../../testdata/tls/tls.crt",
				ToUploadKey:  "../../testdata/tls/tls.key",
			}); err != nil {
				stdlog.Fatalf("failed to reconfigure worker with second test server url: %v", err)
			}
		}()
	}))
	defer ts1.Close()

	from, err := url.Parse(ts1.URL)
	if err != nil {
		t.Fatalf("failed to parse first test server URL: %v", err)
	}
	if err := w.Reconfigure(Config{
		From:           from,
		FromQuery:      from,
		RecordingRules: []string{"{\"name\":\"test\",\"query\":\"test\"}"},
		Logger:         log.NewNopLogger(),
		Metrics:        NewWorkerMetrics(prometheus.NewRegistry()),
		ToUploadCA:     "../../testdata/tls/ca.crt",
		ToUploadCert:   "../../testdata/tls/tls.crt",
		ToUploadKey:    "../../testdata/tls/tls.key",
	}); err != nil {
		t.Fatalf("failed to reconfigure worker with first test server url: %v", err)
	}

	wg.Add(1)
	// In this goroutine we run the worker and only decrement
	// the wait group when the worker finishes running.
	go func() {
		w.Run(ctx)
		wg.Done()
	}()

	wg.Wait()
}
