package jshttp

import (
	"context"
	"net/url"

	"github.com/playwright-community/playwright-go"

	"github.com/gosom/scrapemate"
	playwrightadapter "github.com/gosom/scrapemate/adapters/browsers/playwright"
)

var _ scrapemate.HTTPFetcher = (*jsFetch)(nil)

type JSFetcherOptions struct {
	Headless          bool
	DisableImages     bool
	Rotator           scrapemate.ProxyRotator
	PoolSize          int
	PageReuseLimit    int
	BrowserReuseLimit int
	UserAgent         string
	// BrowserType selects which Playwright browser engine to use.
	// Accepted values: "chromium" (default, empty string), "firefox", "webkit".
	BrowserType string
	// ExecutablePath, when non-empty, overrides the browser binary path.
	// Use this to point at a custom binary such as Camoufox.
	ExecutablePath string
	// DisableSingleProcess, when true, omits the --single-process Chromium
	// launch flag.  Set this when using per-job proxies (ProxyProvider): in
	// single-process mode all BrowserContexts share one renderer process and
	// closing a per-job context tears down the shared renderer, breaking
	// subsequent fetches.  Has no effect on Firefox or WebKit.
	// Default (false) preserves the existing --single-process behaviour.
	DisableSingleProcess bool
}

// browsersToInstall returns the Playwright browser list for the given
// BrowserType value.  An empty string or "chromium" both map to Chromium so
// that callers that never set BrowserType are unaffected.
func browsersToInstall(bt string) []string {
	switch bt {
	case "firefox":
		return []string{"firefox"}
	case "webkit":
		return []string{"webkit"}
	default:
		return []string{"chromium"}
	}
}

func New(params JSFetcherOptions) (scrapemate.HTTPFetcher, error) {
	opts := []*playwright.RunOptions{
		{
			Browsers: browsersToInstall(params.BrowserType),
			Verbose:  true,
		},
	}

	if err := playwright.Install(opts...); err != nil {
		return nil, err
	}

	pw, err := playwright.Run()
	if err != nil {
		return nil, err
	}

	var pool *ProxyPool

	if params.Rotator != nil {
		proxies := params.Rotator.Proxies()

		if len(proxies) > 0 {
			pool, err = NewProxyPool(proxies)
			if err != nil {
				return nil, err
			}
		}
	}

	ans := jsFetch{
		pw:                   pw,
		headless:             params.Headless,
		disableImages:        params.DisableImages,
		pool:                 make(chan *browser, params.PoolSize),
		rotator:              params.Rotator,
		pageReuseLimit:       params.PageReuseLimit,
		browserReuseLimit:    params.BrowserReuseLimit,
		ua:                   params.UserAgent,
		proxyPool:            pool,
		browserType:          params.BrowserType,
		executablePath:       params.ExecutablePath,
		disableSingleProcess: params.DisableSingleProcess,
	}

	for range params.PoolSize {
		b, err := newBrowser(pw, params.Headless, params.DisableImages, ans.proxyPool, params.UserAgent, params.BrowserType, params.ExecutablePath, params.DisableSingleProcess)
		if err != nil {
			_ = ans.Close()
			return nil, err
		}

		ans.pool <- b
	}

	return &ans, nil
}

type jsFetch struct {
	pw                   *playwright.Playwright
	headless             bool
	disableImages        bool
	pool                 chan *browser
	rotator              scrapemate.ProxyRotator
	pageReuseLimit       int
	browserReuseLimit    int
	ua                   string
	proxyPool            *ProxyPool
	browserType          string
	executablePath       string
	disableSingleProcess bool
}

func (o *jsFetch) GetBrowser(ctx context.Context) (*browser, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case ans := <-o.pool:
		if ans.browser.IsConnected() && (o.browserReuseLimit <= 0 || ans.browserUsage < o.browserReuseLimit) {
			return ans, nil
		}

		ans.browser.Close()
	default:
	}

	return newBrowser(o.pw, o.headless, o.disableImages, o.proxyPool, o.ua, o.browserType, o.executablePath, o.disableSingleProcess)
}

func (o *jsFetch) Close() error {
	close(o.pool)

	for b := range o.pool {
		b.Close()
	}

	_ = o.pw.Stop()

	return nil
}

