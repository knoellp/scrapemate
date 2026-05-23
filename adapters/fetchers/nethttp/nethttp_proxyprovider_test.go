package nethttp_test

// Tests for the R1.5 per-job-proxy logic in the nethttp fetcher.
//
// Three test cases from the plan:
//  1. Backward-Compat: no ProxyProvider → injected client is used (request succeeds)
//  2. Positive: ProxyProvider with URL → per-job Transport built (routes via proxy,
//     fails because test proxy address is not listening — proves per-job path taken)
//  3. Empty: ProxyProvider returns "" → injected client used (request succeeds)

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/gosom/scrapemate"
	netthttp "github.com/gosom/scrapemate/adapters/fetchers/nethttp"
)

// --- Job helpers ---

type plainJob struct {
	scrapemate.Job
}

type proxyProviderJob struct {
	scrapemate.Job
	proxyURL string
}

func (j *proxyProviderJob) GetProxyURL() string {
	return j.proxyURL
}

func newJob(targetURL string) *plainJob {
	job := &plainJob{}
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

// TestNethttp_BackwardCompat verifies that a plain job (no ProxyProvider)
// uses the injected client and succeeds against the test server.
func TestNethttp_BackwardCompat(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	fetcher := netthttp.New(srv.Client())
	defer fetcher.Close()

	job := newJob(srv.URL)
	resp := fetcher.Fetch(context.Background(), job)

	require.NoError(t, resp.Error, "plain job via injected client should succeed against test server")
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestNethttp_PerJobProxy verifies that when a job implements ProxyProvider
// with a non-empty URL, the fetcher routes through a per-job Transport.
// The proxy address (127.0.0.1:19990) is not listening, so the request fails —
// proving that the per-job code path was taken instead of the injected client.
func TestNethttp_PerJobProxy(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	fetcher := netthttp.NewWithRotator(srv.Client(), nil)
	defer fetcher.Close()

	// Per-job proxy → non-listening address → must produce a connection error.
	job := newProxyJob(srv.URL, "http://127.0.0.1:19990")
	resp := fetcher.Fetch(context.Background(), job)

	require.Error(t, resp.Error,
		"per-job proxy pointing to a non-listening address must produce an error, "+
			"proving the per-job Transport path was taken")
}

// TestNethttp_EmptyProxyProvider verifies that a job implementing ProxyProvider
// but returning "" falls back to the injected client (same as backward-compat).
func TestNethttp_EmptyProxyProvider(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	fetcher := netthttp.NewWithRotator(srv.Client(), nil)
	defer fetcher.Close()

	// Empty proxyURL → must fall back to injected client → request succeeds.
	job := newProxyJob(srv.URL, "")
	resp := fetcher.Fetch(context.Background(), job)

	require.NoError(t, resp.Error, "empty ProxyProvider URL must fall back to injected client and succeed")
	require.Equal(t, http.StatusOK, resp.StatusCode)
}
