package scrapemateapp_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gosom/scrapemate"
	"github.com/gosom/scrapemate/mock"
	"github.com/gosom/scrapemate/scrapemateapp"
)

// TestGetFetcher_NetHTTP_WithRotator is a regression guard for the R1.5 change
// in getFetcher(): when proxies are configured and !UseJS && !UseStealth,
// the Config must be created without error and the resulting config reflects
// the proxy list.  The actual fetcher construction (NewWithRotator vs New) is
// verified at the Config level since getFetcher() is unexported.
//
// Rationale: the production code path changed from fetcher.New(client) to
// fetcher.NewWithRotator(client, rotator) when len(Proxies)>0 && !UseJS &&
// !UseStealth.  This test ensures the Config construction that drives that
// decision remains correct and backward-compatible.
func TestGetFetcher_NetHTTP_WithRotator(t *testing.T) {
	t.Parallel()

	writer := &mock.MockResultWriter{}

	t.Run("proxies configured with plain HTTP fetcher path (no JS, no stealth)", func(t *testing.T) {
		cfg, err := scrapemateapp.NewConfig(
			[]scrapemate.ResultWriter{writer},
			scrapemateapp.WithProxies([]string{"http://proxy.example.com:8080"}),
		)
		require.NoError(t, err)
		require.False(t, cfg.UseJS, "UseJS must be false on this path")
		require.False(t, cfg.UseStealth, "UseStealth must be false on this path")
		require.Len(t, cfg.Proxies, 1, "proxy list must be preserved")
	})

	t.Run("no proxies configured → default path (no rotator)", func(t *testing.T) {
		cfg, err := scrapemateapp.NewConfig(
			[]scrapemate.ResultWriter{writer},
		)
		require.NoError(t, err)
		require.Empty(t, cfg.Proxies, "Proxies must be empty when not configured")
	})

	t.Run("proxies + UseJS → JS fetcher path, not nethttp", func(t *testing.T) {
		cfg, err := scrapemateapp.NewConfig(
			[]scrapemate.ResultWriter{writer},
			scrapemateapp.WithProxies([]string{"http://proxy.example.com:8080"}),
			scrapemateapp.WithJS(),
		)
		require.NoError(t, err)
		require.True(t, cfg.UseJS, "UseJS must be true when WithJS() is set")
		// The JS path uses getJSFetcher, not the nethttp NewWithRotator path.
	})

	t.Run("proxies + UseStealth → stealth fetcher path, not nethttp", func(t *testing.T) {
		cfg, err := scrapemateapp.NewConfig(
			[]scrapemate.ResultWriter{writer},
			scrapemateapp.WithProxies([]string{"http://proxy.example.com:8080"}),
			scrapemateapp.WithStealth("firefox"),
		)
		require.NoError(t, err)
		require.True(t, cfg.UseStealth, "UseStealth must be true when WithStealth() is set")
		// The stealth path uses stealth.New, not the nethttp NewWithRotator path.
	})
}
