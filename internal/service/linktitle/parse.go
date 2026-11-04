package linktitle

import (
	"encoding/json"
	"io"
	"net/url"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

const (
	// maxTitleRunes caps the page title before it goes into a template.
	maxTitleRunes = 200
	// maxDescriptionRunes caps the page summary copied into the description.
	maxDescriptionRunes = 1000
)

// parseHead reads the document up to the end of <head> (or the start of
// <body>) and picks the best title and summary it declares.
func parseHead(r io.Reader, u *url.URL) Page {
	z := html.NewTokenizer(r)
	meta := map[string]string{}
	var (
		docTitle, ldHeadline string
		inTitle, inLD        bool
		titleBuf, ldBuf      strings.Builder
	)
loop:
	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			break loop
		case html.StartTagToken, html.SelfClosingTagToken:
			name, hasAttr := z.TagName()
			switch atom.Lookup(name) {
			case atom.Body:
				break loop
			case atom.Title:
				if docTitle == "" && tt == html.StartTagToken {
					inTitle = true
				}
			case atom.Meta:
				if hasAttr {
					readMeta(z, meta)
				}
			case atom.Script:
				if hasAttr && ldHeadline == "" && isLDJSON(z) && tt == html.StartTagToken {
					inLD = true
				}
			case atom.Svg:
				// An inline SVG carries its own <title>; never take it for the page's.
				if tt == html.StartTagToken {
					skipElement(z, atom.Svg)
				}
			}
		case html.TextToken:
			if inTitle {
				titleBuf.Write(z.Text())
			} else if inLD {
				ldBuf.Write(z.Text())
			}
		case html.EndTagToken:
			name, _ := z.TagName()
			switch atom.Lookup(name) {
			case atom.Head:
				break loop
			case atom.Title:
				if inTitle {
					docTitle = trimSpaceCollapse(titleBuf.String())
					inTitle = false
				}
			case atom.Script:
				if inLD {
					ldHeadline = ldJSONHeadline(ldBuf.String())
					ldBuf.Reset()
					inLD = false
				}
			}
		}
	}

	p := Page{SiteName: trimSpaceCollapse(meta["og:site_name"])}
	p.Title = firstNonEmpty(meta["og:title"], meta["twitter:title"], ldHeadline, docTitle)
	p.Title = cleanTitle(p.Title, p.SiteName, u)
	p.Description = truncateRunes(trimSpaceCollapse(
		firstNonEmpty(meta["og:description"], meta["twitter:description"], meta["description"])), maxDescriptionRunes)
	return p
}

func readMeta(z *html.Tokenizer, meta map[string]string) {
	var key, content string
	for {
		k, v, more := z.TagAttr()
		switch string(k) {
		case "property", "name", "itemprop":
			if key == "" {
				key = strings.ToLower(strings.TrimSpace(string(v)))
			}
		case "content":
			content = string(v)
		}
		if !more {
			break
		}
	}
	if key == "" || content == "" {
		return
	}
	if _, seen := meta[key]; !seen {
		meta[key] = content
	}
}

func isLDJSON(z *html.Tokenizer) bool {
	for {
		k, v, more := z.TagAttr()
		if string(k) == "type" && strings.EqualFold(strings.TrimSpace(string(v)), "application/ld+json") {
			return true
		}
		if !more {
			return false
		}
	}
}

func skipElement(z *html.Tokenizer, a atom.Atom) {
	depth := 1
	for depth > 0 {
		switch z.Next() {
		case html.ErrorToken:
			return
		case html.StartTagToken:
			if n, _ := z.TagName(); atom.Lookup(n) == a {
				depth++
			}
		case html.EndTagToken:
			if n, _ := z.TagName(); atom.Lookup(n) == a {
				depth--
			}
		}
	}
}

// ldJSONHeadline digs the article headline out of a JSON-LD block: a single
// object, an array of them, or an @graph.
func ldJSONHeadline(raw string) string {
	var v any
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &v); err != nil {
		return ""
	}
	return findHeadline(v, 0)
}

func findHeadline(v any, depth int) string {
	if depth > 3 {
		return ""
	}
	switch t := v.(type) {
	case []any:
		for _, item := range t {
			if h := findHeadline(item, depth+1); h != "" {
				return h
			}
		}
	case map[string]any:
		if h, ok := t["headline"].(string); ok && strings.TrimSpace(h) != "" {
			return trimSpaceCollapse(h)
		}
		if g, ok := t["@graph"]; ok {
			return findHeadline(g, depth+1)
		}
	}
	return ""
}

// titleSeparators split "Article title | Site" style titles.
var titleSeparators = []string{" | ", " — ", " – ", " - ", " · ", " :: ", " / ", " » "}

// cleanTitle normalises whitespace and drops the site's name when the title
// carries it as a prefix or suffix ("How X works | Habr" → "How X works").
func cleanTitle(title, siteName string, u *url.URL) string {
	title = trimSpaceCollapse(title)
	if title == "" {
		return ""
	}
	names := []string{}
	if siteName != "" {
		names = append(names, siteName)
	}
	if u != nil {
		host := displayHost(u)
		names = append(names, host)
		if i := strings.IndexByte(host, '.'); i > 0 {
			names = append(names, host[:i])
		}
	}
	for _, sep := range titleSeparators {
		for _, n := range names {
			if n == "" {
				continue
			}
			if suffix := sep + n; len(title) > len(suffix) && strings.EqualFold(title[len(title)-len(suffix):], suffix) {
				return truncateRunes(strings.TrimSpace(title[:len(title)-len(suffix)]), maxTitleRunes)
			}
			if prefix := n + sep; len(title) > len(prefix) && strings.EqualFold(title[:len(prefix)], prefix) {
				return truncateRunes(strings.TrimSpace(title[len(prefix):]), maxTitleRunes)
			}
		}
	}
	return truncateRunes(title, maxTitleRunes)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}

func truncateRunes(s string, limit int) string {
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	r := []rune(s)
	return strings.TrimSpace(string(r[:limit-1])) + "…"
}
