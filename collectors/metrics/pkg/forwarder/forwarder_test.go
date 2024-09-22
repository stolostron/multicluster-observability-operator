// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package forwarder

import (
	"net/url"
	"os"
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
				Logger: log.NewNopLogger(),
			},
			err: true,
		},
		{
			// Only providing a `From` should not error.
			c: Config{
				FromClientConfig: FromClientConfig{
					URL: from,
				},
				Logger: log.NewNopLogger(),
			},
			err: false,
		},
		{
			// Providing `From` and `ToUpload` should not error.
			c: Config{
				FromClientConfig: FromClientConfig{
					URL: from,
				},
				ToClientConfig: ToClientConfig{
					URL: toUpload,
				},
				Logger: log.NewNopLogger(),
			},
			err: false,
		},
		{
			// Providing an invalid `FromTokenFile` file should error.
			c: Config{
				FromClientConfig: FromClientConfig{
					URL:       from,
					TokenFile: "/this/path/does/not/exist",
				},
				Logger: log.NewNopLogger(),
			},
			err: true,
		},
		{
			// Providing only `AnonymizeSalt` should not error.
			c: Config{
				FromClientConfig: FromClientConfig{
					URL: from,
				},
				AnonymizeSalt: "1",
				Logger:        log.NewNopLogger(),
			},
			err: false,
		},
		{
			// Providing only `AnonymizeLabels` should error.
			c: Config{
				FromClientConfig: FromClientConfig{
					URL: from,
				},
				AnonymizeLabels: []string{"foo"},
				Logger:          log.NewNopLogger(),
			},
			err: true,
		},
		{
			// Providing only `AnonymizeSalt` and `AnonymizeLabels should not error.
			c: Config{
				FromClientConfig: FromClientConfig{
					URL: from,
				},
				AnonymizeLabels: []string{"foo"},
				AnonymizeSalt:   "1",
				Logger:          log.NewNopLogger(),
			},
			err: false,
		},
		{
			// Providing an invalid `AnonymizeSaltFile` should error.
			c: Config{
				FromClientConfig: FromClientConfig{
					URL: from,
				},
				AnonymizeLabels:   []string{"foo"},
				AnonymizeSaltFile: "/this/path/does/not/exist",
				Logger:            log.NewNopLogger(),
			},
			err: true,
		},
		{
			// Providing `AnonymizeSalt` takes preference over an invalid `AnonymizeSaltFile` and should not error.
			c: Config{
				FromClientConfig: FromClientConfig{
					URL: from,
				},
				AnonymizeLabels:   []string{"foo"},
				AnonymizeSalt:     "1",
				AnonymizeSaltFile: "/this/path/does/not/exist",
				Logger:            log.NewNopLogger(),
				ToClientConfig: ToClientConfig{
					CAFile:   "../../testdata/tls/ca.crt",
					CertFile: "../../testdata/tls/tls.crt",
					KeyFile:  "../../testdata/tls/tls.key",
				},
			},
			err: false,
		},
		{
			// Providing an invalid `FromCAFile` should error.
			c: Config{
				FromClientConfig: FromClientConfig{
					URL:    from,
					CAFile: "/this/path/does/not/exist",
				},
				Logger: log.NewNopLogger(),
			},
			err: true,
		},
		{
			// Providing CustomCA should not error.
			c: Config{
				FromClientConfig: FromClientConfig{
					URL: from,
				},
				ToClientConfig: ToClientConfig{
					URL:      toUpload,
					CAFile:   "../../testdata/tls/ca.crt",
					CertFile: "../../testdata/tls/tls.crt",
					KeyFile:  "../../testdata/tls/tls.key",
				},
				Logger: log.NewNopLogger(),
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
		FromClientConfig: FromClientConfig{
			URL: from,
		},
		Logger:  log.NewNopLogger(),
		Metrics: NewWorkerMetrics(prometheus.NewRegistry()),
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
				Logger: log.NewNopLogger(),
			},
			err: true,
		},
		{
			// Configuration with new `From` should not error.
			c: Config{
				FromClientConfig: FromClientConfig{
					URL: from2,
				},
				Logger: log.NewNopLogger(),
			},
			err: false,
		},
		{
			// Configuration with new invalid field should error.
			c: Config{
				FromClientConfig: FromClientConfig{
					URL:       from,
					TokenFile: "/this/path/does/not/exist",
				},
				Logger: log.NewNopLogger(),
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
