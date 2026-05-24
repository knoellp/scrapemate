# scrapemate
[![Documentation](https://img.shields.io/badge/Documentation-Read%20Here-blue)](https://gosom.github.io/scrapemate)
[![GoDoc](https://godoc.org/github.com/gosom/scrapemate?status.svg)](https://godoc.org/github.com/gosom/scrapemate)
![build](https://github.com/gosom/scrapemate/actions/workflows/build.yml/badge.svg)
[![Go Report Card](https://goreportcard.com/badge/github.com/gosom/scrapemate)](https://goreportcard.com/report/github.com/gosom/scrapemate)

[Scrapemate](https://gosom.github.io/scrapemate) is a web crawling and scraping framework written in Golang. It is designed to be simple and easy to use, yet powerful enough to handle complex scraping tasks.


## Features

- Low level API & Easy High Level API
- Customizable retry and error handling
- Javascript Rendering with ability to control the browser
- Screenshots support (when JS rendering is enabled)
- Capability to write your own result exporter
- Capability to write results in multiple sinks
- Default CSV writer
- Caching (File/LevelDB/Custom)
- Custom job providers (memory provider included)
- Headless and Headful support when using JS rendering
- Automatic cookie and session handling
- Rotating HTTP/HTTPS/SOCKS5 proxy support

## JavaScript Rendering

Scrapemate uses Playwright for JavaScript rendering. It requires the Playwright browsers to be installed.

```bash
# Install playwright browsers
go run github.com/playwright-community/playwright-go/cmd/playwright install --with-deps chromium
```

Build and run without any special tags:

```bash
go build ./...
```

### Example Usage

The [books-to-scrape-simple](https://github.com/gosom/scrapemate/tree/main/examples/books-to-scrape-simple) example demonstrates JavaScript rendering with Playwright:

```bash
# Run with Playwright (default)
# First install browsers: go run github.com/playwright-community/playwright-go/cmd/playwright install --with-deps chromium
go run . -js
```


## JS Fetcher Options

When using `WithJS(...)` you can pass additional sub-options to control which
browser engine is used and whether to override the browser binary.

| Option | Default | Purpose |
|---|---|---|
| `WithJSBrowserType(s string)` | `""` (Chromium) | Select engine: `"chromium"`, `"firefox"`, or `"webkit"` |
| `WithJSExecutablePath(p string)` | `""` (Playwright cache) | Path to a custom browser binary |

By design these options are opt-in: omitting them preserves the existing
Chromium-default behavior. They were introduced so downstream consumers can
support browsers other than Chromium — for example, Firefox-based anti-detect
builds like [Camoufox](https://camoufox.com) for sites with aggressive Chromium
fingerprinting — without changing scrapemate's default for everyone else.

Both options are additive — existing code that calls `WithJS()` without them is
unaffected.

### Example: Firefox with a custom Camoufox binary

```go
cfg, err := scrapemateapp.NewConfig(
    writers,
    scrapemateapp.WithJS(
        scrapemateapp.WithJSBrowserType("firefox"),
        scrapemateapp.WithJSExecutablePath("/opt/camoufox/firefox"),
    ),
)
```

This is useful for anti-detect setups where a patched Firefox build (such as
[Camoufox](https://camoufox.com)) must be used instead of the Playwright-managed
binary.

## Per-Job Proxy

By default scrapemate uses a round-robin `WithProxies` rotator for all jobs.
If you need a different proxy per job — for example a sticky session ID for
account-specific routing or geo-targeting — implement the `ProxyProvider`
interface on your job type:

```go
type MyJob struct {
    scrapemate.Job
    SessionID string
}

// GetProxyURL implements scrapemate.ProxyProvider.
func (j *MyJob) GetProxyURL() string {
    return fmt.Sprintf("http://user-session-%s:pass@gate.example.com:7000", j.SessionID)
}
```

Jobs that do not implement `ProxyProvider` continue to use the app-level
`WithProxies` round-robin as before — **no action required** for existing code.
Empty return strings from `GetProxyURL()` are treated as "no preference" and
also fall back to round-robin.

Fetchers that currently honour `ProxyProvider`: `jshttp` (creates a fresh
`BrowserContext` per job on a pool browser), `stealth` (calls
`session.SetProxy`), `nethttp` (builds a per-request `http.Transport`).

> **Chromium users with `ProxyProvider` must also call `WithJSDisableSingleProcess`.**
> Chromium's `--single-process` flag (enabled by default) shares one renderer
> process across all `BrowserContext`s.  Closing a per-job context in that mode
> tears down the shared renderer and breaks subsequent fetches.
> `WithJSDisableSingleProcess` removes the flag so each context gets an isolated
> renderer.  Firefox and WebKit are unaffected.
>
> ```go
> scrapemateapp.WithJS(
>     scrapemateapp.WithJSDisableSingleProcess(), // required for ProxyProvider + Chromium
> )
> ```

## Browser Request Hooks

Some jobs need to observe or capture browser-level request and response headers
— for example, to extract a Bearer token that a JavaScript SPA attaches to
outbound API calls client-side.

Scrapemate provides an optional `RequestHookProvider` interface that
`BrowserPage` implementations may satisfy.  Always check for the capability via
a type assertion so your job remains portable across adapters:

```go
func (j *MyJob) BrowserActions(ctx context.Context, page scrapemate.BrowserPage) scrapemate.Response {
    if hook, ok := page.(scrapemate.RequestHookProvider); ok {
        hook.OnRequest(func(url string, headers map[string]string) {
            // headers keys are lower-cased; no blocking calls inside this handler.
            if strings.Contains(url, "/api/v3/") {
                if auth := headers["authorization"]; strings.HasPrefix(auth, "Bearer ") {
                    captureToken(strings.TrimPrefix(auth, "Bearer "))
                }
            }
        })
    }
    // Navigate; the SPA will fire XHRs, triggering the handler above.
    page.Goto(j.URL, scrapemate.WaitUntilDOMContentLoaded)
    // ...
}
```

**Threading constraint:** handlers registered via `OnRequest` / `OnResponse` are
invoked synchronously in the browser event loop.  They must not perform blocking
I/O or blocking Playwright calls (e.g. `Page.Eval`, `Response.AllHeaders`).
Non-blocking operations — channel sends, `atomic` stores, mutex-guarded writes
— are safe.

The jshttp Playwright adapter implements `RequestHookProvider`.  Other adapters
may not; the type assertion handles that gracefully.

**Why not `AllHeaders()`?** `req.AllHeaders()` and `resp.AllHeaders()` issue a
CDP protocol round-trip and block until the browser responds.  Calling them
inside an event handler deadlocks the event loop.  The hooks in the jshttp
adapter use `req.Headers()` / `resp.Headers()` instead, which return the headers
already stored in memory — no round-trip, no deadlock.

## Installation

```
go get github.com/gosom/scrapemate
```

## Quickstart


```go
package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/gosom/scrapemate"
	"github.com/gosom/scrapemate/adapters/writers/csvwriter"
	"github.com/gosom/scrapemate/scrapemateapp"
)

func main() {
	csvWriter := csvwriter.NewCsvWriter(csv.NewWriter(os.Stdout))

	cfg, err := scrapemateapp.NewConfig(
		[]scrapemate.ResultWriter{csvWriter},
	)
	if err != nil {
		panic(err)
	}
	app, err := scrapemateapp.NewScrapeMateApp(cfg)
	if err != nil {
		panic(err)
	}
	seedJobs := []scrapemate.IJob{
		&SimpleCountryJob{
			Job: scrapemate.Job{
				ID:     "identity",
				Method: http.MethodGet,
				URL:    "https://www.scrapethissite.com/pages/simple/",
				Headers: map[string]string{
					"User-Agent": scrapemate.DefaultUserAgent,
				},
				Timeout:    10 * time.Second,
				MaxRetries: 3,
			},
		},
	}
	err = app.Start(context.Background(), seedJobs...)
	if err != nil && err != scrapemate.ErrorExitSignal {
		panic(err)
	}
}

type SimpleCountryJob struct {
	scrapemate.Job
}

func (j *SimpleCountryJob) Process(ctx context.Context, resp *scrapemate.Response) (any, []scrapemate.IJob, error) {
	doc, ok := resp.Document.(*goquery.Document)
	if !ok {
		return nil, nil, fmt.Errorf("failed to cast response document to goquery document")
	}
	var countries []Country
	doc.Find("div.col-md-4.country").Each(func(i int, s *goquery.Selection) {
		var country Country
		country.Name = strings.TrimSpace(s.Find("h3.country-name").Text())
		country.Capital = strings.TrimSpace(s.Find("div.country-info span.country-capital").Text())
		country.Population = strings.TrimSpace(s.Find("div.country-info span.country-population").Text())
		country.Area = strings.TrimSpace(s.Find("div.country-info span.country-area").Text())
		countries = append(countries, country)
	})
	return countries, nil, nil
}

type Country struct {
	Name       string
	Capital    string
	Population string
	Area       string
}

func (c Country) CsvHeaders() []string {
	return []string{"Name", "Capital", "Population", "Area"}
}

func (c Country) CsvRow() []string {
	return []string{c.Name, c.Capital, c.Population, c.Area}
}

```

```
go mod tidy
go run main.go 1>countries.csv
```

(hit CTRL-C to exit)

## Migrating from v0.9.x to v1.0.0

Version 1.0.0 introduces a `BrowserPage` interface abstraction for browser automation. This is a breaking change for users who use JavaScript rendering with `BrowserActions`.

### Update `BrowserActions` signature

```go
// Before (v0.9.x)
func (j *MyJob) BrowserActions(ctx context.Context, page playwright.Page) scrapemate.Response {
    page.Goto("https://example.com", playwright.PageGotoOptions{
        WaitUntil: playwright.WaitUntilStateNetworkidle,
    })
    html, _ := page.Content()
    return scrapemate.Response{Body: []byte(html)}
}

// After (v1.0.0)
func (j *MyJob) BrowserActions(ctx context.Context, page scrapemate.BrowserPage) scrapemate.Response {
    resp, err := page.Goto("https://example.com", scrapemate.WaitUntilNetworkIdle)
    if err != nil {
        return scrapemate.Response{Error: err}
    }
    return scrapemate.Response{
        Body:       resp.Body,
        StatusCode: resp.StatusCode,
    }
}
```

### Accessing the underlying browser page

If you need browser-specific features, use `Unwrap()`:

```go
// For Playwright
pwPage := page.Unwrap().(playwright.Page)
```

See [CHANGELOG.md](CHANGELOG.md) for the full list of changes.

## Documentation

You can find more documentation [here](https://gosom.github.io/scrapemate)

For the High Level API see this [example](https://github.com/gosom/scrapemate/tree/main/examples/quotes-to-scrape-app).

Read also [how to use high level api](https://blog.gkomninos.com/golang-web-scraping-using-scrapemate)

For the Low Level API see [books.toscrape.com](https://github.com/gosom/scrapemate/tree/main/examples/books-to-scrape-simple)

Additionally, for low level API you can read [the blogpost](https://blog.gkomninos.com/getting-started-with-web-scraping-using-golang-and-scrapemate)


See an example of how you can use `scrapemate` go scrape Google Maps: https://github.com/gosom/google-maps-scraper

## Contributing

Contributions are welcome.

## Licence

Scrapemate is licensed under the MIT License. See LICENCE file
