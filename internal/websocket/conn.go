package websocket

import (
	"time"

	"github.com/gorilla/websocket"
)

// Conn is an interface that wraps websocket.Conn for testability
type Conn interface {
	ReadMessage() (messageType int, p []byte, err error)
	WriteMessage(messageType int, data []byte) error
	Close() error
	WriteControl(messageType int, data []byte, deadline time.Time) error
}

// conn wraps *websocket.Conn to implement the Conn interface
type conn struct {
	*websocket.Conn
}

// NewConn wraps a *websocket.Conn
func NewConn(c *websocket.Conn) Conn {
	return &conn{Conn: c}
}
