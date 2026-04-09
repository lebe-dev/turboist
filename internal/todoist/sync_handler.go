package todoist

import (
	"context"
	gosync "sync"

	synctodoist "github.com/CnTeng/todoist-api-go/sync"
	extclient "github.com/CnTeng/todoist-api-go/todoist"
)

// syncHandler implements the todoist-api-go Handler interface.
//
// Unlike the library's DefaultHandler, it does NOT advance the sync token when
// processing command (mutation) responses. Command responses are identified by
// having non-empty SyncStatus. This ensures the subsequent incremental sync
// uses the pre-mutation token and picks up the mutation's effects from the API.
type syncHandler struct {
	mu        gosync.Mutex
	syncToken string
}

var _ extclient.Handler = (*syncHandler)(nil)

func newSyncHandler() *syncHandler {
	return &syncHandler{syncToken: synctodoist.DefaultSyncToken}
}

func (h *syncHandler) SyncToken(_ context.Context) (*string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	token := h.syncToken
	return &token, nil
}

func (h *syncHandler) ResourceTypes(_ context.Context) (*synctodoist.ResourceTypes, error) {
	return &synctodoist.ResourceTypes{synctodoist.All}, nil
}

func (h *syncHandler) HandleResponse(_ context.Context, resp any) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	r, ok := resp.(*synctodoist.SyncResponse)
	if !ok {
		return nil
	}
	// Only update sync token for pure data syncs, not command executions.
	// Command responses include SyncStatus entries; data-only syncs don't.
	// By keeping the pre-mutation token, the next incremental sync will
	// return the delta that includes the mutation's effects.
	if len(r.SyncStatus) == 0 {
		h.syncToken = r.SyncToken
	}
	return nil
}
