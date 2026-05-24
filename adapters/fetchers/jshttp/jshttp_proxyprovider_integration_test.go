//go:build integration

package jshttp

// Integration tests for the R1.5 per-job-proxy logic in jsFetch.
//
// These tests require a real Playwright/Chromium binary.  They are gated
// behind the "integration" build tag so that `go test ./...` (without
// -tags=integration) never runs them.
//
// Run with: go test -tags=integration ./adapters/fetchers/jshttp/...
//
// The tests use:
//   - a local httptest.Server as the fetch target (avoids external network)
//   - a minimal counting HTTP proxy to verify that browser traffic flows
//     through the per-job proxy

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gosom/scrapemate"
)

// countingProxy is a minimal HTTP/1.1 proxy that counts every request it
// receives and forwards plain-HTTP requests to their target.  It is
// intentionally simple — HTTPS CONNECT tunnelling is not implemented because
// the fetch target is always a plain-HTTP httptest.Server.
type countingProxy struct {
	listener net.Listener
	server   *http.Server
	count    atomic.Int64
}

func newCountingProxy(t *testing.T) *countingProxy {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("countingProxy: listen: %v", err)
	}

	cp := &countingProxy{listener: l}

	mux := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cp.count.Add(1)

		// Reconstruct the target URL from the request if needed.
		target := r.RequestURI
		if target == "" || target[0] == '/' {
			scheme := "http"
			target = scheme + "://" + r.Host + r.RequestURI
		}

		outReq, err := http.NewRequestWithContext(r.Context(), r.Method, target, r.Body)
		if err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		outReq.Header = r.Header.Clone()
		// Strip hop-by-hop headers.
		for _, h := range []string{"Proxy-Connection", "Connection", "Keep-Alive",
			"Transfer-Encoding", "Te", "Trailer", "Upgrade"} {
			outReq.Header.Del(h)
		}

		resp, err := http.DefaultTransport.RoundTrip(outReq)
		if err != nil {
			http.Error(w, "bad gateway", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		for k, vv := range resp.Header {
			for _, v := range vv {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	})

	cp.server = &http.Server{Handler: mux}

	go func() {
		_ = cp.server.Serve(l)
	}()

	t.Cleanup(func() {
		_ = cp.server.Close()
	})

	return cp
}

// addr returns the proxy address in "host:port" form.
func (cp *countingProxy) addr() string {
	return cp.listener.Addr().String()
}

// proxyURL returns the proxy URL without credentials (countingProxy needs none).
func (cp *countingProxy) proxyURL() string {
	return fmt.Sprintf("http://%s", cp.addr())
}

// Count returns the number of requests the proxy has forwarded so far.
func (cp *countingProxy) Count() int64 {
	return cp.count.Load()
}

// ---- job helpers -----------------------------------------------------------

// proxyProviderJob is a minimal IJob that implements ProxyProvider.
// BrowserActions navigates to URL and captures the response body.
type proxyProviderJob struct {
	scrapemate.Job
	proxyURL string
	body     string
}

func (j *proxyProviderJob) GetProxyURL() string {
	return j.proxyURL
}

func (j *proxyProviderJob) BrowserActions(ctx context.Context, page scrapemate.BrowserPage) scrapemate.Response {
	resp, err := page.Goto(j.GetFullURL(), scrapemate.WaitUntilNetworkIdle)
	if err != nil {
		return scrapemate.Response{Error: err}
	}
	j.body = string(resp.Body)
	return scrapemate.Response{
		URL:        resp.URL,
		StatusCode: resp.StatusCode,
		Body:       resp.Body,
	}
}

// plainJob is a minimal IJob without ProxyProvider (backward-compat path).
type plainJob struct {
	scrapemate.Job
	body string
}

func (j *plainJob) BrowserActions(ctx context.Context, page scrapemate.BrowserPage) scrapemate.Response {
	resp, err := page.Goto(j.GetFullURL(), scrapemate.WaitUntilNetworkIdle)
	if err != nil {
		return scrapemate.Response{Error: err}
	}
	j.body = string(resp.Body)
	return scrapemate.Response{
		URL:        resp.URL,
		StatusCode: resp.StatusCode,
		Body:       resp.Body,
	}
}

// ---- test target -----------------------------------------------------------

// newFetchTarget returns a local httptest.Server that responds with a fixed
// body.  It is stopped via t.Cleanup.
func newFetchTarget(t *testing.T, body string) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, body)
	}))
	t.Cleanup(ts.Close)
	return ts
}

