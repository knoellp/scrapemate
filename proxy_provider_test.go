package scrapemate_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gosom/scrapemate"
)

// jobWithoutProxyProvider is a plain Job that does not implement ProxyProvider.
type jobWithoutProxyProvider struct {
	scrapemate.Job
}

// jobWithProxyProvider implements ProxyProvider and returns a fixed URL.
type jobWithProxyProvider struct {
	scrapemate.Job
	proxyURL string
}

func (j *jobWithProxyProvider) GetProxyURL() string {
	return j.proxyURL
}

func TestResolveJobProxyURL(t *testing.T) {
	t.Parallel()

	t.Run("job without ProxyProvider returns empty string", func(t *testing.T) {
		job := &jobWithoutProxyProvider{}
		got := scrapemate.ResolveJobProxyURL(job)
		require.Equal(t, "", got, "non-ProxyProvider job must yield empty string so callers fall back to round-robin")
	})

	t.Run("job with ProxyProvider returning empty string yields empty string", func(t *testing.T) {
		job := &jobWithProxyProvider{proxyURL: ""}
		got := scrapemate.ResolveJobProxyURL(job)
		require.Equal(t, "", got, "ProxyProvider returning empty string must propagate as empty (fallback to round-robin)")
	})

	t.Run("job with ProxyProvider returning valid URL yields that URL", func(t *testing.T) {
		const want = "http://user-session-abc:pass@gate.example.com:7000"
		job := &jobWithProxyProvider{proxyURL: want}
		got := scrapemate.ResolveJobProxyURL(job)
		require.Equal(t, want, got)
	})
}
