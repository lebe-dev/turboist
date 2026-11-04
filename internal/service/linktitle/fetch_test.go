package linktitle

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// testFetcher points every https request at srv, whatever the host, so the
// fetch logic can be exercised without DNS or a public address.
func testFetcher(t *testing.T, srv *httptest.Server) *DirectFetcher {
	t.Helper()
	tr := srv.Client().Transport.(*http.Transport).Clone()
	addr := srv.Listener.Addr().String()
	tr.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}
	tr.TLSClientConfig.InsecureSkipVerify = true //nolint:gosec // test server certificate
	return newDirectFetcher(tr)
}

func TestDirectFetcherReadsHead(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept-Language") == "" {
			t.Error("Accept-Language not sent")
		}
		w.Header().Set("Content-Type", "text/html; charset=windows-1251")
		// "Привет" in windows-1251.
		_, _ = w.Write([]byte("<html><head><title>\xcf\xf0\xe8\xe2\xe5\xf2</title></head></html>"))
	}))
	defer srv.Close()
	p, err := testFetcher(t, srv).Fetch(WithAcceptLanguage(context.Background(), "ru"), mustURL(t, "https://example.com/a"))
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if p.Title != "Привет" {
		t.Fatalf("title = %q", p.Title)
	}
}

func TestDirectFetcherClassifiesErrors(t *testing.T) {
	status := http.StatusServiceUnavailable
	contentType := "text/html"
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(status)
	}))
	defer srv.Close()
	f := testFetcher(t, srv)
	u := mustURL(t, "https://example.com/")

	if _, err := f.Fetch(context.Background(), u); !IsTransient(err) {
		t.Errorf("503 must be transient, got %v", err)
	}
	status = http.StatusNotFound
	if _, err := f.Fetch(context.Background(), u); err == nil || IsTransient(err) {
		t.Errorf("404 must be permanent, got %v", err)
	}
	status, contentType = http.StatusOK, "application/pdf"
	if _, err := f.Fetch(context.Background(), u); err == nil || IsTransient(err) {
		t.Errorf("PDF must be refused permanently, got %v", err)
	}
}

func TestDirectFetcherRefusesUnsafeRedirects(t *testing.T) {
	for _, target := range []string{"http://example.com/", "https://10.0.0.1/", "https://metadata.internal/"} {
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, target, http.StatusFound)
		}))
		_, err := testFetcher(t, srv).Fetch(context.Background(), mustURL(t, "https://example.com/"))
		srv.Close()
		if err == nil || IsTransient(err) || !errors.Is(err, ErrNotPublicURL) {
			t.Errorf("redirect to %s: got %v", target, err)
		}
	}
}

func TestProductionFetcherRefusesPrivateResolution(t *testing.T) {
	// The name passes the syntactic checks, but the dial-time guard sees the
	// loopback address the test server listens on.
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("request must never reach a loopback server")
	}))
	defer srv.Close()
	f := NewDirectFetcher()
	tr := f.client.Transport.(*http.Transport)
	dial := tr.DialContext
	addr := srv.Listener.Addr().String()
	tr.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return dial(ctx, network, addr) // what DNS rebinding to 127.0.0.1 would look like
	}
	_, err := f.Fetch(context.Background(), mustURL(t, "https://example.com/"))
	if !errors.Is(err, ErrBlockedAddress) || IsTransient(err) {
		t.Fatalf("got %v", err)
	}
}

type stubFetcher struct {
	page  Page
	err   error
	calls int
}

func (s *stubFetcher) Fetch(context.Context, *url.URL) (Page, error) {
	s.calls++
	return s.page, s.err
}

func TestChainFetcher(t *testing.T) {
	u := mustURL(t, "https://spa.example.com/")
	direct := &stubFetcher{page: Page{Description: "from head"}}
	renderer := &stubFetcher{page: Page{Title: "Rendered"}}
	p, err := ChainFetcher{Direct: direct, Renderer: renderer}.Fetch(context.Background(), u)
	if err != nil || p.Title != "Rendered" || p.Description != "from head" {
		t.Fatalf("got %+v, %v", p, err)
	}

	direct = &stubFetcher{page: Page{Title: "Direct"}}
	renderer = &stubFetcher{}
	if p, _ := (ChainFetcher{Direct: direct, Renderer: renderer}).Fetch(context.Background(), u); p.Title != "Direct" || renderer.calls != 0 {
		t.Fatalf("renderer must not be asked when the page had a title: %+v calls=%d", p, renderer.calls)
	}

	direct = &stubFetcher{err: &FetchError{Err: ErrBlockedAddress}}
	renderer = &stubFetcher{page: Page{Title: "leak"}}
	if _, err := (ChainFetcher{Direct: direct, Renderer: renderer}).Fetch(context.Background(), u); err == nil || renderer.calls != 0 {
		t.Fatalf("a blocked address must not be handed to the renderer: err=%v calls=%d", err, renderer.calls)
	}
}

func TestRendererRequestURL(t *testing.T) {
	u := mustURL(t, "https://example.com/a?b=1")
	if got := NewRenderer("https://markdown.new/", 0).requestURL(u); got != "https://markdown.new/https://example.com/a?b=1" {
		t.Errorf("prefix: %q", got)
	}
	if got := NewRenderer("https://render.lan/md?u={url}", 0).requestURL(u); got != "https://render.lan/md?u=https://example.com/a?b=1" {
		t.Errorf("placeholder: %q", got)
	}
	if NewRenderer("  ", 0) != nil {
		t.Error("empty template must disable the renderer")
	}
}
