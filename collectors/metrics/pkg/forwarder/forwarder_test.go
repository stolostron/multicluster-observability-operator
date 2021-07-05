// Copyright Contributors to the Open Cluster Management project
package forwarder

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sync"
	"testing"

	"github.com/go-kit/kit/log"
)

func init() {
	os.Setenv("UNIT_TEST", "true")
}

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
			c:   Config{Logger: log.NewNopLogger()},
			err: true,
		},
		{
			// Only providing a `From` should not error.
			c: Config{
				From:   from,
				Logger: log.NewNopLogger(),
			},
			err: false,
		},
		{
			// Providing `From` and `ToUpload` should not error.
			c: Config{
				From:     from,
				ToUpload: toUpload,
				Logger:   log.NewNopLogger(),
			},
			err: false,
		},
		{
			// Providing an invalid `FromTokenFile` file should error.
			c: Config{
				From:          from,
				FromTokenFile: "/this/path/does/not/exist",
				Logger:        log.NewNopLogger(),
			},
			err: true,
		},
		{
			// Providing only `AnonymizeSalt` should not error.
			c: Config{
				From:          from,
				AnonymizeSalt: "1",
				Logger:        log.NewNopLogger(),
			},
			err: false,
		},
		{
			// Providing only `AnonymizeLabels` should error.
			c: Config{
				From:            from,
				AnonymizeLabels: []string{"foo"},
				Logger:          log.NewNopLogger(),
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
			},
			err: false,
		},
		{
			// Providing an invalid `FromCAFile` should error.
			c: Config{
				From:       from,
				FromCAFile: "/this/path/does/not/exist",
				Logger:     log.NewNopLogger(),
			},
			err: true,
		},
	}

	for i := range tc {
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
		From:   from,
		Logger: log.NewNopLogger(),
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
			c:   Config{Logger: log.NewNopLogger()},
			err: true,
		},
		{
			// Configuration with new `From` should not error.
			c: Config{
				From:   from2,
				Logger: log.NewNopLogger(),
			},
			err: false,
		},
		{
			// Configuration with new invalid field should error.
			c: Config{
				From:          from,
				FromTokenFile: "/this/path/does/not/exist",
				Logger:        log.NewNopLogger(),
			},
			err: true,
		},
	}

	for i := range tc {
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
		From:   &url.URL{},
		Logger: log.NewNopLogger(),
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
				t.Fatalf("failed to parse second test server URL: %v", err)
			}
			if err := w.Reconfigure(Config{From: from, Logger: log.NewNopLogger()}); err != nil {
				t.Fatalf("failed to reconfigure worker with second test server url: %v", err)
			}
		}()
	}))
	defer ts1.Close()

	from, err := url.Parse(ts1.URL)
	if err != nil {
		t.Fatalf("failed to parse first test server URL: %v", err)
	}
	if err := w.Reconfigure(Config{From: from, Logger: log.NewNopLogger()}); err != nil {
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
