package winet

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/url"

	"github.com/rgamba/evtwebsocket"

	"github.com/anicoll/winet-integration/internal/pkg/config"
)

type service struct {
	cfg        *config.WinetConfig
	properties map[string]string
	conn       evtwebsocket.Conn
	errChan    chan error
	token      string
}

func New(cfg *config.WinetConfig, errChan chan error) *service {
	return &service{
		cfg:     cfg,
		errChan: errChan,
	}
}

func (s *service) sendIfErr(err error) {
	if err != nil {
		s.errChan <- err
	}
}
func (s *service) onconnect(c *evtwebsocket.Conn) {
	data, err := json.Marshal(ConnectRequest{
		Lang:    "en_us",
		Service: Connect.String(),
		Token:   s.token,
	})
	s.sendIfErr(err)
	err = c.Send(evtwebsocket.Msg{
		Body: data,
	})
	s.sendIfErr(err)
}

func (s *service) onMessage(data []byte, c *evtwebsocket.Conn) {
	result := GenericResult{}
	err := json.Unmarshal(data, &result)
	s.sendIfErr(err)

	switch result.ResultData.Service {
	case Connect:
		res := ConnectResponse{}
		err := json.Unmarshal(data, &res)
		s.sendIfErr(err)
		s.token = res.ResultData.Token

		// login now
		data, err = json.Marshal(LoginRequest{
			Lang:     "en_us",
			Service:  Login.String(),
			Token:    s.token,
			Password: s.cfg.Password,
			Username: s.cfg.Username,
		})
		s.sendIfErr(err)
		err = c.Send(evtwebsocket.Msg{
			Body: data,
		})
		s.sendIfErr(err)
	case DeviceList:
	case Local:
	case Notice:
	case Login:
		fmt.Println("here", data)
		// get some other stats
	case Direct:
	case Real, RealBattery:
	case Statistics:
	default:
		s.reconnect()
		panic(fmt.Sprintf("unexpected winet.WebSocketService: %#v", result.ResultData.Service))
	}

}

func (s *service) onError(err error) {
	if errors.Is(err, io.EOF) {
		s.token = "" // clear token for reconnect
		err = s.reconnect()
	}
	s.sendIfErr(err)
}

func (s *service) reconnect() error {
	u := url.URL{Scheme: "ws", Host: s.cfg.HostPort, Path: "/ws/home/overview"}
	log.Printf("connecting to %s", u.String())
	s.token = "" // clear it out just incase.
	s.conn = evtwebsocket.Conn{
		OnConnected: s.onconnect,
		OnMessage:   s.onMessage,
		OnError:     s.onError,
	}
	return s.conn.Dial(u.String(), "")

}

func (s *service) Connect(ctx context.Context) error {
	if err := s.getProperties(ctx); err != nil {
		return err
	}
	return s.reconnect()

}