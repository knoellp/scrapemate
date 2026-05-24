package scrapemate

// RequestHookProvider is an optional capability for BrowserPage implementations
// that support intercepting outgoing browser requests and responses.
//
// This interface is intentionally separate from BrowserPage so that it can be
// added without breaking existing BrowserPage implementations. Consumers check
// for the capability via a type assertion:
//
//	func (j *MyJob) BrowserActions(ctx context.Context, page scrapemate.BrowserPage) scrapemate.Response {
//	    if hook, ok := page.(scrapemate.RequestHookProvider); ok {
//	        hook.OnRequest(func(url string, headers map[string]string) {
//	            if strings.Contains(url, "/api/auth") {
//	                // headers keys are lower-cased (non-blocking, no round-trip)
//	                captureToken(headers["authorization"])
//	            }
//	        })
//	    }
//	    page.Goto(j.URL, scrapemate.WaitUntilNetworkIdle)
//	    // ...
//	}
//
// The jshttp adapter (Playwright-based) implements this interface. Other adapters
// may or may not — always check via type assertion before use so code remains
// forward-compatible with adapters that do not support request interception.
//
// Practical use case: capturing Bearer tokens emitted by SPA auth wrappers.
// When a JavaScript SPA fetches /api/v3/ resources it typically attaches an
// Authorization header client-side.  Registering an OnRequest handler lets the
// job capture that header without any Unwrap() cast to the underlying library type.
//
// Example — jameda-style token capture:
//
//	hook.OnRequest(func(url string, headers map[string]string) {
//	    if !strings.Contains(url, "/api/v3/") {
//	        return
//	    }
//	    // headers keys arrive lower-cased; direct lookup is safe.
//	    auth := headers["authorization"]
//	    if strings.HasPrefix(auth, "Bearer ") {
//	        token := strings.TrimPrefix(auth, "Bearer ")
//	        captureToken(token)
//	    }
//	})
type RequestHookProvider interface {
	// OnRequest registers a handler that is called for every outgoing browser
	// request.  url is the full request URL; headers is a map of request headers
	// with lower-cased keys.
	//
	// The handler is called synchronously in the browser event loop.  It MUST
	// NOT perform blocking I/O or blocking Playwright calls (e.g. Page.Eval,
	// Response.AllHeaders).  Non-blocking operations such as channel sends and
	// atomic stores are safe.
	OnRequest(handler func(url string, headers map[string]string))

	// OnResponse registers a handler that is called for every browser response.
	// url is the response URL; statusCode is the HTTP status; headers is a map
	// of response headers with lower-cased keys.
	//
	// Same threading constraints apply as for OnRequest: the handler runs in
	// the browser event loop and must not block.
	OnResponse(handler func(url string, statusCode int, headers map[string]string))
}
