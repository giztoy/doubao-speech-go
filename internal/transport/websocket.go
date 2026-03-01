package transport

import (
	"context"
	"net/http"

	"github.com/gorilla/websocket"
)

// WSConn 抽象 WebSocket 连接，方便单测注入。
type WSConn interface {
	WriteMessage(messageType int, data []byte) error
	ReadMessage() (messageType int, p []byte, err error)
	Close() error
}

// WSDialer 抽象 WebSocket 建连器。
type WSDialer interface {
	DialContext(ctx context.Context, url string, requestHeader http.Header) (WSConn, *http.Response, error)
}

// GorillaDialer 是默认 gorilla/websocket 实现。
type GorillaDialer struct {
	dialer *websocket.Dialer
}

// NewGorillaDialer 创建 GorillaDialer。
func NewGorillaDialer(d *websocket.Dialer) *GorillaDialer {
	if d == nil {
		d = websocket.DefaultDialer
	}
	return &GorillaDialer{dialer: d}
}

// DialContext 建立 WebSocket 连接。
func (d *GorillaDialer) DialContext(ctx context.Context, url string, requestHeader http.Header) (WSConn, *http.Response, error) {
	conn, resp, err := d.dialer.DialContext(ctx, url, requestHeader)
	if err != nil {
		return nil, resp, err
	}
	return conn, resp, nil
}
