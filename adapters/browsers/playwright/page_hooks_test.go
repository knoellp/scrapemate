package playwright

// Tests for the RequestHookProvider implementation on *Page.
//
// Full hook-invocation tests (OnRequest handler actually called by the browser)
// require a live Playwright browser and are gated with //go:build integration.
// The unit tests here cover:
//
//  1. Backward-compat: BrowserPage implementations without RequestHookProvider
//     compile and work as before — no interface change.
//  2. Positive: *Page satisfies RequestHookProvider (compile-time assertion in
//     page.go; documented here as a test for visibility).
//  3. Type-assertion failure branch: a BrowserPage that is NOT *Page does not
//     satisfy RequestHookProvider, so the guarded consumer pattern works correctly.

import (
	"time"
	"testing"

	"github.com/gosom/scrapemate"
)

// minimalPage is a minimal BrowserPage implementation that does NOT implement
// RequestHookProvider.  It verifies backward compatibility: callers that guard
// hook registration with a type assertion continue to work when the page does
// not support hooks.
type minimalPage struct{}

func (m *minimalPage) Goto(_ string, _ scrapemate.WaitUntilState) (*scrapemate.PageResponse, error) {
	return &scrapemate.PageResponse{StatusCode: 200}, nil
}
func (m *minimalPage) URL() string                                              { return "" }
func (m *minimalPage) Content() (string, error)                                 { return "", nil }
func (m *minimalPage) Reload(_ scrapemate.WaitUntilState) error                 { return nil }
func (m *minimalPage) Screenshot(_ bool) ([]byte, error)                        { return nil, nil }
func (m *minimalPage) Eval(_ string, _ ...any) (any, error)                     { return nil, nil }
func (m *minimalPage) WaitForURL(_ string, _ time.Duration) error               { return nil }
func (m *minimalPage) WaitForSelector(_ string, _ time.Duration) error          { return nil }
func (m *minimalPage) WaitForTimeout(_ time.Duration)                           {}
func (m *minimalPage) Locator(_ string) scrapemate.Locator                      { return nil }
func (m *minimalPage) Close() error                                             { return nil }
func (m *minimalPage) Unwrap() any                                              { return nil }

// Compile-time assertion: minimalPage satisfies BrowserPage (NOT RequestHookProvider).
var _ scrapemate.BrowserPage = (*minimalPage)(nil)

// TestBackwardCompat_NonHookPage verifies that BrowserPage implementations
// that do not implement RequestHookProvider are handled gracefully.
// The consumer pattern — type-assert before calling hooks — must work without
// panics or errors when the page does not support hooks.
func TestBackwardCompat_NonHookPage(t *testing.T) {
	var page scrapemate.BrowserPage = &minimalPage{}

	// Type assertion must return ok == false for a non-hook page.
	_, ok := page.(scrapemate.RequestHookProvider)
	if ok {
		t.Error("minimalPage should NOT implement RequestHookProvider")
	}

	// Consumer pattern simulation: guard with type assertion, skip if not supported.
	hookRegistered := false
	if hook, ok := page.(scrapemate.RequestHookProvider); ok {
		hook.OnRequest(func(_ string, _ map[string]string) {})
		hookRegistered = true
	}
	if hookRegistered {
		t.Error("OnRequest must not be registered for a non-hook page")
	}
}

// TestPageImplementsRequestHookProvider documents the compile-time guarantee.
//
// The statement `var _ scrapemate.RequestHookProvider = (*Page)(nil)` in page.go
// ensures *Page satisfies RequestHookProvider at compile time. If that assertion
// were removed and the interface were no longer satisfied, this package would fail
// to compile. This test records the intent in a human-readable form.
func TestPageImplementsRequestHookProvider(t *testing.T) {
	// Verified by compile-time assertion in page.go:
	//   var _ scrapemate.RequestHookProvider = (*Page)(nil)
	//
	// A live *Page cannot be constructed in a unit test without a Playwright
	// browser process. Runtime type-assertion of *Page through BrowserPage is
	// covered by TestIntegration_RequestHookProvider_TypeAssertion (build tag:
	// integration, requires PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=0).
	t.Log("*Page implements scrapemate.RequestHookProvider — guaranteed by compile-time assertion in page.go")
}

// TestTypeAssertion_BrowserPageFailsForNonHookPage verifies the negative branch
// of the consumer type-assertion pattern. When a BrowserPage does not implement
// RequestHookProvider the assertion returns (nil, false) and the caller skips
// hook registration, falling back to alternative behaviour (e.g. page.Unwrap()).
func TestTypeAssertion_BrowserPageFailsForNonHookPage(t *testing.T) {
	var page scrapemate.BrowserPage = &minimalPage{}
	if _, ok := page.(scrapemate.RequestHookProvider); ok {
		t.Error("minimalPage must not satisfy RequestHookProvider")
	}
}
