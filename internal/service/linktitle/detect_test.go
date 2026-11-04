package linktitle

import (
	"net/netip"
	"net/url"
	"strings"
	"testing"

	"github.com/lebe-dev/turboist/internal/model"
)

func TestDetectURL(t *testing.T) {
	cases := []struct {
		title string
		want  bool
	}{
		{"https://habr.com/ru/articles/123/", true},
		{"  https://example.org/a?b=c#d  ", true},
		{"HTTPS://Example.org/", true},
		{"https://пример.рф/статья", true},
		{"https://example.org:443/x", true},
		{"http://example.org/", false},                  // https only
		{"example.org", false},                          // no scheme
		{"read https://example.org/", false},            // text before
		{"https://example.org/ later", false},           // text after
		{"https://example.org/\nnext", false},           // multi-line
		{"https://127.0.0.1/", false},                   // IP literal
		{"https://[::1]/", false},                       // IPv6 literal
		{"https://2130706433/", false},                  // numeric host
		{"https://0x7f.0x1/", false},                    // hex host
		{"https://localhost/", false},                   // single label
		{"https://router/", false},                      // single label
		{"https://nas.local/", false},                   // reserved suffix
		{"https://grafana.internal/", false},            // reserved suffix
		{"https://box.home.arpa/", false},               // reserved suffix
		{"https://example.org:8443/", false},            // non-default port
		{"https://user:pass@example.org/", false},       // credentials
		{"https://exa_mple.org/", false},                // invalid label
		{"https://" + strings.Repeat("a", 2100), false}, // too long
	}
	for _, c := range cases {
		_, got := DetectURL(c.title)
		if got != c.want {
			t.Errorf("DetectURL(%q) = %v, want %v", c.title, got, c.want)
		}
	}
}

func TestIsPublicAddr(t *testing.T) {
	cases := map[string]bool{
		"93.184.216.34":        true,
		"2606:4700:4700::1111": true,
		"127.0.0.1":            false,
		"10.1.2.3":             false,
		"172.16.0.1":           false,
		"192.168.1.1":          false,
		"169.254.169.254":      false, // cloud metadata
		"100.100.1.1":          false, // CGNAT / tailnet
		"0.0.0.0":              false,
		"::1":                  false,
		"fd00::1":              false,
		"fe80::1":              false,
		"::ffff:127.0.0.1":     false, // v4-mapped loopback
		"64:ff9b::a00:1":       false, // NAT64 of 10.0.0.1
		"64:ff9b::5db8:d822":   true,  // NAT64 of 93.184.216.34
		"224.0.0.1":            false,
		"198.18.0.5":           false,
		"2001:db8::1":          false,
	}
	for s, want := range cases {
		if got := IsPublicAddr(netip.MustParseAddr(s)); got != want {
			t.Errorf("IsPublicAddr(%s) = %v, want %v", s, got, want)
		}
	}
}

func TestGuardDialRefusesPrivate(t *testing.T) {
	if err := guardDial("tcp", "127.0.0.1:443", nil); err == nil {
		t.Fatal("loopback must be refused")
	}
	if err := guardDial("tcp", "[fd00::1]:443", nil); err == nil {
		t.Fatal("ULA must be refused")
	}
	if err := guardDial("tcp", "93.184.216.34:443", nil); err != nil {
		t.Fatalf("public address refused: %v", err)
	}
}

func TestNormalizeDomain(t *testing.T) {
	ok := map[string]string{
		"youtube.com":                "youtube.com",
		" https://www.YouTube.com/ ": "youtube.com",
		"*.youtube.com":              "youtube.com",
		"m.youtube.com/watch?v=1":    "m.youtube.com",
		"youtu.be":                   "youtu.be",
	}
	for in, want := range ok {
		got, err := NormalizeDomain(in)
		if err != nil || got != want {
			t.Errorf("NormalizeDomain(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "localhost", "10.0.0.1", "foo bar.com", "nas.local"} {
		if _, err := NormalizeDomain(bad); err == nil {
			t.Errorf("NormalizeDomain(%q) accepted", bad)
		}
	}
}

func TestNormalizeRules(t *testing.T) {
	rules, _, err := NormalizeRules([]model.LinkTitleRule{
		{Domain: "www.youtube.com", Template: " Посмотреть «{title}» "},
		{Domain: "youtube.com", Template: "dup {title}"},
	})
	if err != nil {
		t.Fatalf("NormalizeRules: %v", err)
	}
	if len(rules) != 1 || rules[0].Domain != "youtube.com" || rules[0].Template != "Посмотреть «{title}»" {
		t.Fatalf("rules = %+v", rules)
	}
	if _, idx, err := NormalizeRules([]model.LinkTitleRule{{Domain: "a.com", Template: "{title}"}, {Domain: "b.com", Template: "no placeholder"}}); err == nil || idx != 1 {
		t.Fatalf("missing placeholder: idx=%d err=%v", idx, err)
	}
}

func TestResolveTemplate(t *testing.T) {
	s := model.LinkTitlesSettings{Rules: []model.LinkTitleRule{
		{Domain: "youtube.com", Template: "Посмотреть «{title}»"},
		{Domain: "music.youtube.com", Template: "Послушать «{title}»"},
	}}
	parse := func(raw string) *url.URL { u, _ := url.Parse(raw); return u }
	if got := ResolveTemplate(s, parse("https://www.youtube.com/watch?v=1"), "ru"); got != "Посмотреть «{title}»" {
		t.Errorf("subdomain match: %q", got)
	}
	if got := ResolveTemplate(s, parse("https://music.youtube.com/x"), "ru"); got != "Послушать «{title}»" {
		t.Errorf("most specific match: %q", got)
	}
	if got := ResolveTemplate(s, parse("https://notyoutube.com/"), "ru"); got != "Статья «{title}»" {
		t.Errorf("suffix must match on a label boundary: %q", got)
	}
	if got := ResolveTemplate(s, parse("https://habr.com/"), "en"); got != "Article “{title}”" {
		t.Errorf("english default: %q", got)
	}
	s.Template = "Read: {title} ({domain})"
	if got := RenderTitle(ResolveTemplate(s, parse("https://www.habr.com/"), "ru"), "Go", parse("https://www.habr.com/")); got != "Read: Go (habr.com)" {
		t.Errorf("custom default: %q", got)
	}
}

func TestFallbackTitle(t *testing.T) {
	u, _ := url.Parse("https://www.example.com/blog/%D0%BF%D0%BE%D1%81%D1%82/")
	if got := FallbackTitle(u); got != "example.com/blog/пост" {
		t.Fatalf("FallbackTitle = %q", got)
	}
}

func TestComposeDescription(t *testing.T) {
	got := ComposeDescription("https://a.com/x", "Summary", " typed note ")
	if got != "https://a.com/x\n\nSummary\n\ntyped note" {
		t.Fatalf("ComposeDescription = %q", got)
	}
	if got := ComposeDescription("https://a.com/x", "", ""); got != "https://a.com/x" {
		t.Fatalf("link only = %q", got)
	}
}
