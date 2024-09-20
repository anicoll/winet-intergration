package winet

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"

	ws "github.com/anicoll/evtwebsocket"
	"github.com/anicoll/winet-integration/internal/pkg/config"
	"go.uber.org/zap"
)

const EnglishLang string = "en_us"

type service struct {
	cfg        *config.WinetConfig
	properties map[string]string
	conn       ws.Connection
	errChan    chan error
	token      string
	logger     *zap.Logger
	storedData []byte
}

func New(cfg *config.WinetConfig, errChan chan error) *service {
	return &service{
		cfg:        cfg,
		errChan:    errChan,
		logger:     zap.L(), // returns the global logger.
		storedData: []byte{},
	}
}

func (s *service) sendIfErr(err error) {
	if err != nil {
		s.logger.Error("failed due to an error", zap.Error(err))
		s.errChan <- err
	}
}
func (s *service) onconnect(c ws.Connection) {
	s.logger.Debug("onconnect ws received")
	data, err := json.Marshal(ConnectRequest{
		Request: Request{
			Lang:    "en_us",
			Service: Connect.String(),
			Token:   s.token,
		},
	})
	s.sendIfErr(err)
	s.logger.Debug("sending msg", zap.ByteString("request", data), zap.String("query_stage", Connect.String()))
	err = c.Send(ws.Msg{
		Body: data,
	})
	s.sendIfErr(err)
	s.logger.Debug("msg sent", zap.String("query_stage", Connect.String()))
}

// returns bool if is json.SyntaxError
func (s *service) unmarshal(data []byte) (*GenericResult, bool) {
	result := GenericResult{}

	if err := json.Unmarshal(data, &result); err != nil {
		var serr *json.SyntaxError
		if errors.As(err, &serr) {
			return nil, true
		}
		s.sendIfErr(err)
	}

	return &result, false
}

func (s *service) onMessage(data []byte, c ws.Connection) {
	result, isSyntaxErr := s.unmarshal(data)
	if isSyntaxErr {
		s.storedData = append(s.storedData, data...)
	}
	if result == nil {
		if result, isSyntaxErr = s.unmarshal(s.storedData); isSyntaxErr {
			return
		}
		data = s.storedData
		s.storedData = []byte{}
	}

	s.logger.Debug("received message", zap.String("result", result.ResultMessage), zap.String("query_stage", result.ResultData.Service.String()))
	if result.ResultMessage != "success" {
		s.reconnect()
	}

	switch result.ResultData.Service {
	case Connect:
		s.handleConnectMessage(data, c)
	case DeviceList:
		s.handleDeviceListMessage(data, c)
	case Local:
	case Notice:
	case Login:
		s.handleLoginMessage(data, c)
	case Direct:
		fmt.Println("HERE", string(data))
	case Real, RealBattery:
		fmt.Println("HERE", string(data))
	case Statistics:
	default:
		s.reconnect()
	}
}

func (s *service) onError(err error) {
	if errors.Is(err, io.EOF) {
		err = s.reconnect()
	}
	s.sendIfErr(err)
}

func (s *service) reconnect() error {
	u := url.URL{Scheme: "ws", Host: s.cfg.HostPort, Path: "/ws/home/overview"}
	s.logger.Debug("connecting to", zap.String("url", u.String()))

	s.token = "" // clear it out just incase.

	s.conn = ws.New(
		ws.OnConnected(s.onconnect),
		ws.OnMessage(s.onMessage),
		ws.OnError(s.onError),
		ws.WithMaxMessageSize(100000),
		ws.WithPingIntervalSec(4),
		ws.WithPingMsg([]byte("ping")),
	)

	if err := s.conn.Dial(u.String(), ""); err != nil {
		s.logger.Error("failed to connect to", zap.String("url", u.String()), zap.Error(err))
		return err
	}
	s.logger.Debug("successfully connected to", zap.String("url", u.String()))
	return nil
}

func (s *service) Connect(ctx context.Context) error {
	if err := s.getProperties(ctx); err != nil {
		s.logger.Error("failed to get properties", zap.Error(err))
		return err
	}
	return s.reconnect()

}
