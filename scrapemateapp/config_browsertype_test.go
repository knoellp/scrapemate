package scrapemateapp_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gosom/scrapemate"
	"github.com/gosom/scrapemate/mock"
	"github.com/gosom/scrapemate/scrapemateapp"
)

// TestWithJSBrowserType verifies that WithJSBrowserType stores the browser
// engine choice in JSOpts and that an empty/omitted value leaves the field
// at its zero value (backwards-compatible Chromium default path).
func TestWithJSBrowserType(t *testing.T) {
	t.Parallel()

	writer := &mock.MockResultWriter{}

	t.Run("default is empty string (chromium path)", func(t *testing.T) {
		cfg, err := scrapemateapp.NewConfig(
			[]scrapemate.ResultWriter{writer},
			scrapemateapp.WithJS(),
		)
		require.NoError(t, err)
		require.Equal(t, "", cfg.JSOpts.BrowserType,
			"BrowserType must be empty by default so existing callers use the Chromium path")
	})

	t.Run("firefox is stored as-is", func(t *testing.T) {
		cfg, err := scrapemateapp.NewConfig(
			[]scrapemate.ResultWriter{writer},
			scrapemateapp.WithJS(
				scrapemateapp.WithJSBrowserType("firefox"),
			),
		)
		require.NoError(t, err)
		require.Equal(t, "firefox", cfg.JSOpts.BrowserType)
	})

	t.Run("webkit is stored as-is", func(t *testing.T) {
		cfg, err := scrapemateapp.NewConfig(
			[]scrapemate.ResultWriter{writer},
			scrapemateapp.WithJS(
				scrapemateapp.WithJSBrowserType("webkit"),
			),
		)
		require.NoError(t, err)
		require.Equal(t, "webkit", cfg.JSOpts.BrowserType)
	})

	t.Run("chromium explicit", func(t *testing.T) {
		cfg, err := scrapemateapp.NewConfig(
			[]scrapemate.ResultWriter{writer},
			scrapemateapp.WithJS(
				scrapemateapp.WithJSBrowserType("chromium"),
			),
		)
		require.NoError(t, err)
		require.Equal(t, "chromium", cfg.JSOpts.BrowserType)
	})
}

// TestWithJSExecutablePath verifies that WithJSExecutablePath stores the path
// and that an omitted call leaves the field empty (no custom binary override).
func TestWithJSExecutablePath(t *testing.T) {
	t.Parallel()

	writer := &mock.MockResultWriter{}

	t.Run("default is empty (no custom binary)", func(t *testing.T) {
		cfg, err := scrapemateapp.NewConfig(
			[]scrapemate.ResultWriter{writer},
			scrapemateapp.WithJS(),
		)
		require.NoError(t, err)
		require.Equal(t, "", cfg.JSOpts.ExecutablePath)
	})

	t.Run("custom path is stored", func(t *testing.T) {
		cfg, err := scrapemateapp.NewConfig(
			[]scrapemate.ResultWriter{writer},
			scrapemateapp.WithJS(
				scrapemateapp.WithJSExecutablePath("/opt/camoufox/firefox"),
			),
		)
		require.NoError(t, err)
		require.Equal(t, "/opt/camoufox/firefox", cfg.JSOpts.ExecutablePath)
	})

	t.Run("browser type and executable path can be combined", func(t *testing.T) {
		cfg, err := scrapemateapp.NewConfig(
			[]scrapemate.ResultWriter{writer},
			scrapemateapp.WithJS(
				scrapemateapp.WithJSBrowserType("firefox"),
				scrapemateapp.WithJSExecutablePath("/opt/camoufox/firefox"),
			),
		)
		require.NoError(t, err)
		require.Equal(t, "firefox", cfg.JSOpts.BrowserType)
		require.Equal(t, "/opt/camoufox/firefox", cfg.JSOpts.ExecutablePath)
	})
}
