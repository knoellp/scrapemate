package scrapemate

// ProxyProvider is an optional capability for jobs that need their own
// proxy URL per-job, overriding the app-level WithProxies round-robin.
//
// Implement this on your job type if you need:
//   - sticky-session proxy IDs (one IP per logical session)
//   - account-specific routing
//   - geo-targeting per job
//
// Example:
//
//	type MyJob struct {
//	    scrapemate.Job
//	    SessionID string
//	}
//
//	func (j *MyJob) GetProxyURL() string {
//	    return fmt.Sprintf("http://user-session-%s:pass@gate.example.com:7000", j.SessionID)
//	}
//
// Jobs that don't implement this interface continue to use the app-level
// WithProxies round-robin as before. Empty return strings are also treated
// as "no preference" and fall back to round-robin.
//
// Fetchers that currently support per-job proxy: jshttp, stealth, nethttp.
type ProxyProvider interface {
	GetProxyURL() string
}

// ResolveJobProxyURL returns the per-job proxy URL if the job implements
// ProxyProvider and returns a non-empty URL, otherwise "" (meaning: caller
// should fall back to its app-level proxy rotation).
//
// Fetchers call this once per Fetch() before deciding which proxy to use.
// Centralising the type-assertion keeps the contract identical across
// jshttp / stealth / nethttp.
func ResolveJobProxyURL(job IJob) string {
	if provider, ok := job.(ProxyProvider); ok {
		return provider.GetProxyURL()
	}

	return ""
}
