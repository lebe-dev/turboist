package ws

import (
	"github.com/fasthttp/websocket"
	"github.com/gofiber/fiber/v3"
	"github.com/valyala/fasthttp"
)

var upgrader = websocket.FastHTTPUpgrader{
	CheckOrigin: func(_ *fasthttp.RequestCtx) bool {
		return true
	},
}

// HandleWS is the Fiber v3 handler for WebSocket upgrade.
func (h *Hub) HandleWS(c fiber.Ctx) error {
	return upgrader.Upgrade(c.RequestCtx(), func(conn *websocket.Conn) {
		client := newClient(conn, h)
		h.register(client)

		go client.writePump()
		client.readPump()
	})
}

// IsWebSocketUpgrade checks if the request is a WebSocket upgrade.
func IsWebSocketUpgrade(c fiber.Ctx) bool {
	return websocket.FastHTTPIsWebSocketUpgrade(c.RequestCtx())
}
