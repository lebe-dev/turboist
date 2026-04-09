package ws

import "encoding/json"

// Client → Server message types
const (
	MsgSubscribe   = "subscribe"
	MsgUnsubscribe = "unsubscribe"
	MsgPong        = "pong"
)

// Server → Client message types
const (
	MsgSnapshot = "snapshot"
	MsgDelta    = "delta"
	MsgPing     = "ping"
	MsgError    = "error"
)

// Channels
const (
	ChannelTasks    = "tasks"
	ChannelPlanning = "planning"
	ChannelTroiki   = "troiki"
)

// IncomingMessage is the envelope for client-to-server messages.
type IncomingMessage struct {
	Type    string `json:"type"`
	Channel string `json:"channel,omitempty"`
	View    string `json:"view,omitempty"`
	Context string `json:"context,omitempty"`
	Seq     int    `json:"seq,omitempty"`
}

// OutgoingMessage is the envelope for server-to-client messages.
type OutgoingMessage struct {
	Type    string `json:"type"`
	Channel string `json:"channel,omitempty"`
	Data    any    `json:"data,omitempty"`
	Message string `json:"message,omitempty"`
	Seq     int    `json:"seq,omitempty"`
}

func marshalMsg(msg OutgoingMessage) ([]byte, error) {
	return json.Marshal(msg)
}
