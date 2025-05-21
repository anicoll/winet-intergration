package sockets

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"time"

	"github.com/gorilla/websocket"
)

type Connection interface {
	Dial(ctx context.Context, url, subprotocol string) error
	Send(msg Msg) error
	// IsConnected() bool
	io.Closer
}

type Conn struct {
	ws               *websocket.Conn
	sslSkipVerify    bool
	closed           bool
	pingIntervalSecs int
	onError          func(err error)
	onMessage        func([]byte, Connection)
	onConnected      func(Connection)
	pingMsg          []byte
	msgQueue         []Msg
	// matchMsg         func([]byte, []byte) bool
}

func New(opts ...func(*Conn)) Connection {
	c := &Conn{}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Msg is the message structure.
type Msg struct {
	Body     []byte
	Callback func([]byte, Connection)
}

// Closes the connection.
func (c *Conn) Close() error {
	c.close()
	return nil
}

func (c *Conn) close() {
	c.ws.Close()
	c.closed = true
}

func (c *Conn) Send(msg Msg) error {
	if c.closed {
		return errors.New("closed connection")
	}
	if err := c.ws.WriteMessage(websocket.TextMessage, msg.Body); err != nil {
		c.close()
		if c.onError != nil {
			c.onError(err)
		}
		return err
	}

	if msg.Callback != nil {
		c.msgQueue = append(c.msgQueue, msg)
	}

	return nil
}

func (c *Conn) Dial(ctx context.Context, url, subProtocol string) error {
	dialer := &websocket.Dialer{
		HandshakeTimeout: 15 * time.Second,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: c.sslSkipVerify,
		},
	}
	conn, res, err := dialer.DialContext(ctx, url, nil)
	c.ws = conn
	c.closed = false

	if c.onConnected != nil {
		go c.onConnected(c)
	}
	go func() {
		for {
			_, msg, err := c.ws.ReadMessage()
			if err != nil {

				if c.onError != nil {
					c.onError(err)
				}
				return
			}
			c.onMsg(msg)
		}
	}()
	c.setupPing()
	_ = res
	_ = err
	return nil
}

func (c *Conn) onMsg(msg []byte) {
	// Fire OnMessage every time.
	if c.onMessage != nil {
		go c.onMessage(msg, c)
	}
}

func (c *Conn) setupPing() {
	if c.pingIntervalSecs > 0 && len(c.pingMsg) > 0 {
		ticker := time.NewTicker(time.Second * time.Duration(c.pingIntervalSecs))
		go func() {
			defer ticker.Stop()
			for {
				<-ticker.C // wait for tick
				if c.Send(Msg{c.pingMsg, nil}) != nil {
					return
				}
			}
		}()
	}
}
