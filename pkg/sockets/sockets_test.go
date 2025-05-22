package sockets

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type testServer struct {
	*httptest.Server
	url    string
	conn   *websocket.Conn
	connMu sync.Mutex
}

func newTestServer(t *testing.T) *testServer {
	ts := &testServer{}

	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		require.NoError(t, err)

		ts.connMu.Lock()
		ts.conn = c
		ts.connMu.Unlock()
	}))

	ts.Server = s
	ts.url = "ws" + s.URL[4:]
	return ts
}

func (ts *testServer) sendMessage(msg []byte) error {
	ts.connMu.Lock()
	defer ts.connMu.Unlock()
	return ts.conn.WriteMessage(websocket.TextMessage, msg)
}

func TestNew(t *testing.T) {
	t.Run("default options", func(t *testing.T) {
		conn := New().(*Conn)
		require.NotNil(t, conn.done)
		require.False(t, conn.closed)
		require.Zero(t, conn.pingIntervalSecs)
	})

	t.Run("with all options", func(t *testing.T) {
		var errorCalled, messageCalled, connectedCalled bool
		_ = errorCalled
		_ = messageCalled
		_ = connectedCalled
		conn := New(
			WithPingIntervalSec(5),
			WithPingMsg([]byte("ping")),
			InsecureSkipVerify(),
			OnError(func(err error) { errorCalled = true }),
			OnMessage(func(b []byte, c Connection) { messageCalled = true }),
			OnConnected(func(c Connection) { connectedCalled = true }),
		).(*Conn)

		require.Equal(t, 5, conn.pingIntervalSecs)
		require.Equal(t, []byte("ping"), conn.pingMsg)
		require.True(t, conn.sslSkipVerify)
		require.NotNil(t, conn.onError)
		require.NotNil(t, conn.onMessage)
		require.NotNil(t, conn.onConnected)
	})
}

func TestDial(t *testing.T) {
	t.Run("successful connection", func(t *testing.T) {
		server := newTestServer(t)
		defer server.Close()

		connected := make(chan struct{})
		conn := New(OnConnected(func(c Connection) {
			close(connected)
		}))

		err := conn.Dial(context.Background(), server.url, "")
		require.NoError(t, err)
		require.True(t, conn.IsConnected())

		select {
		case <-connected:
			// Success
		case <-time.After(time.Second):
			t.Fatal("connection callback not called")
		}
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		conn := New()
		err := conn.Dial(ctx, "ws://invalid-url", "")
		require.Error(t, err)
		require.False(t, conn.IsConnected())
	})

	t.Run("cannot dial twice", func(t *testing.T) {
		server := newTestServer(t)
		defer server.Close()

		conn := New()
		err := conn.Dial(context.Background(), server.url, "")
		require.NoError(t, err)

		err = conn.Dial(context.Background(), server.url, "")
		require.Error(t, err)
		require.Contains(t, err.Error(), "already established")
	})
}

func TestSend(t *testing.T) {
	t.Run("send and receive message", func(t *testing.T) {
		server := newTestServer(t)
		defer server.Close()

		received := make(chan []byte)
		conn := New(OnMessage(func(msg []byte, c Connection) {
			received <- msg
		}))

		err := conn.Dial(context.Background(), server.url, "")
		require.NoError(t, err)

		testMsg := []byte("test message")
		err = server.sendMessage(testMsg)
		require.NoError(t, err)

		select {
		case msg := <-received:
			require.Equal(t, testMsg, msg)
		case <-time.After(time.Second):
			t.Fatal("message not received")
		}
	})

	t.Run("message callbacks", func(t *testing.T) {
		server := newTestServer(t)
		defer server.Close()

		callbackCalled := make(chan []byte)
		conn := New()

		err := conn.Dial(context.Background(), server.url, "")
		require.NoError(t, err)

		err = conn.Send(Msg{
			Body: []byte("request"),
			Callback: func(msg []byte, c Connection) {
				callbackCalled <- msg
			},
		})
		require.NoError(t, err)

		err = server.sendMessage([]byte("response"))
		require.NoError(t, err)

		select {
		case msg := <-callbackCalled:
			require.Equal(t, []byte("response"), msg)
		case <-time.After(time.Second):
			t.Fatal("callback not called")
		}
	})
}

func TestPing(t *testing.T) {
	t.Run("sends ping messages", func(t *testing.T) {
		server := newTestServer(t)
		defer server.Close()

		pingReceived := make(chan []byte)
		conn := New(
			WithPingIntervalSec(1),
			WithPingMsg([]byte("ping")),
		)

		err := conn.Dial(context.Background(), server.url, "")
		require.NoError(t, err)

		go func() {
			ts := server
			ts.connMu.Lock()
			defer ts.connMu.Unlock()
			_, msg, err := ts.conn.ReadMessage()
			require.NoError(t, err)
			pingReceived <- msg
		}()

		select {
		case msg := <-pingReceived:
			require.Equal(t, []byte("ping"), msg)
		case <-time.After(2 * time.Second):
			t.Fatal("ping not received")
		}
	})

	t.Run("stops ping on close", func(t *testing.T) {
		server := newTestServer(t)
		defer server.Close()

		conn := New(
			WithPingIntervalSec(1),
			WithPingMsg([]byte("ping")),
		)

		err := conn.Dial(context.Background(), server.url, "")
		require.NoError(t, err)

		time.Sleep(100 * time.Millisecond)
		err = conn.Close()
		require.NoError(t, err)

		// Ensure no more pings are sent
		time.Sleep(2 * time.Second)
	})
}

func TestClose(t *testing.T) {
	t.Run("close stops readLoop", func(t *testing.T) {
		server := newTestServer(t)
		defer server.Close()

		messageReceived := make(chan struct{})
		conn := New(OnMessage(func(msg []byte, c Connection) {
			messageReceived <- struct{}{}
		}))

		err := conn.Dial(context.Background(), server.url, "")
		require.NoError(t, err)

		err = conn.Close()
		require.NoError(t, err)

		err = server.sendMessage([]byte("test"))
		require.NoError(t, err)

		select {
		case <-messageReceived:
			t.Fatal("should not receive messages after close")
		case <-time.After(100 * time.Millisecond):
			// Success
		}
	})

	t.Run("multiple close calls", func(t *testing.T) {
		server := newTestServer(t)
		defer server.Close()

		conn := New()
		err := conn.Dial(context.Background(), server.url, "")
		require.NoError(t, err)

		err = conn.Close()
		require.NoError(t, err)

		err = conn.Close()
		require.NoError(t, err) // Second close should not error
	})
}
