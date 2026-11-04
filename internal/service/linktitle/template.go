package linktitle

import (
	"errors"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/lebe-dev/turboist/internal/model"
)

// defaultTemplates are the built-in title templates per UI language.
var defaultTemplates = map[string]string{
	"ru": "Статья «{title}»",
	"en": "Article “{title}”",
}

// DefaultTemplate is the built-in template for a language, English when the
// language has none.
func DefaultTemplate(locale string) string {
	if t, ok := defaultTemplates[strings.ToLower(locale)]; ok {
		return t
	}
	return defaultTemplates["en"]
}

// ResolveTemplate picks the template for a link: the most specific matching
// domain rule, else the configured default, else the built-in one.
func ResolveTemplate(s model.LinkTitlesSettings, u *url.URL, locale string) string {
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	best, bestLen := "", -1
	for _, r := range s.Rules {
		d := r.Domain
		if d == "" || r.Template == "" {
			continue
		}
		if (host == d || strings.HasSuffix(host, "."+d)) && len(d) > bestLen {
			best, bestLen = r.Template, len(d)
		}
	}
	if best != "" {
		return best
	}
	if strings.TrimSpace(s.Template) != "" {
		return s.Template
	}
	return DefaultTemplate(locale)
}

// RenderTitle fills a template with the page title and the link's host.
func RenderTitle(template, title string, u *url.URL) string {
	out := strings.ReplaceAll(template, TitlePlaceholder, title)
	out = strings.ReplaceAll(out, DomainPlaceholder, displayHost(u))
	return strings.TrimSpace(out)
}

// ComposeDescription puts the link first, then the page summary, then whatever
// the task's description already said.
func ComposeDescription(link, summary, existing string) string {
	parts := make([]string, 0, 3)
	for _, p := range []string{link, summary, existing} {
		if p = strings.TrimSpace(p); p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, "\n\n")
}

// Validation errors of the settings endpoint.
var (
	ErrTemplateInvalid = errors.New("template must contain {title} and be at most 200 characters")
	ErrDomainInvalid   = errors.New("domain must be a public domain name, e.g. youtube.com")
	ErrTooManyRules    = errors.New("at most 100 domain rules")
)

// NormalizeTemplate trims a template and checks it. An empty one is valid and
// means "built-in default".
func NormalizeTemplate(t string) (string, error) {
	t = strings.TrimSpace(t)
	if t == "" {
		return "", nil
	}
	if !strings.Contains(t, TitlePlaceholder) || utf8.RuneCountInString(t) > maxTemplateLength {
		return "", ErrTemplateInvalid
	}
	return t, nil
}

// NormalizeDomain accepts what people paste — "https://www.YouTube.com/",
// "*.youtube.com" — and reduces it to the bare domain a rule matches by.
func NormalizeDomain(d string) (string, error) {
	d = strings.ToLower(strings.TrimSpace(d))
	for _, p := range []string{"https://", "http://"} {
		d = strings.TrimPrefix(d, p)
	}
	if i := strings.IndexAny(d, "/?#"); i >= 0 {
		d = d[:i]
	}
	d = strings.TrimPrefix(d, "*.")
	d = strings.TrimPrefix(d, "www.")
	d = strings.TrimSuffix(d, ".")
	if !IsPublicHostname(d) {
		return "", ErrDomainInvalid
	}
	return d, nil
}

// NormalizeRules validates and normalises a whole rule list. Later duplicates
// of a domain are dropped.
func NormalizeRules(rules []model.LinkTitleRule) ([]model.LinkTitleRule, int, error) {
	if len(rules) > maxRules {
		return nil, -1, ErrTooManyRules
	}
	out := make([]model.LinkTitleRule, 0, len(rules))
	seen := map[string]bool{}
	for i, r := range rules {
		d, err := NormalizeDomain(r.Domain)
		if err != nil {
			return nil, i, err
		}
		t, err := NormalizeTemplate(r.Template)
		if err != nil || t == "" {
			return nil, i, ErrTemplateInvalid
		}
		if seen[d] {
			continue
		}
		seen[d] = true
		out = append(out, model.LinkTitleRule{Domain: d, Template: t})
	}
	return out, -1, nil
}
