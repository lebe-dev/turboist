package ws

import (
	"encoding/json"
	"hash/fnv"
	"sync"

	"github.com/charmbracelet/log"
	"github.com/lebe-dev/turboist/internal/config"
	"github.com/lebe-dev/turboist/internal/taskview"
	"github.com/lebe-dev/turboist/internal/todoist"
	"github.com/lebe-dev/turboist/internal/troiki"
)

// Hub manages all WebSocket clients and broadcasts cache updates.
type Hub struct {
	mu            sync.RWMutex
	clients       map[*Client]struct{}
	cache         *todoist.Cache
	cfg           *config.AppConfig
	troikiService *troiki.Service
}

// NewHub creates a new Hub.
func NewHub(cache *todoist.Cache, cfg *config.AppConfig) *Hub {
	return &Hub{
		clients: make(map[*Client]struct{}),
		cache:   cache,
		cfg:     cfg,
	}
}

// SetTroikiService sets the troiki service for the hub (nil when disabled).
func (h *Hub) SetTroikiService(svc *troiki.Service) {
	h.troikiService = svc
}

func (h *Hub) register(c *Client) {
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
	log.Debug("ws: client connected", "total", len(h.clients))
}

func (h *Hub) unregister(c *Client) {
	h.mu.Lock()
	delete(h.clients, c)
	h.mu.Unlock()
	log.Debug("ws: client disconnected", "total", len(h.clients))
}

// Broadcast is called on every cache refresh. It computes deltas for each subscribed client.
func (h *Hub) Broadcast() {
	h.mu.RLock()
	clients := make([]*Client, 0, len(h.clients))
	for c := range h.clients {
		clients = append(clients, c)
	}
	h.mu.RUnlock()

	if len(clients) == 0 {
		return
	}

	for _, c := range clients {
		h.broadcastToClient(c)
	}
}

func (h *Hub) broadcastToClient(c *Client) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.tasksSub != nil {
		h.broadcastTasks(c)
	}
	if c.planningSub != nil {
		h.broadcastPlanning(c)
	}
	if c.troikiSub != nil && h.troikiService != nil {
		h.broadcastTroiki(c)
	}
}

func (h *Hub) broadcastTasks(c *Client) {
	result := taskview.ComputeTasks(h.cache, h.cfg, taskview.ViewParams{
		View:    c.tasksSub.View,
		Context: c.tasksSub.Context,
	})
	result.Meta.LastSyncedAt = h.cache.LastSyncedAt().Format("2006-01-02T15:04:05Z")

	if c.lastTasksSnap == nil {
		// First time or re-subscribe: send full snapshot
		c.lastTasksSnap = buildSnapshot(result.Tasks)
		ok := c.sendJSON(OutgoingMessage{
			Type:    MsgSnapshot,
			Channel: ChannelTasks,
			Seq:     c.tasksSub.Seq,
			Data: map[string]any{
				"tasks": result.Tasks,
				"meta":  result.Meta,
			},
		})
		if !ok {
			c.lastTasksSnap = nil // rollback so next broadcast sends full snapshot
		}
		return
	}

	delta, newSnap := computeTasksDelta(c.lastTasksSnap, result.Tasks, result.Meta)
	c.lastTasksSnap = newSnap
	if delta == nil {
		return
	}

	c.sendJSON(OutgoingMessage{
		Type:    MsgDelta,
		Channel: ChannelTasks,
		Seq:     c.tasksSub.Seq,
		Data:    delta,
	})
}

func (h *Hub) broadcastPlanning(c *Client) {
	result := taskview.ComputePlanning(h.cache, h.cfg, c.planningSub.Context)
	result.Meta.LastSyncedAt = h.cache.LastSyncedAt().Format("2006-01-02T15:04:05Z")

	if c.lastPlanningSnap == nil {
		c.lastPlanningSnap = &PlanningSnapshot{
			Backlog: buildSnapshot(result.Backlog),
			Weekly:  buildSnapshot(result.Weekly),
		}
		ok := c.sendJSON(OutgoingMessage{
			Type:    MsgSnapshot,
			Channel: ChannelPlanning,
			Seq:     c.planningSub.Seq,
			Data: map[string]any{
				"backlog": result.Backlog,
				"weekly":  result.Weekly,
				"meta":    result.Meta,
			},
		})
		if !ok {
			c.lastPlanningSnap = nil // rollback so next broadcast sends full snapshot
		}
		return
	}

	backlogDelta, newBacklogSnap := computeTasksDelta(c.lastPlanningSnap.Backlog, result.Backlog, nil)
	weeklyDelta, newWeeklySnap := computeTasksDelta(c.lastPlanningSnap.Weekly, result.Weekly, nil)
	c.lastPlanningSnap.Backlog = newBacklogSnap
	c.lastPlanningSnap.Weekly = newWeeklySnap

	if backlogDelta == nil && weeklyDelta == nil {
		return
	}

	pd := PlanningDelta{Meta: result.Meta}
	if backlogDelta != nil {
		pd.BacklogUpserted = backlogDelta.Upserted
		pd.BacklogRemoved = backlogDelta.Removed
	}
	if weeklyDelta != nil {
		pd.WeeklyUpserted = weeklyDelta.Upserted
		pd.WeeklyRemoved = weeklyDelta.Removed
	}

	c.sendJSON(OutgoingMessage{
		Type:    MsgDelta,
		Channel: ChannelPlanning,
		Seq:     c.planningSub.Seq,
		Data:    pd,
	})
}

