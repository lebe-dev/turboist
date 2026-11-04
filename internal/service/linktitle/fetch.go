package linktitle

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"syscall"
	"time"

	"golang.org/x/net/html/charset"
)

const (
	// maxPageBytes is how much of a page is read looking for its <head>.
	maxPageBytes = 1 << 20
	// maxRedirects caps the redirect chain of one fetch.
	maxRedirects = 5
	// fetchTimeout bounds one direct fetch end to end.
	fetchTimeout = 12 * time.Second
	userAgent    = "Mozilla/5.0 (compatible; TurboistLinkTitle/1.0; +https://github.com/lebe-dev/turboist)"
)

// Page is what a fetch learned about a link.
type Page struct {
	Title       string
	Description string
	SiteName    string
}

// FetchError is a failed fetch. Transient ones (network, timeout, 429, 5xx) are
// worth retrying later; the others will fail the same way again.
type FetchError struct {
	Transient bool
	Err       error
}

func (e *FetchError) Error() string { return e.Err.Error() }
func (e *FetchError) Unwrap() error { return e.Err }

// IsTransient reports whether err is worth another attempt later.
func IsTransient(err error) bool {
	var fe *FetchError
	if errors.As(err, &fe) {
		return fe.Transient
	}
	return false
}

// PageFetcher learns the title of a link.
type PageFetcher interface {
	Fetch(ctx context.Context, u *url.URL) (Page, error)
}

// ErrBlockedAddress is a connection refused because the name resolved to an
// address that is not on the public internet.
var ErrBlockedAddress = errors.New("linktitle: address is not public")

// nonPublic are ranges IsGlobalUnicast/IsPrivate do not already exclude but
// that are not reachable public internet either.
var nonPublic = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"), // CGNAT / Tailscale
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("fec0::/10"),
}

// IsPublicAddr reports whether a is a routable public internet address.
func IsPublicAddr(a netip.Addr) bool {
	a = a.Unmap()
	if !a.IsValid() || !a.IsGlobalUnicast() || a.IsPrivate() || a.IsLoopback() ||
		a.IsLinkLocalUnicast() || a.IsMulticast() || a.IsUnspecified() {
		return false
	}
	for _, p := range nonPublic {
		if p.Contains(a) {
			return false
		}
	}
	// NAT64 well-known prefix embeds an IPv4 address: judge that one.
	if a.Is6() && netip.MustParsePrefix("64:ff9b::/96").Contains(a) {
		b := a.As16()
		return IsPublicAddr(netip.AddrFrom4([4]byte{b[12], b[13], b[14], b[15]}))
	}
	return true
}

// guardDial runs after DNS resolution with the address actually being dialled,
// so a name that resolves (or re-resolves) into a private range is refused no
// matter what the URL looked like.
func guardDial(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return ErrBlockedAddress
	}
	a, err := netip.ParseAddr(host)
	if err != nil || !IsPublicAddr(a) {
		return ErrBlockedAddress
	}
	return nil
}

// DirectFetcher fetches the page itself and reads its <head>. It never goes
// through a proxy: the address check must see the real destination.
type DirectFetcher struct {
	client *http.Client
}

// NewDirectFetcher builds the hardened client used in production.
func NewDirectFetcher() *DirectFetcher {
	dialer := &net.Dialer{Timeout: 5 * time.Second, Control: guardDial}
	return newDirectFetcher(&http.Transport{
		Proxy:                  nil,
		DialContext:            dialer.DialContext,
		ForceAttemptHTTP2:      true,
		TLSHandshakeTimeout:    5 * time.Second,
		ResponseHeaderTimeout:  8 * time.Second,
		MaxResponseHeaderBytes: 64 << 10,
		MaxIdleConns:           4,
		IdleConnTimeout:        30 * time.Second,
	})
}

func newDirectFetcher(rt http.RoundTripper) *DirectFetcher {
	return &DirectFetcher{client: &http.Client{
		Transport: rt,
		Timeout:   fetchTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return errors.New("too many redirects")
			}
			if err := ValidateURL(req.URL); err != nil {
				return fmt.Errorf("redirect to %s: %w", req.URL.Redacted(), err)
			}
			return nil
		},
	}}
}

// Fetch reads the page's <head> and returns what it says about itself.
func (f *DirectFetcher) Fetch(ctx context.Context, u *url.URL) (Page, error) {
	if err := ValidateURL(u); err != nil {
		return Page{}, &FetchError{Err: err}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return Page{}, &FetchError{Err: err}
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml;q=0.9,*/*;q=0.5")
	if lang, ok := ctx.Value(acceptLanguageKey{}).(string); ok && lang != "" {
		req.Header.Set("Accept-Language", lang)
	}
	resp, err := f.client.Do(req)
	if err != nil {
		// A blocked address or a refused redirect fails the same way next time.
		permanent := errors.Is(err, ErrBlockedAddress) || errors.Is(err, ErrNotPublicURL)
		return Page{}, &FetchError{Transient: !permanent, Err: err}
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return Page{}, &FetchError{Transient: true, Err: fmt.Errorf("status %d", resp.StatusCode)}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return Page{}, &FetchError{Err: fmt.Errorf("status %d", resp.StatusCode)}
	}
	ct := resp.Header.Get("Content-Type")
	if mt, _, err := mime.ParseMediaType(ct); ct != "" && (err != nil || (mt != "text/html" && mt != "application/xhtml+xml")) {
		return Page{}, &FetchError{Err: fmt.Errorf("not an HTML page (%s)", ct)}
	}
	body, err := charset.NewReader(io.LimitReader(resp.Body, maxPageBytes), ct)
	if err != nil {
		return Page{}, &FetchError{Err: fmt.Errorf("decode charset: %w", err)}
	}
	return parseHead(body, u), nil
}

type acceptLanguageKey struct{}

// WithAcceptLanguage asks the site for pages in the user's language.
func WithAcceptLanguage(ctx context.Context, locale string) context.Context {
	if locale == "" {
		return ctx
	}
	return context.WithValue(ctx, acceptLanguageKey{}, locale+",en;q=0.5")
}

// ChainFetcher tries the page itself first and asks the renderer only when that
// produced no title: most sites — single-page apps included — serve og:title in
// their initial HTML for link previews, so the renderer is a rare fallback.
type ChainFetcher struct {
	Direct   PageFetcher
	Renderer PageFetcher // nil = no fallback
}

func (c ChainFetcher) Fetch(ctx context.Context, u *url.URL) (Page, error) {
	page, err := c.Direct.Fetch(ctx, u)
	if err == nil && page.Title != "" {
		return page, nil
	}
	// A blocked address is not rendered elsewhere either: the renderer would
	// just be asked to reach the same private network.
	if c.Renderer != nil && !errors.Is(err, ErrBlockedAddress) && !errors.Is(err, ErrNotPublicURL) {
		rendered, rerr := c.Renderer.Fetch(ctx, u)
		if rerr == nil && rendered.Title != "" {
			if rendered.Description == "" {
				rendered.Description = page.Description
			}
			return rendered, nil
		}
		if err == nil && rerr != nil && IsTransient(rerr) {
			// The page had no title and the renderer is down: try again later.
			return page, rerr
		}
	}
	return page, err
}

// trimSpaceCollapse folds runs of whitespace into single spaces.
func trimSpaceCollapse(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
