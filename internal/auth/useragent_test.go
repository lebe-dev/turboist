package auth

import "testing"

func TestParseUserAgent_Cases(t *testing.T) {
	cases := []struct {
		name        string
		ua          string
		wantBrowser string
		wantOS      string
		wantDisplay string
	}{
		{
			name:        "Chrome on macOS",
			ua:          "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
			wantBrowser: "Chrome",
			wantOS:      "macOS",
			wantDisplay: "Chrome on macOS",
		},
		{
			name:        "Firefox on Windows",
			ua:          "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:121.0) Gecko/20100101 Firefox/121.0",
			wantBrowser: "Firefox",
			wantOS:      "Windows",
			wantDisplay: "Firefox on Windows",
		},
		{
			name:        "Safari on iOS",
			ua:          "Mozilla/5.0 (iPhone; CPU iPhone OS 17_2 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.2 Mobile/15E148 Safari/604.1",
			wantBrowser: "Safari",
			wantOS:      "iOS",
			wantDisplay: "Safari on iOS",
		},
		{
			name:        "Edge wins over Chrome token",
			ua:          "Mozilla/5.0 (Windows NT 10.0) AppleWebKit/537.36 Chrome/120.0.0.0 Safari/537.36 Edg/120.0.0.0",
			wantBrowser: "Edge",
			wantOS:      "Windows",
			wantDisplay: "Edge on Windows",
		},
		{
			name:        "Chrome on iOS via CriOS",
			ua:          "Mozilla/5.0 (iPhone; CPU iPhone OS 16_6 like Mac OS X) AppleWebKit/605.1.15 CriOS/119.0.0.0 Mobile/15E148 Safari/604.1",
			wantBrowser: "Chrome",
			wantOS:      "iOS",
			wantDisplay: "Chrome on iOS",
		},
		{
			name:        "Firefox on Android",
			ua:          "Mozilla/5.0 (Android 13; Mobile; rv:120.0) Gecko/120.0 Firefox/120.0",
			wantBrowser: "Firefox",
			wantOS:      "Android",
			wantDisplay: "Firefox on Android",
		},
		{
			name:        "Linux Chrome",
			ua:          "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
			wantBrowser: "Chrome",
			wantOS:      "Linux",
			wantDisplay: "Chrome on Linux",
		},
		{
			name:        "Turboist iOS",
			ua:          "Turboist-iOS/1.2 (iPhone; iOS 17.2)",
			wantBrowser: "Turboist iOS",
			wantOS:      "iOS",
			wantDisplay: "Turboist iOS on iOS",
		},
		{
			name:        "Turboist CLI",
			ua:          "turboist-cli/0.4 (macOS)",
			wantBrowser: "Turboist CLI",
			wantOS:      "macOS",
			wantDisplay: "Turboist CLI on macOS",
		},
		{
			name:        "Empty",
			ua:          "",
			wantBrowser: "",
			wantOS:      "",
			wantDisplay: "Unknown",
		},
		{
			name:        "Unknown gibberish",
			ua:          "totally-not-a-real-ua",
			wantBrowser: "",
			wantOS:      "",
			wantDisplay: "Unknown",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseUserAgent(tc.ua)
			if got.Browser != tc.wantBrowser {
				t.Errorf("browser: got %q, want %q", got.Browser, tc.wantBrowser)
			}
			if got.OS != tc.wantOS {
				t.Errorf("os: got %q, want %q", got.OS, tc.wantOS)
			}
			if d := got.DisplayName(); d != tc.wantDisplay {
				t.Errorf("display: got %q, want %q", d, tc.wantDisplay)
			}
		})
	}
}
