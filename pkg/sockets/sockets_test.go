package sockets

import (
	"context"
	"testing"

	"github.com/gorilla/websocket"
)

func TestConn_Dial(t *testing.T) {
	type fields struct {
		ws               *websocket.Conn
		closed           bool
		pingIntervalSecs int
		onError          func(err error)
		onMessage        func([]byte, Connection)
		onConnected      func(Connection)
		pingMsg          []byte
		msgQueue         []Msg
	}
	type args struct {
		url         string
		subProtocol string
	}
	tests := map[string]struct {
		fields  fields
		args    args
		wantErr bool
	}{
		"test": {
			fields: fields{
				ws:               nil,
				closed:           true,
				pingIntervalSecs: 5,
				onError:          nil,
				onMessage:        nil,
				onConnected:      nil,
				pingMsg:          []byte("ping"),
				msgQueue:         []Msg{},
			},
			args: args{
				// url: "ws://192.168.107.8",
				url: "wss://192.168.107.8:443/ws/home/overview",
			},
			wantErr: false,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			c := &Conn{
				ws:               tt.fields.ws,
				closed:           tt.fields.closed,
				pingIntervalSecs: tt.fields.pingIntervalSecs,
				onError:          tt.fields.onError,
				onMessage:        tt.fields.onMessage,
				onConnected:      tt.fields.onConnected,
				pingMsg:          tt.fields.pingMsg,
				msgQueue:         tt.fields.msgQueue,
			}
			if err := c.Dial(context.Background(), tt.args.url, tt.args.subProtocol); (err != nil) != tt.wantErr {
				t.Errorf("Conn.Dial() error = %v, wantErr %v", err, tt.wantErr)
			}
			cccc := make(chan struct{})
			<-cccc
		})
	}
}
