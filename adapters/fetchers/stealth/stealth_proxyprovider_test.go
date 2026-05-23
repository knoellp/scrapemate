package stealth_test

// Tests for R1.5 per-job-proxy integration in the stealth fetcher.
//
// The stealth fetcher uses azuretls.Session.SetProxy for proxy configuration.
// These tests verify the proxy URL resolution logic: which proxy URL (per-job
// or rotator fallback) is selected without requiring real network proxies.
//
// Approach: use a local httptest.Server as the request target.  We verify
// proxy selection by observing that:
//   - per-job proxy URL (pointing to a non-existent address) causes a
//     connection error distinct from the no-proxy path
//   - the rotator's Next() is called on the fallback path and not on the
//     per-job-proxy path

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/gosom/scrapemate"
	stealthfetcher "github.com/gosom/scrapemate/adapters/fetchers/stealth"
)

// --- Helpers ---

// minimalJob is an IJob that targets a given URL with no proxy preference.
type minimalJob struct {
	scrapemate.Job
}

// proxyProviderJob implements ProxyProvider in addition to IJob.
type proxyProviderJob struct {
	scrapemate.Job
	proxyURL string
}

func (j *proxyProviderJob) GetProxyURL() string {
	return j.proxyURL
}

// countingRotator records how many times Next() is called.
type countingRotator struct {
	count int
	proxy scrapemate.Proxy
}

func (r *countingRotator) Next() scrapemate.Proxy {
	r.count++
	return r.proxy
}

func (r *countingRotator) Proxies() []string {
	return []string{r.proxy.URL}
}

func (r *countingRotator) RoundTrip(_ *http.Request) (*http.Response, error) {
	return nil, nil
}

func (r *countingRotator) GetCredentials() (string, string) {
	return "", ""
}

func newTestJob(targetURL string) *minimalJob {
	job := &minimalJob{}
	job.Job.URL = targetURL
	job.Job.Method = http.MethodGet
	job.Job.Timeout = 3 * time.Second

	return job
}

func newProxyJob(targetURL, proxyURL string) *proxyProviderJob {
	job := &proxyProviderJob{proxyURL: proxyURL}
	job.Job.URL = targetURL
	job.Job.Method = http.MethodGet
	job.Job.Timeout = 3 * time.Second

	return job
}

// --- Tests ---

// TestStealth_BackwardCompat verifies that a plain job (no ProxyProvider)
// continues to use the rotator's Next() for proxy selection.
func TestStealth_BackwardCompat(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	rotator := &countingRotator{
		// Use an invalid proxy address — we only care that Next() was called,
		// not that the request succeeds via the proxy.
		proxy: scrapemate.Proxy{URL: "http://127.0.0.1:19999"},
	}

	fetcher := stealthfetcher.New("", rotator)
	defer fetcher.Close()

	job := newTestJob(srv.URL)
	// Request will fail via proxy (invalid address), but rotator was consulted.
	_ = fetcher.Fetch(context.Background(), job)

	require.Equal(t, 1, rotator.count, "rotator.Next() must be called once for a plain job")
}

// TestStealth_PerJobProxy verifies that when a job implements ProxyProvider
// with a non-empty URL, the per-job URL is used and the rotator is NOT called.
func TestStealth_PerJobProxy(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	rotator := &countingRotator{
		proxy: scrapemate.Proxy{URL: "http://127.0.0.1:19998"},
	}

	fetcher := stealthfetcher.New("", rotator)
	defer fetcher.Close()

	// Per-job proxy points to an invalid address — we verify that the rotator
	// is NOT consulted (count stays 0).
	job := newProxyJob(srv.URL, "http://127.0.0.1:19997")
	_ = fetcher.Fetch(context.Background(), job)

	require.Equal(t, 0, rotator.count, "rotator.Next() must NOT be called when job has its own proxy URL")
}

// TestStealth_EmptyProxyProvider verifies that a ProxyProvider returning ""
// falls back to the rotator (same as no ProxyProvider — backward-compat).
func TestStealth_EmptyProxyProvider(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	rotator := &countingRotator{
		proxy: scrapemate.Proxy{URL: "http://127.0.0.1:19996"},
	}

	fetcher := stealthfetcher.New("", rotator)
	defer fetcher.Close()

	// Empty proxyURL → fall through to rotator.
	job := newProxyJob(srv.URL, "")
	_ = fetcher.Fetch(context.Background(), job)

	require.Equal(t, 1, rotator.count, "rotator.Next() must be called when ProxyProvider returns empty string")
}
