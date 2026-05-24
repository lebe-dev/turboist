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
// Note: /events itself authenticates via ticket query parameter, not the
// group-level Authorization header — but it is mounted on the protected group
// so the auth middleware runs first when a Bearer header is present. Browsers
// using EventSource will not send the header, so the handler must accept
// requests where group auth has not produced a user id and authenticate
// solely via the ticket.
func (h *EventsHandler) Register(r fiber.Router) {
	r.Post("/events/ticket", h.issueTicket)
}

// RegisterPublic wires the streaming endpoint onto app (no auth middleware).
// Ticket-based auth happens inside the handler.
func (h *EventsHandler) RegisterPublic(app *fiber.App) {
	app.Get("/api/v1/events", h.stream)
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
	tok, err := h.tickets.Issue(userID)
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
	userID, err := h.tickets.Consume(token)
	if err != nil {
		log.WarnContext(ctx, op+": invalid ticket")
		return httpapi.ErrAuthInvalid("invalid or expired events ticket")
	}

	ch, cancel := h.hub.Subscribe(userID)

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