// sendTasksSnapshot computes and sends a full tasks snapshot to a client.
// Caller must hold c.mu.
func (h *Hub) sendTasksSnapshot(c *Client) {
	if c.tasksSub == nil {
		return
	}

	result := taskview.ComputeTasks(h.cache, h.cfg, taskview.ViewParams{
		View:    c.tasksSub.View,
		Context: c.tasksSub.Context,
	})
	result.Meta.LastSyncedAt = h.cache.LastSyncedAt().Format("2006-01-02T15:04:05Z")

	c.lastTasksSnap = buildSnapshot(result.Tasks)
	ok := c.sendJSON(OutgoingMessage{
		Type:    MsgSnapshot,
		Channel: ChannelTasks,
		Seq:     c.tasksSub.Seq,
		Data: map[string]any{
			"tasks": result.Tasks,
			"meta":  result.Meta,
		},
	})
	if !ok {
		c.lastTasksSnap = nil
	}
}

// sendPlanningSnapshot computes and sends a full planning snapshot to a client.
// Caller must hold c.mu.
func (h *Hub) sendPlanningSnapshot(c *Client) {
	if c.planningSub == nil {
		return
	}

	result := taskview.ComputePlanning(h.cache, h.cfg, c.planningSub.Context)
	result.Meta.LastSyncedAt = h.cache.LastSyncedAt().Format("2006-01-02T15:04:05Z")

	c.lastPlanningSnap = &PlanningSnapshot{
		Backlog: buildSnapshot(result.Backlog),
		Weekly:  buildSnapshot(result.Weekly),
	}
	ok := c.sendJSON(OutgoingMessage{
		Type:    MsgSnapshot,
		Channel: ChannelPlanning,
		Seq:     c.planningSub.Seq,
		Data: map[string]any{
			"backlog": result.Backlog,
			"weekly":  result.Weekly,
			"meta":    result.Meta,
		},
	})
	if !ok {
		c.lastPlanningSnap = nil
	}
}

func (h *Hub) broadcastTroiki(c *Client) {
	state, err := h.troikiService.ComputeState()
	if err != nil {
		log.Error("ws: troiki compute state failed", "err", err)
		return
	}

	newSnap := hashTroikiState(state)

	if c.lastTroikiSnap == nil {
		snap := newSnap
		c.lastTroikiSnap = &snap
		ok := c.sendJSON(OutgoingMessage{
			Type:    MsgSnapshot,
			Channel: ChannelTroiki,
			Seq:     c.troikiSub.Seq,
			Data:    state,
		})
		if !ok {
			c.lastTroikiSnap = nil
		}
		return
	}

	// Find changed sections
	var changed []troiki.SectionState
	for i := 0; i < 3 && i < len(state.Sections); i++ {
		if newSnap[i] != c.lastTroikiSnap[i] {
			changed = append(changed, state.Sections[i])
		}
	}

	if len(changed) == 0 {
		return
	}

	snap := newSnap
	c.lastTroikiSnap = &snap
	c.sendJSON(OutgoingMessage{
		Type:    MsgDelta,
		Channel: ChannelTroiki,
		Seq:     c.troikiSub.Seq,
		Data: TroikiDelta{
			Sections: toAnySlice(changed),
		},
	})
}

// sendTroikiSnapshot computes and sends a full troiki snapshot to a client.
// Caller must hold c.mu.
func (h *Hub) sendTroikiSnapshot(c *Client) {
	if c.troikiSub == nil || h.troikiService == nil {
		return
	}

	state, err := h.troikiService.ComputeState()
	if err != nil {
		log.Error("ws: troiki compute state failed", "err", err)
		return
	}

	snap := hashTroikiState(state)
	c.lastTroikiSnap = &snap
	ok := c.sendJSON(OutgoingMessage{
		Type:    MsgSnapshot,
		Channel: ChannelTroiki,
		Seq:     c.troikiSub.Seq,
		Data:    state,
	})
	if !ok {
		c.lastTroikiSnap = nil
	}
}

func hashTroikiState(state troiki.State) TroikiSnapshot {
	var snap TroikiSnapshot
	for i, sec := range state.Sections {
		if i >= 3 {
			break
		}
		data, err := json.Marshal(sec)
		if err != nil {
			continue
		}
		h := fnv.New64a()
		h.Write(data)
		snap[i] = h.Sum64()
	}
	return snap
}

func toAnySlice(sections []troiki.SectionState) []any {
	result := make([]any, len(sections))
	for i, s := range sections {
		result[i] = s
	}
	return result
}