func (o *jsFetch) PutBrowser(ctx context.Context, b *browser) {
	if !b.browser.IsConnected() {
		b.Close()

		return
	}

	select {
	case <-ctx.Done():
		b.Close()
	case o.pool <- b:
	default:
		b.Close()
	}
}

// Fetch fetches the url specicied by the job and returns the response.
// If the job implements scrapemate.ProxyProvider and returns a non-empty URL,
// a fresh per-job BrowserContext is created on a pool browser with that proxy
// (Strategie C: browser reuse + per-fetch context). Otherwise the existing
// pool-browser + pool-context path is used unchanged.
func (o *jsFetch) Fetch(ctx context.Context, job scrapemate.IJob) scrapemate.Response {
	jobProxyURL := scrapemate.ResolveJobProxyURL(job)

	if jobProxyURL != "" {
		// R1.5: per-job proxy → fresh short-lived BrowserContext on a pool browser.
		return o.fetchWithJobProxy(ctx, job, jobProxyURL)
	}

	// Unchanged path: pool browser with pool context (round-robin proxy).
	return o.fetchDefault(ctx, job)
}

// fetchDefault is the original Fetch logic: pool browser + pool context with
// page reuse. Unchanged from pre-R1.5 behaviour.
func (o *jsFetch) fetchDefault(ctx context.Context, job scrapemate.IJob) scrapemate.Response {
	br, err := o.GetBrowser(ctx)
	if err != nil {
		return scrapemate.Response{
			Error: err,
		}
	}

	defer o.PutBrowser(ctx, br)

	if job.GetTimeout() > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, job.GetTimeout())

		defer cancel()
	}

	var page playwright.Page

	if len(br.ctx.Pages()) > 0 {
		page = br.ctx.Pages()[0]

		for i := 1; i < len(br.ctx.Pages()); i++ {
			br.ctx.Pages()[i].Close()
		}
	} else {
		page, err = br.ctx.NewPage()
		if err != nil {
			return scrapemate.Response{
				Error: err,
			}
		}
	}

	// match the browser default timeout to the job timeout
	if job.GetTimeout() > 0 {
		page.SetDefaultTimeout(float64(job.GetTimeout().Milliseconds()))
	}

	br.page0Usage++
	br.browserUsage++

	defer func() {
		if o.pageReuseLimit == 0 || br.page0Usage >= o.pageReuseLimit {
			_ = page.Close()

			br.page0Usage = 0
		}
	}()

	wrappedPage := playwrightadapter.NewPage(page)

	return job.BrowserActions(ctx, wrappedPage)
}

// fetchWithJobProxy creates a fresh BrowserContext on a pool browser using the
// job-specific proxy URL. The browser is returned to the pool after the fetch;
// the context is closed immediately (no page reuse for per-job contexts).
// browserUsage is incremented so BrowserReuseLimit continues to apply.
func (o *jsFetch) fetchWithJobProxy(ctx context.Context, job scrapemate.IJob, jobProxyURL string) scrapemate.Response {
	br, err := o.GetBrowser(ctx)
	if err != nil {
		return scrapemate.Response{Error: err}
	}

	defer o.PutBrowser(ctx, br)

	// Apply job timeout.
	if job.GetTimeout() > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, job.GetTimeout())

		defer cancel()
	}

	const defaultWidth, defaultHeight = 1920, 1080

	ua := o.ua
	if ua == "" {
		ua = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36"
	}

	// Fresh per-job BrowserContext with the job-specific proxy.
	jobCtx, err := br.browser.NewContext(playwright.BrowserNewContextOptions{
		UserAgent: playwright.String(ua),
		Viewport: &playwright.Size{
			Width:  defaultWidth,
			Height: defaultHeight,
		},
		Proxy: parseProxyURL(jobProxyURL),
	})
	if err != nil {
		return scrapemate.Response{Error: err}
	}

	defer jobCtx.Close()

	page, err := jobCtx.NewPage()
	if err != nil {
		return scrapemate.Response{Error: err}
	}

	defer page.Close()

	if job.GetTimeout() > 0 {
		page.SetDefaultTimeout(float64(job.GetTimeout().Milliseconds()))
	}

	// Count browser usage so BrowserReuseLimit continues to apply.
	br.browserUsage++

	wrappedPage := playwrightadapter.NewPage(page)

	return job.BrowserActions(ctx, wrappedPage)
}

