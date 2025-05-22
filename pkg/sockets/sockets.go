package sockets

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Connection interface {
	Dial(ctx context.Context, url, subprotocol string) error
	Send(msg Msg) error
	IsConnected() bool
	io.Closer
}

type Conn struct {
	ws               *websocket.Conn
	sslSkipVerify    bool
	closed           bool
	mu               sync.Mutex // protects closed and ws
	pingIntervalSecs int
	onError          func(err error)
	onMessage        func([]byte, Connection)
	onConnected      func(Connection)
	pingMsg          []byte
	msgQueue         []Msg
	done             chan struct{} // for coordinating shutdown
}

type Msg struct {
	Body     []byte
	Callback func([]byte, Connection)
}

func New(opts ...func(*Conn)) Connection {
	c := &Conn{
		done: make(chan struct{}),
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

func (c *Conn) IsConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.closed && c.ws != nil
}

func (c *Conn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}

	select {
	case <-c.done:
	default:
		close(c.done)
	}

	c.closed = true

	if c.ws != nil {
		return c.ws.Close()
	}
	return nil
}

func (c *Conn) Send(msg Msg) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return errors.New("closed connection")
	}
	if c.ws == nil {
		return errors.New("connection not established")
	}

	if err := c.ws.WriteMessage(websocket.TextMessage, msg.Body); err != nil {
		c.closeUnsafe()
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
	c.mu.Lock()
	if c.ws != nil {
		c.mu.Unlock()
		return errors.New("connection already established")
	}
	c.mu.Unlock()

	dialer := &websocket.Dialer{
		HandshakeTimeout: 15 * time.Second,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: c.sslSkipVerify,
		},
		Subprotocols: []string{subProtocol},
	}

	conn, res, err := dialer.DialContext(ctx, url, nil)
	if err != nil {
		if c.onError != nil {
			c.onError(err)
		}
		return err
	}
	if res.StatusCode != 101 {
		err := errors.New("unexpected status code: " + res.Status)
		if c.onError != nil {
			c.onError(err)
		}
		return err
	}

	c.mu.Lock()
	c.ws = conn
	c.closed = false
	c.mu.Unlock()

	if c.onConnected != nil {
		go c.onConnected(c)
	}

	go c.readLoop(ctx)
	go c.setupPing(ctx)

	return nil
}

func (c *Conn) readLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.done:
			return
		default:
			_, msg, err := c.ws.ReadMessage()
			if err != nil {
				c.mu.Lock()
				if !c.closed {
					if c.onError != nil {
						c.onError(err)
					}
					c.closeUnsafe()
				}
				c.mu.Unlock()
				return
			}

			if c.onMessage != nil {
				go c.onMessage(msg, c)
			}

			// Process message queue callbacks
			if len(c.msgQueue) > 0 {
				c.mu.Lock()
				for i, qMsg := range c.msgQueue {
					if qMsg.Callback != nil {
						go qMsg.Callback(msg, c)
					}
					// Remove processed message
					c.msgQueue = append(c.msgQueue[:i], c.msgQueue[i+1:]...)
					break
				}
				c.mu.Unlock()
			}
		}
	}
}

func (c *Conn) closeUnsafe() {
	if c.ws != nil {
		c.ws.Close()
	}
	c.closed = true
	select {
	case <-c.done:
	default:
		close(c.done)
	}
}

func (c *Conn) setupPing(ctx context.Context) {
	if c.pingIntervalSecs <= 0 || len(c.pingMsg) == 0 {
		return
	}

	ticker := time.NewTicker(time.Second * time.Duration(c.pingIntervalSecs))
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.done:
			return
		case <-ticker.C:
			if err := c.Send(Msg{Body: c.pingMsg}); err != nil {
				return
			}
		}
	}
}
