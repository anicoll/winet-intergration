package winet

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"time"

	ws "github.com/anicoll/evtwebsocket"
	"github.com/anicoll/winet-integration/internal/pkg/config"
	"github.com/anicoll/winet-integration/internal/pkg/model"
	"go.uber.org/zap"
)

const EnglishLang string = "en_us"

type publisher interface {
	PublishData(deviceStatusMap map[model.Device][]model.DeviceStatus) error
	RegisterDevice(device *model.Device) error
}

type service struct {
	cfg            *config.WinetConfig
	properties     map[string]string
	conn           ws.Connection
	errChan        chan error
	token          string
	logger         *zap.Logger
	storedData     []byte
	publisher      publisher
	connectionTime time.Time
	currentDevice  *model.Device
	processed      chan any // used to communicate when messages are processed.
}

func New(cfg *config.WinetConfig, publisher publisher, errChan chan error) *service {
	return &service{
		cfg:        cfg,
		errChan:    errChan,
		logger:     zap.L(), // returns the global logger.
		storedData: []byte{},
		publisher:  publisher,
		processed:  make(chan any),
	}
}

func (s *service) sendIfErr(err error) {
	if err != nil {
		s.errChan <- err
	}
}

func (s *service) onconnect(c ws.Connection) {
	s.conn = c
	s.logger.Debug("onconnect ws received")
	data, err := json.Marshal(model.ConnectRequest{
		Request: model.Request{
			Lang:    "en_us",
			Service: model.Connect.String(),
			Token:   s.token,
		},
	})
	s.sendIfErr(err)
	s.logger.Debug("sending msg", zap.ByteString("request", data), zap.String("query_stage", model.Connect.String()))
	s.sendMessage(data)
	s.logger.Debug("msg sent", zap.String("query_stage", model.Connect.String()))
}

// returns bool if is json.SyntaxError
func (s *service) unmarshal(data []byte) (*model.GenericResult, bool) {
	result := model.GenericResult{}

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
	s.conn = c
	if !s.conn.IsConnected() {
		s.sendIfErr(s.reconnect())
		return
	}
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
	switch result.ResultMessage {
	case "success":
		break
	case "normal user limit":
		return
	case "login timeout":
		s.logger.Info("time since last connection", zap.Duration("timeout_duration", time.Since(s.connectionTime)))
		s.reconnect()
		return
	}

	switch result.ResultData.Service {
	case model.Connect:
		s.handleConnectMessage(data)
	case model.DeviceList:
		go s.handleDeviceListMessage(data)
	case model.Param:
		go s.handleParamMessage(data)
	case model.Local:
	case model.Notice:
	case model.Login:
		s.handleLoginMessage(data)
	case model.Direct:
		go s.handleDirectMessage(data)
	case model.Real, model.RealBattery:
		go s.handleRealMessage(data)
	}
}

func (s *service) onError(err error) {
	if errors.Is(err, io.EOF) {
		err = s.reconnect()
	}
	s.sendIfErr(err)
}

func (s *service) reconnect() error {
	_ = s.conn.Close()
	s.conn = nil
	var u url.URL
	if s.cfg.Ssl {
		u = url.URL{Scheme: "wss", Host: s.cfg.Host + ":443", Path: "/ws/home/overview"}
	} else {
		u = url.URL{Scheme: "ws", Host: s.cfg.Host + ":8082", Path: "/ws/home/overview"}
	}

	s.logger.Debug("connecting to", zap.String("url", u.String()))

	s.token = "" // clear it out just incase.

	s.conn = ws.New(
		ws.OnConnected(s.onconnect),
		ws.OnMessage(s.onMessage),
		ws.OnError(s.onError),
		ws.InsecureSkipVerify(),
		ws.WithMaxMessageSize(100000),
		ws.WithPingIntervalSec(4),
		ws.WithPingMsg([]byte("ping")),
	)

	if err := s.conn.Dial(u.String(), ""); err != nil {
		s.logger.Error("failed to connect to", zap.String("url", u.String()), zap.Error(err))
		return err
	}
	s.logger.Debug("successfully connected to", zap.String("url", u.String()))
	s.connectionTime = time.Now()
	return nil
}

func (s *service) Connect(ctx context.Context) error {
	if err := s.getProperties(ctx); err != nil {
		s.logger.Error("failed to get properties", zap.Error(err))
		return err
	}
	return s.reconnect()
}

func (s *service) Reconnect() error {
	return s.reconnect()
}
