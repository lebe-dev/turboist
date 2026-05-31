package handlers

import (
	"bufio"
	"fmt"
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/logging"
	"github.com/lebe-dev/turboist/internal/service/events"
)

// sseHeartbeatInterval is short enough to keep idle connections alive behind
// nginx (default proxy_read_timeout is 60s).
const sseHeartbeatInterval = 25 * time.Second

// EventsHandler exposes SSE invalidation endpoints. EventSource cannot send
// Authorization headers, so clients first POST /events/ticket under their
// normal JWT auth to obtain a short-lived ticket, then open
// GET /events?ticket=... .
type EventsHandler struct {
	hub     *events.Hub
	tickets *events.TicketStore
}

// NewEventsHandler constructs an EventsHandler.
func NewEventsHandler(hub *events.Hub, tickets *events.TicketStore) *EventsHandler {
	return &EventsHandler{hub: hub, tickets: tickets}
}

// Register wires the events endpoints onto r (the authenticated /api/v1 group).
// /events/ticket is restricted to JWT sessions: an API token must not be able
// to mint a stream ticket, since the SSE stream itself bypasses per-route scope
// checks and would otherwise let a narrowly-scoped token observe events across
// every resource.
func (h *EventsHandler) Register(r fiber.Router) {
	r.Post("/events/ticket", httpapi.RequireJWTAuth(), h.issueTicket)
}

// RegisterPublic wires the streaming endpoint onto app (no auth middleware).
// Ticket-based auth happens inside the handler.
func (h *EventsHandler) RegisterPublic(app *fiber.App) {
	app.Get("/api/v1/events", h.stream)
}

type ticketRequest struct {
	// Origin is the caller's per-tab client id. When present it is bound to the
	// ticket so the hub can skip echoing this client's own mutations back to
	// its stream (see Hub.Publish). Optional and best-effort — an empty origin
	// simply disables echo suppression for the stream.
	Origin string `json:"origin"`
}

type ticketResponse struct {
	Ticket    string `json:"ticket"`
	ExpiresIn int    `json:"expiresIn"`
}

func (h *EventsHandler) issueTicket(c fiber.Ctx) error {
	const op = "handler.Events.IssueTicket"
	userID := httpapi.GetUserID(c)
	if userID == 0 {
		return httpapi.ErrAuthInvalid("authentication required")
	}
	logEntry(c, op)
	var req ticketRequest
	// Body is optional; ignore bind errors and treat as no origin.
	_ = c.Bind().JSON(&req)
	tok, err := h.tickets.Issue(userID, req.Origin)
	if err != nil {
		return httpapi.ErrInternal("issue events ticket").WithCause(err)
	}
	return c.JSON(ticketResponse{
		Ticket:    tok,
		ExpiresIn: int(events.TicketTTL / time.Second),
	})
}

func (h *EventsHandler) stream(c fiber.Ctx) error {
	const op = "handler.Events.Stream"
	ctx := c.Context()
	log := logging.FromContext(ctx)

	token := c.Query("ticket")
	userID, origin, err := h.tickets.Consume(token)
	if err != nil {
		log.WarnContext(ctx, op+": invalid ticket")
		return httpapi.ErrAuthInvalid("invalid or expired events ticket")
	}

	ch, cancel := h.hub.Subscribe(userID, origin)

	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache, no-transform")
	c.Set("Connection", "keep-alive")
	// Disable response buffering on nginx even when proxy_buffering is on.
	c.Set("X-Accel-Buffering", "no")

	log.InfoContext(ctx, op+": stream open", slog.Int64("user_id", userID))

	return c.SendStreamWriter(func(w *bufio.Writer) {
		defer cancel()
		defer log.InfoContext(ctx, op+": stream closed", slog.Int64("user_id", userID))

		if _, err := fmt.Fprint(w, ": connected\n\n"); err != nil {
			return
		}
		if err := w.Flush(); err != nil {
			return
		}

		ticker := time.NewTicker(sseHeartbeatInterval)
		defer ticker.Stop()

		for {
			select {
			case ev, ok := <-ch:
				if !ok {
					// Hub closed (server shutdown).
					return
				}
				if _, err := fmt.Fprintf(w, "event: %s\ndata: {}\n\n", ev.Scope); err != nil {
					return
				}
				if err := w.Flush(); err != nil {
					return
				}
			case <-ticker.C:
				if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
					return
				}
				if err := w.Flush(); err != nil {
					return
				}
			}
		}
	})
}
