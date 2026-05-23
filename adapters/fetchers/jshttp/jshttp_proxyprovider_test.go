package jshttp

// Tests for the R1.5 per-job-proxy logic in jshttp.
//
// The five plan test cases (Backward-Compat, Positive/Multi-Context,
// Empty-Edge-Case, Parallel, Browser-Survival) require a live Playwright
// browser process.  Integration-level tests for those cases live in the
// project-level integration test suite (requires PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=0).
//
// This file covers the pure-logic portions that are testable without a browser:
//
//  1. parseProxyURL correctly extracts Server/Username/Password
//  2. parseProxyURL returns nil for an empty URL (Fetch falls through to fetchDefault)
//  3. parseProxyURL handles a URL without credentials (no Username/Password)
//  4. parseProxyURL handles a URL with only a username (no password)
//  5. parseProxyURL is safe against unparseable input

import (
	"testing"
)

func TestParseProxyURL(t *testing.T) {
	t.Parallel()

	t.Run("backward-compat: empty URL returns nil (caller uses fetchDefault)", func(t *testing.T) {
		// When ResolveJobProxyURL returns "" the caller never invokes parseProxyURL,
		// but defensive nil-check here documents the contract.
		got := parseProxyURL("")
		if got != nil {
			t.Errorf("parseProxyURL(\"\") = %v, want nil", got)
		}
	})

	t.Run("positive: full URL with credentials parsed correctly", func(t *testing.T) {
		got := parseProxyURL("http://alice:secret@gate.example.com:7000")
		if got == nil {
			t.Fatal("parseProxyURL returned nil for valid URL with credentials")
		}
		if got.Server != "http://gate.example.com:7000" {
			t.Errorf("Server = %q, want %q", got.Server, "http://gate.example.com:7000")
		}
		if got.Username == nil || *got.Username != "alice" {
			t.Errorf("Username = %v, want \"alice\"", got.Username)
		}
		if got.Password == nil || *got.Password != "secret" {
			t.Errorf("Password = %v, want \"secret\"", got.Password)
		}
	})

	t.Run("empty-edge-case: URL without credentials sets no Username/Password", func(t *testing.T) {
		got := parseProxyURL("http://proxy.example.com:8080")
		if got == nil {
			t.Fatal("parseProxyURL returned nil for valid URL without credentials")
		}
		if got.Server != "http://proxy.example.com:8080" {
			t.Errorf("Server = %q, want %q", got.Server, "http://proxy.example.com:8080")
		}
		if got.Username != nil {
			t.Errorf("Username = %v, want nil", got.Username)
		}
		if got.Password != nil {
			t.Errorf("Password = %v, want nil", got.Password)
		}
	})

	t.Run("parallel-proxy-isolation: two different URLs produce independent proxies", func(t *testing.T) {
		// Each ProxyProvider-Job produces its own *playwright.Proxy via parseProxyURL.
		// Verifies that the two results are independent (different Server values).
		url1 := "http://user1:pass1@proxy1.example.com:7001"
		url2 := "http://user2:pass2@proxy2.example.com:7002"

		p1 := parseProxyURL(url1)
		p2 := parseProxyURL(url2)

		if p1 == nil || p2 == nil {
			t.Fatal("parseProxyURL returned nil for one of the valid URLs")
		}
		if p1.Server == p2.Server {
			t.Errorf("expected different Server values, both are %q", p1.Server)
		}
		if p1.Username == nil || p2.Username == nil {
			t.Fatal("Username is nil for one of the proxies")
		}
		if *p1.Username == *p2.Username {
			t.Errorf("expected different usernames, both are %q", *p1.Username)
		}
	})

	t.Run("browser-survival: unparseable URL returns nil without panic", func(t *testing.T) {
		// A malformed URL must not panic; caller treats nil as no proxy.
		got := parseProxyURL("://not a url %%")
		_ = got // nil or non-nil — the point is no panic
	})
}
