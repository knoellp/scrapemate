# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- `ProxyProvider` interface — optional capability for jobs that need a per-job
  proxy URL, overriding the app-level `WithProxies` round-robin. Implement
  `GetProxyURL() string` on your job type to opt in. Backward-compatible —
  jobs that do not implement the interface continue to use the existing
  `WithProxies` round-robin without any code changes. Fetchers that support
  per-job proxy: `jshttp`, `stealth`, `nethttp`.
- `ResolveJobProxyURL(job IJob) string` — package-level helper that performs
  the type-assertion for `ProxyProvider`. Fetchers call this once per `Fetch()`
  before deciding which proxy to use.
- `nethttp.NewWithRotator(netClient HTTPClient, rotator ProxyRotator)` — variant
  of `nethttp.New` used by `scrapemateapp` when a rotator is configured.
  Enables per-job proxy overrides via `ProxyProvider` while leaving the injected
  client unchanged for jobs that do not implement the interface.

- `JSFetcherOptions.BrowserType` — selects the Playwright browser engine for JS
  rendering. Accepted values: `"chromium"` (default, empty string), `"firefox"`,
  `"webkit"`. Empty string retains existing Chromium behaviour; no action needed
  by callers that do not set this field (backward-compatible).
- `JSFetcherOptions.ExecutablePath` — optional path to a custom browser binary.
  When set, overrides the Playwright-managed binary (e.g. point at a Camoufox
  build for anti-detect Firefox).
- `scrapemateapp.WithJSBrowserType(s string)` — `WithJS` sub-option that sets
  `BrowserType`. Example: `WithJS(WithJSBrowserType("firefox"))`.
- `scrapemateapp.WithJSExecutablePath(p string)` — `WithJS` sub-option that sets
  `ExecutablePath`. Example:
  `WithJS(WithJSBrowserType("firefox"), WithJSExecutablePath("/opt/camoufox/firefox"))`.

### Removed

- Rod browser support, build tags, and related fetcher/page implementations
- Rod-specific example wiring and documentation

### Changed

- JavaScript rendering is now Playwright-only
- `scrapemateapp.WithBrowserEngine()` and `scrapemateapp.WithRodStealth()` remain as deprecated no-op compatibility shims

## [1.0.0] - 2026-01-06

### Breaking Changes

#### Browser Interface Abstraction

The `IJob` interface signature has changed to support multiple browser engines:

**Before (v0.9.6):**
```go
BrowserActions(ctx context.Context, page playwright.Page) Response
```

**After (v1.0.0):**
```go
BrowserActions(ctx context.Context, page BrowserPage) Response
```

#### Migration Guide

1. **Update the method signature** in your job types:

```go
// Before
func (j *MyJob) BrowserActions(ctx context.Context, page playwright.Page) scrapemate.Response {
    // ...
}

// After
func (j *MyJob) BrowserActions(ctx context.Context, page scrapemate.BrowserPage) scrapemate.Response {
    // ...
}
```

2. **Update navigation calls** to use the new interface:

```go
// Before
page.Goto("https://example.com", playwright.PageGotoOptions{
    WaitUntil: playwright.WaitUntilStateNetworkidle,
})

// After
resp, err := page.Goto("https://example.com", scrapemate.WaitUntilNetworkIdle)
```

3. **If you need the underlying browser page**, use `Unwrap()`:

```go
// For Playwright-specific features
pwPage := page.Unwrap().(playwright.Page)

// For Rod-specific features (when compiled with -tags rod)
rodPage := page.Unwrap().(*rod.Page)
```

#### Browser Engine Selection

Browser engine selection has changed from runtime configuration to compile-time build tags:

**Before (v0.9.6):**
```go
// Runtime selection (no longer supported)
scrapemateapp.WithBrowserEngine(scrapemateapp.BrowserEngineRod)
```

**After (v1.0.0):**
```bash
# Playwright (default)
go build ./...

# Rod
go build -tags rod ./...
```

### Added

- New `BrowserPage` interface providing a unified API for browser automation
- New `Locator` interface for element selection
- Support for Rod browser engine via build tags
- `WaitUntilState` constants: `WaitUntilLoad`, `WaitUntilDOMContentLoaded`, `WaitUntilNetworkIdle`
- `PageResponse` struct with `URL`, `StatusCode`, `Headers`, and `Body` fields
- Rod stealth mode support via `-stealth` flag
- Chrome flags for containerized environments (Rod)

### Changed

- `BrowserActions` now receives `scrapemate.BrowserPage` instead of `playwright.Page`
- Browser engine is now selected at compile time using build tags
- Rod implementation now returns actual response data (status code, headers, body)

### Removed

- Runtime browser engine selection via `WithBrowserEngine()` (function exists but is a no-op)

## [0.9.6] and earlier

See [GitHub Releases](https://github.com/gosom/scrapemate/releases) for previous versions.
