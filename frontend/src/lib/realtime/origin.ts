import { browser } from '$app/environment';

// clientOrigin is a stable, per-tab identifier generated once when the module
// loads in the browser. It is sent on mutating requests via the
// `X-Client-Origin` header and handed to the SSE handshake when issuing a
// stream ticket. The backend uses it to skip echoing a client's own mutations
// back to its own stream (see internal/service/events/hub.go), so the tab that
// made a change does not re-fetch and re-render data it already applied from
// the mutation's response.
//
// It lives only in memory: a fresh tab (or reload) gets a new origin, which is
// correct — there is nothing to suppress for a connection that has not yet made
// a mutation. On the server it is empty, which disables suppression (no SSE
// stream exists there anyway).
export const clientOrigin: string = browser ? crypto.randomUUID() : '';
