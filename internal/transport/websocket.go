package transport

import (
	"context"
	"net/http"

	"github.com/gorilla/websocket"
)

// WSConn abstracts a WebSocket connection for testing.
type WSConn interface {
	WriteMessage(messageType int, data []byte) error
	ReadMessage() (messageType int, p []byte, err error)
	Close() error
}

// WSDialer abstracts WebSocket dialing.
type WSDialer interface {
	DialContext(ctx context.Context, url string, requestHeader http.Header) (WSConn, *http.Response, error)
}

// GorillaDialer is the default gorilla/websocket implementation.
type GorillaDialer struct {
	dialer *websocket.Dialer
}

// NewGorillaDialer creates a GorillaDialer.
func NewGorillaDialer(d *websocket.Dialer) *GorillaDialer {
	if d == nil {
		d = websocket.DefaultDialer
	}
	return &GorillaDialer{dialer: d}
}

// DialContext opens a WebSocket connection.
func (d *GorillaDialer) DialContext(ctx context.Context, url string, requestHeader http.Header) (WSConn, *http.Response, error) {
	conn, resp, err := d.dialer.DialContext(ctx, url, requestHeader)
	if err != nil {
		return nil, resp, err
	}
	return conn, resp, nil
}