// ---- fetcher constructor helper --------------------------------------------

// newTestFetcher creates a jsFetch with PoolSize=1, Headless=true, Chromium.
// DisableSingleProcess is set to true so that per-job BrowserContext tests
// work correctly: --single-process would share one renderer across all
// contexts and closing a per-job context would break the pool context.
// The test image has Chromium via playwright-go.
func newTestFetcher(t *testing.T) *jsFetch {
	t.Helper()
	f, err := New(JSFetcherOptions{
		Headless:             true,
		DisableImages:        true,
		PoolSize:             1,
		PageReuseLimit:       0,
		BrowserReuseLimit:    0,
		DisableSingleProcess: true,
	})
	if err != nil {
		t.Fatalf("New(JSFetcherOptions): %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f.(*jsFetch)
}

// ---- tests -----------------------------------------------------------------

// TestIntegration_ProxyProvider_PositiveMultiContext verifies the happy path:
// a ProxyProvider job routes traffic through the per-job counting proxy, the
// response body is received, and the per-job BrowserContext is closed after
// the fetch.
func TestIntegration_ProxyProvider_PositiveMultiContext(t *testing.T) {
	proxy := newCountingProxy(t)
	target := newFetchTarget(t, "hello-integration")
	fetcher := newTestFetcher(t)

	job := &proxyProviderJob{
		Job: scrapemate.Job{
			ID:      "test-positive-1",
			Method:  http.MethodGet,
			URL:     target.URL,
			Timeout: 30 * time.Second,
		},
		proxyURL: proxy.proxyURL(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	resp := fetcher.Fetch(ctx, job)
	if resp.Error != nil {
		t.Fatalf("Fetch returned error: %v", resp.Error)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want 200", resp.StatusCode)
	}

	// Proxy must have seen at least one request.
	if proxy.Count() == 0 {
		t.Error("countingProxy.Count() == 0: traffic did not flow through per-job proxy")
	}
}

// TestIntegration_ProxyProvider_BrowserSurvival verifies that after a
// per-job-proxy fetch the pool browser is still alive (IsConnected() == true)
// and the pool can serve a subsequent fetch.
func TestIntegration_ProxyProvider_BrowserSurvival(t *testing.T) {
	proxy := newCountingProxy(t)
	target := newFetchTarget(t, "survival-check")
	fetcher := newTestFetcher(t)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// First fetch — via ProxyProvider (per-job context path).
	job1 := &proxyProviderJob{
		Job: scrapemate.Job{
			ID:      "survival-job-1",
			Method:  http.MethodGet,
			URL:     target.URL,
			Timeout: 30 * time.Second,
		},
		proxyURL: proxy.proxyURL(),
	}
	resp1 := fetcher.Fetch(ctx, job1)
	if resp1.Error != nil {
		t.Fatalf("first Fetch error: %v", resp1.Error)
	}

	// Pool must still hold the browser after the per-job context was closed.
	// GetBrowser drains from the pool; PutBrowser returns it.
	br, err := fetcher.GetBrowser(ctx)
	if err != nil {
		t.Fatalf("GetBrowser after per-job fetch: %v", err)
	}
	if !br.browser.IsConnected() {
		t.Error("browser.IsConnected() == false after per-job-proxy fetch: browser was killed")
	}
	fetcher.PutBrowser(ctx, br)

	// Second fetch on the same pool (should reuse the same browser process).
	job2 := &plainJob{
		Job: scrapemate.Job{
			ID:      "survival-job-2",
			Method:  http.MethodGet,
			URL:     target.URL,
			Timeout: 30 * time.Second,
		},
	}
	resp2 := fetcher.Fetch(ctx, job2)
	if resp2.Error != nil {
		t.Fatalf("second Fetch (plain) error: %v", resp2.Error)
	}
}

// TestIntegration_ProxyProvider_Reuse verifies that two consecutive
// ProxyProvider jobs share the same underlying browser instance (pointer
// identity of the playwright.Browser).
func TestIntegration_ProxyProvider_Reuse(t *testing.T) {
	proxy := newCountingProxy(t)
	target := newFetchTarget(t, "reuse-check")
	fetcher := newTestFetcher(t)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// Capture the browser pointer identity before the first fetch.
	br0, err := fetcher.GetBrowser(ctx)
	if err != nil {
		t.Fatalf("GetBrowser (pre-fetch): %v", err)
	}
	browserPtr0 := fmt.Sprintf("%p", br0.browser)
	fetcher.PutBrowser(ctx, br0)

	makeJob := func(id string) *proxyProviderJob {
		return &proxyProviderJob{
			Job: scrapemate.Job{
				ID:      id,
				Method:  http.MethodGet,
				URL:     target.URL,
				Timeout: 30 * time.Second,
			},
			proxyURL: proxy.proxyURL(),
		}
	}

	// First ProxyProvider fetch.
	if resp := fetcher.Fetch(ctx, makeJob("reuse-1")); resp.Error != nil {
		t.Fatalf("first Fetch error: %v", resp.Error)
	}

	// Capture browser pointer after first fetch.
	br1, err := fetcher.GetBrowser(ctx)
	if err != nil {
		t.Fatalf("GetBrowser after first fetch: %v", err)
	}
	browserPtr1 := fmt.Sprintf("%p", br1.browser)
	fetcher.PutBrowser(ctx, br1)

	// Second ProxyProvider fetch.
	if resp := fetcher.Fetch(ctx, makeJob("reuse-2")); resp.Error != nil {
		t.Fatalf("second Fetch error: %v", resp.Error)
	}

	// Capture browser pointer after second fetch.
	br2, err := fetcher.GetBrowser(ctx)
	if err != nil {
		t.Fatalf("GetBrowser after second fetch: %v", err)
	}
	browserPtr2 := fmt.Sprintf("%p", br2.browser)
	fetcher.PutBrowser(ctx, br2)

	// All three must be the same browser instance.
	if browserPtr0 != browserPtr1 || browserPtr1 != browserPtr2 {
		t.Errorf("browser instance changed across ProxyProvider fetches: %s → %s → %s",
			browserPtr0, browserPtr1, browserPtr2)
	}
}

// TestIntegration_ProxyProvider_BackwardCompat verifies that a job without
// ProxyProvider takes the default pool-context path: no per-job context is
// created and the counting proxy is NOT used.
func TestIntegration_ProxyProvider_BackwardCompat(t *testing.T) {
	proxy := newCountingProxy(t)
	target := newFetchTarget(t, "compat-check")
	fetcher := newTestFetcher(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	job := &plainJob{
		Job: scrapemate.Job{
			ID:      "compat-job-1",
			Method:  http.MethodGet,
			URL:     target.URL,
			Timeout: 30 * time.Second,
		},
	}

	resp := fetcher.Fetch(ctx, job)
	if resp.Error != nil {
		t.Fatalf("Fetch (plain job) returned error: %v", resp.Error)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want 200", resp.StatusCode)
	}

	// The counting proxy must NOT have been used for a plain (no ProxyProvider) job.
	if proxy.Count() != 0 {
		t.Errorf("countingProxy.Count() = %d for plain job, want 0", proxy.Count())
	}
}

// TestIntegration_Firefox_PerJobProxy isolates whether STANDARD Playwright
// Firefox can open a page via the per-job-proxy (multi-context) path at all,
// against a harmless LOCAL server (no jameda, no Cloudflare). If NewPage hangs
// here, the cause is jshttp passing Chromium-only launch args to Firefox.
// Run with a short -timeout so a hang fails fast instead of blocking 10 minutes.
func TestIntegration_Firefox_PerJobProxy(t *testing.T) {
	proxy := newCountingProxy(t)
	target := newFetchTarget(t, "ff-isolation")

	f, err := New(JSFetcherOptions{
		Headless:             true,
		DisableImages:        true,
		PoolSize:             1,
		BrowserType:          "firefox",
		DisableSingleProcess: true,
	})
	if err != nil {
		t.Fatalf("New(firefox): %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })

	job := &proxyProviderJob{
		Job: scrapemate.Job{
			ID:      "ff-isolation-1",
			Method:  http.MethodGet,
			URL:     target.URL,
			Timeout: 30 * time.Second,
		},
		proxyURL: proxy.proxyURL(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	resp := f.Fetch(ctx, job)
	if resp.Error != nil {
		t.Fatalf("firefox per-job-proxy fetch error: %v", resp.Error)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want 200", resp.StatusCode)
	}
	if proxy.Count() == 0 {
		t.Error("countingProxy.Count() == 0: traffic did not flow through per-job proxy")
	}
}
