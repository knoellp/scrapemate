package nethttp

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/url"
	"sync"

	"github.com/gosom/scrapemate"
)

var _ scrapemate.HTTPFetcher = (*httpFetch)(nil)

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// New creates a nethttp fetcher using the provided HTTP client.
// The client's Transport is used for all requests.  This is the original
// constructor and remains the default path — no per-job proxy override.
func New(netClient HTTPClient) scrapemate.HTTPFetcher {
	return &httpFetch{
		netClient: netClient,
	}
}

// NewWithRotator creates a nethttp fetcher that supports per-job proxy
// overrides via the scrapemate.ProxyProvider interface.
//
// When a job implements ProxyProvider and returns a non-empty URL,
// a per-request http.Client is built with a Transport that routes through
// that URL.  Otherwise the injected netClient is used unchanged (same
// behaviour as New).
//
// scrapemateapp.getFetcher() calls this variant when a proxy rotator is
// configured alongside !UseJS && !UseStealth, so that jobs with sticky
// sessions can override the app-level round-robin without touching
// the default path for jobs that do not implement ProxyProvider.
func NewWithRotator(netClient HTTPClient, rotator scrapemate.ProxyRotator) scrapemate.HTTPFetcher {
	return &httpFetch{
		netClient:    netClient,
		proxyRotator: rotator,
	}
}

type httpFetch struct {
	netClient    HTTPClient
	proxyRotator scrapemate.ProxyRotator // optional; used for per-job proxy override signalling
}

func (o *httpFetch) Close() error {
	return nil
}

func (o *httpFetch) Fetch(ctx context.Context, job scrapemate.IJob) scrapemate.Response {
	// R1.5: per-job proxy takes precedence over the injected netClient.
	jobProxyURL := scrapemate.ResolveJobProxyURL(job)

	client := o.netClient
	if jobProxyURL != "" {
		client = buildPerJobClient(jobProxyURL)
	}

	return o.fetchWith(ctx, job, client)
}

// fetchWith executes the HTTP request using the provided client.
func (o *httpFetch) fetchWith(ctx context.Context, job scrapemate.IJob, client HTTPClient) scrapemate.Response {
	u := job.GetFullURL()
	reqBody := getBuffer()

	defer putBuffer(reqBody)

	if len(job.GetBody()) > 0 {
		reqBody.Write(job.GetBody())
	}

	var ans scrapemate.Response

	req, err := http.NewRequestWithContext(ctx, job.GetMethod(), u, reqBody)
	if err != nil {
		ans.Error = err
		return ans
	}

	for k, v := range job.GetHeaders() {
		req.Header.Add(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		ans.Error = err
		return ans
	}

	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	ans.StatusCode = resp.StatusCode
	ans.Headers = http.Header{}

	for k, v := range resp.Header {
		ans.Headers[k] = v
	}

	var reader io.ReadCloser

	switch resp.Header.Get("Content-Encoding") {
	case "gzip":
		reader, err = gzip.NewReader(resp.Body)
		if err != nil {
			ans.Error = err
			return ans
		}
		defer reader.Close()
	default:
		reader = resp.Body
	}

	ans.Body, ans.Error = io.ReadAll(reader)
	ans.URL = resp.Request.URL.String()

	return ans
}

// buildPerJobClient creates a disposable *http.Client that routes all traffic
// through the given proxy URL.  The client is used for exactly one Fetch call.
func buildPerJobClient(proxyURL string) *http.Client {
	u, err := url.Parse(proxyURL)
	if err != nil {
		// Return a plain client if the URL cannot be parsed; the request will
		// proceed without a proxy rather than failing to build the client.
		return &http.Client{}
	}

	return &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(u),
		},
	}
}

var bufferPool = sync.Pool{
	New: func() interface{} {
		return new(bytes.Buffer)
	},
}

func getBuffer() *bytes.Buffer {
	//nolint:errcheck // we don't care about errors here
	b := bufferPool.Get().(*bytes.Buffer)
	b.Reset()

	return b
}

func putBuffer(buf *bytes.Buffer) {
	bufferPool.Put(buf)
}
