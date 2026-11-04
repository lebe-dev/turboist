package linktitle

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// maxRenderedBytes is how much of a rendered page is read.
	maxRenderedBytes = 2 << 20
	// URLPlaceholder marks where the link goes in a renderer URL template.
	URLPlaceholder = "{url}"
)

// Renderer asks an external "URL → Markdown" service (markdown.new, Jina
// Reader, a self-hosted headless browser, …) for a page the server could not
// read itself — typically one that only builds its title in JavaScript.
//
// The service is configured by the operator (LINK_TITLES_RENDERER_URL), so it
// is reached through the ordinary client and may sit on a private network; only
// the link it is given has passed the public-URL checks.
type Renderer struct {
	template string
	client   *http.Client
}

// NewRenderer returns nil for an empty template, i.e. no fallback.
func NewRenderer(template string, timeout time.Duration) *Renderer {
	template = strings.TrimSpace(template)
	if template == "" {
		return nil
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &Renderer{template: template, client: &http.Client{Timeout: timeout}}
}

// requestURL substitutes the link into the template, or appends it when the
// template has no placeholder ("https://markdown.new/" + link).
func (r *Renderer) requestURL(u *url.URL) string {
	if strings.Contains(r.template, URLPlaceholder) {
		return strings.ReplaceAll(r.template, URLPlaceholder, u.String())
	}
	return r.template + u.String()
}

func (r *Renderer) Fetch(ctx context.Context, u *url.URL) (Page, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.requestURL(u), nil)
	if err != nil {
		return Page{}, &FetchError{Err: fmt.Errorf("renderer request: %w", err)}
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/markdown, text/plain;q=0.9, */*;q=0.5")
	resp, err := r.client.Do(req)
	if err != nil {
		return Page{}, &FetchError{Transient: true, Err: fmt.Errorf("renderer: %w", err)}
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return Page{}, &FetchError{Transient: true, Err: fmt.Errorf("renderer status %d", resp.StatusCode)}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return Page{}, &FetchError{Err: fmt.Errorf("renderer status %d", resp.StatusCode)}
	}
	p := parseMarkdown(io.LimitReader(resp.Body, maxRenderedBytes))
	p.Title = cleanTitle(p.Title, p.SiteName, u)
	return p, nil
}

// parseMarkdown understands the shapes the common renderers produce: YAML front
// matter (title:, description:), Jina's "Title: …" header lines, and failing
// both the first "# " heading.
func parseMarkdown(r io.Reader) Page {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	var (
		p                  Page
		heading            string
		inFront, lineCount = false, 0
	)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		lineCount++
		if lineCount == 1 && line == "---" {
			inFront = true
			continue
		}
		if inFront {
			if line == "---" {
				inFront = false
				continue
			}
			if v, ok := header(line, "title:"); ok && p.Title == "" {
				p.Title = unquote(v)
			} else if v, ok := header(line, "description:"); ok && p.Description == "" {
				p.Description = unquote(v)
			}
			continue
		}
		if v, ok := header(line, "Title:"); ok && p.Title == "" {
			p.Title = v
			continue
		}
		if v, ok := header(line, "Description:"); ok && p.Description == "" {
			p.Description = v
			continue
		}
		if heading == "" && strings.HasPrefix(line, "# ") {
			heading = strings.TrimSpace(line[2:])
		}
		if p.Title != "" || (heading != "" && lineCount > 50) {
			break
		}
	}
	if p.Title == "" {
		p.Title = heading
	}
	p.Title = trimSpaceCollapse(p.Title)
	p.Description = truncateRunes(trimSpaceCollapse(p.Description), maxDescriptionRunes)
	return p
}

func header(line, key string) (string, bool) {
	if len(line) <= len(key) || !strings.EqualFold(line[:len(key)], key) {
		return "", false
	}
	return strings.TrimSpace(line[len(key):]), true
}

func unquote(s string) string {
	if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') && s[len(s)-1] == s[0] {
		return s[1 : len(s)-1]
	}
	return s
}
