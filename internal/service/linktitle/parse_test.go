package linktitle

import (
	"net/url"
	"strings"
	"testing"
)

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestParseHeadPrefersOpenGraph(t *testing.T) {
	doc := `<!doctype html><html><head>
<title>Fallback | Habr</title>
<meta property="og:site_name" content="Habr">
<meta property="og:title" content="  How   SQLite &amp; WAL work | Habr ">
<meta property="og:description" content="A deep dive.">
<meta name="description" content="ignored">
</head><body><meta property="og:title" content="from body"></body></html>`
	p := parseHead(strings.NewReader(doc), mustURL(t, "https://habr.com/ru/articles/1/"))
	if p.Title != "How SQLite & WAL work" {
		t.Errorf("title = %q", p.Title)
	}
	if p.Description != "A deep dive." {
		t.Errorf("description = %q", p.Description)
	}
	if p.SiteName != "Habr" {
		t.Errorf("site = %q", p.SiteName)
	}
}

func TestParseHeadTitleOnlyAndSvg(t *testing.T) {
	doc := `<html><head><svg><title>icon</title></svg><title>Release notes — example</title>
<meta name="description" content="What changed"></head></html>`
	p := parseHead(strings.NewReader(doc), mustURL(t, "https://www.example.com/notes"))
	if p.Title != "Release notes" {
		t.Errorf("title = %q", p.Title)
	}
	if p.Description != "What changed" {
		t.Errorf("description = %q", p.Description)
	}
}

func TestParseHeadJSONLD(t *testing.T) {
	doc := `<html><head><script type="application/ld+json">
{"@context":"https://schema.org","@graph":[{"@type":"WebSite","name":"x"},{"@type":"NewsArticle","headline":"Big news"}]}
</script><title>Site</title></head></html>`
	p := parseHead(strings.NewReader(doc), mustURL(t, "https://news.example.com/a"))
	if p.Title != "Big news" {
		t.Errorf("title = %q", p.Title)
	}
}

func TestParseHeadNothing(t *testing.T) {
	p := parseHead(strings.NewReader(`<html><body><div id="app"></div></body></html>`), mustURL(t, "https://spa.example.com/"))
	if p.Title != "" {
		t.Errorf("title = %q", p.Title)
	}
}

func TestParseHeadTruncates(t *testing.T) {
	long := strings.Repeat("слово ", 100)
	p := parseHead(strings.NewReader(`<title>`+long+`</title>`), mustURL(t, "https://a.example.com/"))
	if n := len([]rune(p.Title)); n > maxTitleRunes {
		t.Errorf("title has %d runes", n)
	}
	if !strings.HasSuffix(p.Title, "…") {
		t.Errorf("title not marked as truncated: %q", p.Title)
	}
}

func TestParseMarkdown(t *testing.T) {
	cases := map[string]struct{ in, title, desc string }{
		"jina": {in: "Title: Rendered title\n\nURL Source: https://x.y/\n\nMarkdown Content:\n# Other", title: "Rendered title"},
		"front matter": {
			in:    "---\ntitle: \"Front title\"\ndescription: Short summary\n---\n\n# Heading\n",
			title: "Front title", desc: "Short summary",
		},
		"heading": {in: "Some intro text\n\n# The heading\n\nbody", title: "The heading"},
		"empty":   {in: "just text", title: ""},
	}
	for name, c := range cases {
		p := parseMarkdown(strings.NewReader(c.in))
		if p.Title != c.title || p.Description != c.desc {
			t.Errorf("%s: got %+v", name, p)
		}
	}
}
