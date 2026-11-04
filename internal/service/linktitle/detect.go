// Package linktitle turns a task created with nothing but an https:// link as
// its title into a readable one: a background worker fetches the page, takes
// its title (og:title, <title>, …) and rewrites the task as "Article «…»", with
// the link and the page's summary moved into the description.
//
// Only public domain names over https are ever fetched — never an IP literal,
// never an address that resolves into a private, loopback or link-local range —
// because the server would otherwise be a proxy into its own network for anyone
// who can create a task.
package linktitle

import (
	"errors"
	"net/netip"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	// maxURLLength caps what is still treated as a link rather than text.
	maxURLLength = 2048
	// maxTemplateLength caps a title template, in runes.
	maxTemplateLength = 200
	// maxRules caps the per-domain template list.
	maxRules = 100
	// TitlePlaceholder is replaced by the page title in a template.
	TitlePlaceholder = "{title}"
	// DomainPlaceholder is replaced by the link's host (without "www.").
	DomainPlaceholder = "{domain}"
)

// reservedSuffixes are names that never denote a public site, whatever they
// resolve to on the server's network.
var reservedSuffixes = []string{
	"localhost", "local", "internal", "lan", "home", "home.arpa", "corp",
	"intranet", "private", "test", "invalid", "example", "onion", "localdomain",
}

// ErrNotPublicURL is a link the worker refuses to fetch.
var ErrNotPublicURL = errors.New("linktitle: not a public https URL")

// DetectURL reports whether a task title is a bare link the feature handles:
// it starts with https://, contains nothing else, and points at a public domain
// name. The returned URL is the parsed link.
func DetectURL(title string) (*url.URL, bool) {
	s := strings.TrimSpace(title)
	if len(s) < len("https://x.y") || len(s) > maxURLLength {
		return nil, false
	}
	if !strings.EqualFold(s[:len("https://")], "https://") {
		return nil, false
	}
	if strings.IndexFunc(s, unicode.IsSpace) >= 0 {
		return nil, false
	}
	u, err := url.Parse(s)
	if err != nil {
		return nil, false
	}
	if err := ValidateURL(u); err != nil {
		return nil, false
	}
	return u, true
}

// ValidateURL accepts only https links to public domain names: no credentials,
// no port other than 443, no IP literal, no reserved or single-label name. It is
// applied to the original link and to every redirect hop.
func ValidateURL(u *url.URL) error {
	if u == nil || !strings.EqualFold(u.Scheme, "https") || u.User != nil || u.Opaque != "" {
		return ErrNotPublicURL
	}
	if p := u.Port(); p != "" && p != "443" {
		return ErrNotPublicURL
	}
	if !IsPublicHostname(u.Hostname()) {
		return ErrNotPublicURL
	}
	return nil
}

// IsPublicHostname reports whether host looks like a public DNS name. It does
// not resolve it: the dialer checks every address the name resolves to.
func IsPublicHostname(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if host == "" || len(host) > 253 {
		return false
	}
	if _, err := netip.ParseAddr(host); err == nil {
		return false
	}
	// Non-canonical numeric hosts ("0x7f.1", "2130706433") are IPs to some
	// resolvers; the all-digit / hex TLD check below refuses them.
	labels := strings.Split(host, ".")
	if len(labels) < 2 {
		return false
	}
	for _, l := range labels {
		if !validLabel(l) {
			return false
		}
	}
	tld := labels[len(labels)-1]
	if isAllDigits(tld) || strings.HasPrefix(tld, "0x") {
		return false
	}
	for _, r := range reservedSuffixes {
		if host == r || strings.HasSuffix(host, "."+r) {
			return false
		}
	}
	return true
}

func validLabel(l string) bool {
	if l == "" || utf8.RuneCountInString(l) > 63 || strings.HasPrefix(l, "-") || strings.HasSuffix(l, "-") {
		return false
	}
	for _, r := range l {
		switch {
		case r == '-':
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'z':
		case r > unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r)):
			// Internationalised names arrive in Unicode form when typed.
		default:
			return false
		}
	}
	return true
}

func isAllDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

// displayHost is the host as shown to a person: lower case, without "www.".
func displayHost(u *url.URL) string {
	return strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
}

// FallbackTitle is what stands in for the page title when none could be found:
// host and path, e.g. "example.com/blog/post".
func FallbackTitle(u *url.URL) string {
	p := strings.TrimRight(u.EscapedPath(), "/")
	if unescaped, err := url.PathUnescape(p); err == nil {
		p = unescaped
	}
	return truncateRunes(displayHost(u)+p, maxTitleRunes)
}
