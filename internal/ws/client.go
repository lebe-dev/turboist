package ws

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/charmbracelet/log"
	"github.com/fasthttp/websocket"
)

const (
	writeChSize   = 64
	pingInterval  = 30 * time.Second
	writeDeadline = 10 * time.Second
	readDeadline  = 60 * time.Second
)

// TasksSubscription holds parameters for the tasks channel.
type TasksSubscription struct {
	View    string
	Context string
	Seq     int
}

// PlanningSubscription holds parameters for the planning channel.
type PlanningSubscription struct {
	Context string
	Seq     int
}

// Client represents a single WebSocket connection.
type Client struct {
	mu sync.Mutex

	conn *websocket.Conn
	hub  *Hub

	tasksSub    *TasksSubscription
	planningSub *PlanningSubscription

	lastTasksSnap    TasksSnapshot
	lastPlanningSnap *PlanningSnapshot

	writeCh chan []byte
	done    chan struct{}
}

func newClient(conn *websocket.Conn, hub *Hub) *Client {
	return &Client{
		conn:    conn,
		hub:     hub,
		writeCh: make(chan []byte, writeChSize),
		done:    make(chan struct{}),
	}
}

// send queues a message for writing. Non-blocking; drops if buffer full.
func (c *Client) send(data []byte) bool {
	select {
	case c.writeCh <- data:
		return true
	default:
		log.Warn("ws: client write buffer full, dropping message")
		return false
	}
}

// sendJSON marshals and sends a message. Returns false if the message was dropped.
func (c *Client) sendJSON(msg OutgoingMessage) bool {
	data, err := marshalMsg(msg)
	if err != nil {
		log.Error("ws: marshal failed", "err", err)
		return false
	}
	return c.send(data)
}

// sendError sends an error message to the client.
func (c *Client) sendError(message string) {
	c.sendJSON(OutgoingMessage{Type: MsgError, Message: message})
}

// readPump reads messages from the WebSocket connection.
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister(c)
		close(c.done)
		_ = c.conn.Close()
	}()

	_ = c.conn.SetReadDeadline(time.Now().Add(readDeadline))

	for {
		_, rawMsg, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Debug("ws: read error", "err", err)
			}
			return
		}

		_ = c.conn.SetReadDeadline(time.Now().Add(readDeadline))

		var msg IncomingMessage
		if err := json.Unmarshal(rawMsg, &msg); err != nil {
			c.sendError("invalid message format")
			continue
		}

		c.handleMessage(msg)
	}
}

// writePump writes messages to the WebSocket connection.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingInterval)
	defer func() {
		ticker.Stop()
		_ = c.conn.Close()
	}()

	for {
		select {
		case data, ok := <-c.writeCh:
			if !ok {
				return
			}
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeDeadline))
			if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
				log.Debug("ws: write error", "err", err)
				return
			}

		case <-ticker.C:
			c.sendJSON(OutgoingMessage{Type: MsgPing})

		case <-c.done:
			return
		}
	}
}

func (c *Client) handleMessage(msg IncomingMessage) {
	switch msg.Type {
	case MsgSubscribe:
		c.handleSubscribe(msg)
	case MsgUnsubscribe:
		c.handleUnsubscribe(msg)
	case MsgPong:
		_ = c.conn.SetReadDeadline(time.Now().Add(readDeadline))
	default:
		c.sendError("unknown message type: " + msg.Type)
	}
}

func (c *Client) handleSubscribe(msg IncomingMessage) {
	c.mu.Lock()
	defer c.mu.Unlock()

	switch msg.Channel {
	case ChannelTasks:
		view := msg.View
		if view == "" {
			view = "all"
		}
		c.tasksSub = &TasksSubscription{View: view, Context: msg.Context, Seq: msg.Seq}
		c.lastTasksSnap = nil
		c.hub.sendTasksSnapshot(c)

	case ChannelPlanning:
		c.planningSub = &PlanningSubscription{Context: msg.Context, Seq: msg.Seq}
		c.lastPlanningSnap = nil
		c.hub.sendPlanningSnapshot(c)

	default:
		c.sendError("unknown channel: " + msg.Channel)
	}
}

func (c *Client) handleUnsubscribe(msg IncomingMessage) {
	c.mu.Lock()
	defer c.mu.Unlock()

	switch msg.Channel {
	case ChannelTasks:
		c.tasksSub = nil
		c.lastTasksSnap = nil
	case ChannelPlanning:
		c.planningSub = nil
		c.lastPlanningSnap = nil
	}
}
