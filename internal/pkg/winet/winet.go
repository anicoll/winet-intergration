package winet

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/anicoll/winet-integration/internal/pkg/config"
	"github.com/anicoll/winet-integration/internal/pkg/model"
	ws "github.com/anicoll/winet-integration/pkg/sockets"
)

var ErrTimeout = errors.New("timeout")

const EnglishLang string = "en_us"

const waiterTimeout = 30 * time.Second

type service struct {
	cfg            *config.WinetConfig
	properties     map[string]string
	conn           ws.Connection
	errChan        chan error
	timeoutErrChan chan error
	token          string
	logger         *zap.Logger
	storedData     []byte
	ctx            context.Context // set in Connect; used by waiter

	deviceMu      sync.RWMutex
	currentDevice *model.Device
	processed     chan any // used to communicate when messages are processed.
}

func New(cfg *config.WinetConfig, errChan chan error) *service {
	return &service{
		cfg:            cfg,
		errChan:        errChan,
		logger:         zap.L(), // returns the global logger.
		storedData:     []byte{},
		processed:      make(chan any),
		timeoutErrChan: make(chan error, 1),
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
	data, err := json.Marshal(model.ConnectRequest{
		Request: model.Request{
			Lang:    EnglishLang,
			Service: model.Connect.String(),
			Token:   s.token,
		},
	})
	if err != nil {
		s.sendIfErr(err)
		return
	}
	s.logger.Debug("sending msg", zap.ByteString("request", data), zap.String("query_stage", model.Connect.String()))
	if err = c.Send(ws.Msg{Body: data}); err != nil {
		s.sendIfErr(err)
		return
	}
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
	if result.ResultMessage == "login timeout" {
		// do we need to control is from here?
		s.logger.Debug("login timeout, reconnecting")
		s.timeoutErrChan <- ErrTimeout
		err := s.reconnect(context.Background())
		s.onError(err)
		return
	}

	if result.ResultMessage == "normal user limit" {
		s.logger.Debug("normal user limit reached.")
		return
	}

	switch result.ResultData.Service {
	case model.Connect:
		s.handleConnectMessage(data, c)
	case model.DeviceList:
		go s.handleDeviceListMessage(data, c)
	case model.Param:
		go s.handleParamMessage(data, c)
	case model.Local:
	case model.Notice:
	case model.Login:
		s.handleLoginMessage(data, c)
	case model.Direct:
		go s.handleDirectMessage(data)
	case model.Real, model.RealBattery:
		go s.handleRealMessage(data)
	}
}

func (s *service) onError(err error) {
	if errors.Is(err, io.EOF) {
		return
	}
	s.sendIfErr(err)
}

func (s *service) reconnect(ctx context.Context) error {
	// Close the old connection before creating a new one. Without this, the old
	// connection's readLoop and setupPing goroutines keep running and continue
	// calling onMessage/onError on this service, racing with the new session.
	if s.conn != nil {
		if err := s.conn.Close(); err != nil {
			s.logger.Warn("error closing previous connection", zap.Error(err))
		}
		s.conn = nil
	}

	var u url.URL
	if s.cfg.Ssl {
		u = url.URL{Scheme: "wss", Host: s.cfg.Host + ":443", Path: "/ws/home/overview"}
	} else {
		u = url.URL{Scheme: "ws", Host: s.cfg.Host + ":8082", Path: "/ws/home/overview"}
	}

	s.logger.Debug("connecting to", zap.String("url", u.String()))

	s.token = "" // clear it out just in case.

	s.conn = ws.New(
		ws.OnConnected(s.onconnect),
		ws.OnMessage(s.onMessage),
		ws.OnError(s.onError),
		ws.InsecureSkipVerify(),
		ws.WithPingIntervalSec(8),
		ws.WithPingMsg([]byte("ping")),
	)

	if err := s.conn.Dial(ctx, u.String(), ""); err != nil {
		s.logger.Error("failed to connect to", zap.String("url", u.String()), zap.Error(err))
		return err
	}
	s.logger.Debug("successfully connected to", zap.String("url", u.String()))
	return nil
}

func (s *service) Connect(ctx context.Context) error {
	s.ctx = ctx
	if err := s.getProperties(ctx); err != nil {
		s.logger.Error("failed to get properties", zap.Error(err))
		return err
	}
	s.logger.Info("received properties")
	return s.reconnect(ctx)
}

func (s *service) SubscribeToTimeout() <-chan error {
	return s.timeoutErrChan
}

// waiter blocks until a response is published to the processed channel, the
// parent context is cancelled, or the waiterTimeout elapses. It returns the
// value and a non-nil error on the latter two cases so callers can bail out
// instead of deadlocking.
func (s *service) waiter() (any, error) {
	select {
	case <-s.ctx.Done():
		return nil, s.ctx.Err()
	case <-time.After(waiterTimeout):
		return nil, errors.New("timed out waiting for device response")
	case v := <-s.processed:
		return v, nil
	}
}
