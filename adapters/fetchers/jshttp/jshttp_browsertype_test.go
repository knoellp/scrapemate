package jshttp

import (
	"testing"
)

// TestBrowsersToInstall verifies that browsersToInstall maps BrowserType
// strings to the correct Playwright browser list.  An empty string must map to
// Chromium so that existing callers that never set BrowserType are unaffected
// (regression guard for gmaps).
func TestBrowsersToInstall(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input string
		want  string
	}{
		{"", "chromium"},
		{"chromium", "chromium"},
		{"firefox", "firefox"},
		{"webkit", "webkit"},
		{"unknown", "chromium"}, // unknown values default to chromium
	}

	for _, tc := range cases {
		got := browsersToInstall(tc.input)
		if len(got) != 1 {
			t.Errorf("browsersToInstall(%q): expected slice of length 1, got %v", tc.input, got)
			continue
		}
		if got[0] != tc.want {
			t.Errorf("browsersToInstall(%q) = %q, want %q", tc.input, got[0], tc.want)
		}
	}
}