// parseProxyURL converts a full proxy URL (http://user:pass@host:port) into a
// *playwright.Proxy suitable for BrowserNewContextOptions.Proxy.
// Returns nil if the URL is empty or unparseable.
func parseProxyURL(rawURL string) *playwright.Proxy {
	if rawURL == "" {
		return nil
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return nil
	}

	// Server is scheme + host (without credentials).
	server := u.Scheme + "://" + u.Host

	proxy := &playwright.Proxy{
		Server: server,
	}

	if u.User != nil {
		username := u.User.Username()
		if username != "" {
			proxy.Username = playwright.String(username)
		}

		password, hasPassword := u.User.Password()
		if hasPassword {
			proxy.Password = playwright.String(password)
		}
	}

	return proxy
}

type browser struct {
	browser      playwright.Browser
	ctx          playwright.BrowserContext
	page0Usage   int
	browserUsage int
}

func (o *browser) Close() {
	_ = o.ctx.Close()
	_ = o.browser.Close()
}

func selectBrowserType(pw *playwright.Playwright, bt string) playwright.BrowserType {
	switch bt {
	case "firefox":
		return pw.Firefox
	case "webkit":
		return pw.WebKit
	default:
		return pw.Chromium
	}
}

// buildChromiumArgs builds the Chromium launch-flag slice.
//
// disableImages appends --blink-settings=imagesEnabled=false when true.
// disableSingleProcess omits --single-process when true; when false (the
// default) --single-process is included, preserving the upstream default.
//
// Extracting the flag list into a testable helper allows unit tests to verify
// the backward-compat contract (--single-process present by default, absent
// when opted out) without launching a real browser.
func buildChromiumArgs(disableImages, disableSingleProcess bool) []string {
	args := []string{
		`--start-maximized`,
		`--no-default-browser-check`,
		`--disable-dev-shm-usage`,
		`--no-sandbox`,
		`--disable-setuid-sandbox`,
		`--no-zygote`,
		`--disable-gpu`,
		`--mute-audio`,
		`--disable-extensions`,
		`--disable-breakpad`,
		`--disable-features=TranslateUI,BlinkGenPropertyTrees`,
		`--disable-ipc-flooding-protection`,
		`--enable-features=NetworkService,NetworkServiceInProcess`,
		"--enable-features=NetworkService",
		`--disable-default-apps`,
		`--disable-notifications`,
		`--disable-webgl`,
		`--disable-blink-features=AutomationControlled`,
		"--ignore-certificate-errors",
		"--ignore-certificate-errors-spki-list",
		"--disable-web-security",
	}
	if !disableSingleProcess {
		args = append(args, "--single-process")
	}
	if disableImages {
		args = append(args, `--blink-settings=imagesEnabled=false`)
	}
	return args
}

func newBrowser(pw *playwright.Playwright, headless, disableImages bool, proxyPool *ProxyPool, ua, browserType, executablePath string, disableSingleProcess bool) (*browser, error) {
	opts := playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(headless),
		Args:     buildChromiumArgs(disableImages, disableSingleProcess),
	}
	if executablePath != "" {
		opts.ExecutablePath = playwright.String(executablePath)
	}

	bt := selectBrowserType(pw, browserType)

	br, err := bt.Launch(opts)
	if err != nil {
		return nil, err
	}

	const defaultWidth, defaultHeight = 1920, 1080

	bctx, err := br.NewContext(playwright.BrowserNewContextOptions{
		UserAgent: func() *string {
			if ua == "" {
				defaultUA := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36"

				return &defaultUA
			}

			return &ua
		}(),
		Viewport: &playwright.Size{
			Width:  defaultWidth,
			Height: defaultHeight,
		},
		Proxy: func() *playwright.Proxy {
			if proxyPool != nil {
				authProxy := proxyPool.Next()

				addr := authProxy.Address()

				return &playwright.Proxy{
					Server: addr,
				}
			}

			return nil
		}(),
	})
	if err != nil {
		return nil, err
	}

	ans := browser{
		browser: br,
		ctx:     bctx,
	}

	return &ans, nil
}
