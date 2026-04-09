package todoist

import (
	"context"
	"testing"

	synctodoist "github.com/CnTeng/todoist-api-go/sync"
	"github.com/google/uuid"
)

func TestSyncHandler_DataSyncAdvancesToken(t *testing.T) {
	h := newSyncHandler()
	ctx := context.Background()

	token, _ := h.SyncToken(ctx)
	if *token != synctodoist.DefaultSyncToken {
		t.Fatalf("initial token = %q, want %q", *token, synctodoist.DefaultSyncToken)
	}

	// Simulate a data-only sync response (no commands → empty SyncStatus).
	resp := &synctodoist.SyncResponse{SyncToken: "token-after-sync"}
	if err := h.HandleResponse(ctx, resp); err != nil {
		t.Fatal(err)
	}

	token, _ = h.SyncToken(ctx)
	if *token != "token-after-sync" {
		t.Fatalf("token after data sync = %q, want %q", *token, "token-after-sync")
	}
}

func TestSyncHandler_CommandResponseDoesNotAdvanceToken(t *testing.T) {
	h := newSyncHandler()
	ctx := context.Background()

	// First, set a known token via a data sync.
	dataResp := &synctodoist.SyncResponse{SyncToken: "pre-mutation-token"}
	if err := h.HandleResponse(ctx, dataResp); err != nil {
		t.Fatal(err)
	}

	// Simulate a command response (has SyncStatus entries).
	cmdResp := &synctodoist.SyncResponse{
		SyncToken:  "post-mutation-token",
		SyncStatus: map[uuid.UUID]error{uuid.New(): nil},
	}
	if err := h.HandleResponse(ctx, cmdResp); err != nil {
		t.Fatal(err)
	}

	token, _ := h.SyncToken(ctx)
	if *token != "pre-mutation-token" {
		t.Fatalf("token after command = %q, want %q (should not advance)", *token, "pre-mutation-token")
	}
}

func TestSyncHandler_SyncTokenReturnsCopy(t *testing.T) {
	h := newSyncHandler()
	ctx := context.Background()

	resp := &synctodoist.SyncResponse{SyncToken: "token-v1"}
	if err := h.HandleResponse(ctx, resp); err != nil {
		t.Fatal(err)
	}

	ptr1, _ := h.SyncToken(ctx)

	// Update the token.
	resp2 := &synctodoist.SyncResponse{SyncToken: "token-v2"}
	if err := h.HandleResponse(ctx, resp2); err != nil {
		t.Fatal(err)
	}

	// The previously returned pointer should still hold the old value.
	if *ptr1 != "token-v1" {
		t.Fatalf("previously returned token changed to %q, want %q", *ptr1, "token-v1")
	}
}
