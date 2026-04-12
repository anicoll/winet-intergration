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
	"github.com/anicoll/winet-integration/internal/pkg/publisher"
	ws "github.com/anicoll/winet-integration/pkg/sockets"
)

var ErrTimeout = errors.New("timeout")

const EnglishLang string = "en_us"

const waiterTimeout = 30 * time.Second

// SessionEvent is emitted on the Events() channel for significant session lifecycle changes.
type SessionEvent struct {
	Err error // ErrTimeout on login timeout; other errors for unexpected failures
}

// pendingCmd provides a thread-safe single-slot channel for request/response correlation.
// The inverter protocol is strictly serial (one outstanding request at a time) so a
// single slot is sufficient. deliver is a no-op when nobody is waiting.
type pendingCmd struct {
	mu sync.Mutex
	ch chan any
}

func (p *pendingCmd) wait(ctx context.Context) (any, error) {
	ch := make(chan any, 1)
	p.mu.Lock()
	p.ch = ch
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		p.ch = nil
		p.mu.Unlock()
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(waiterTimeout):
		return nil, errors.New("timed out waiting for device response")
	case v := <-ch:
		return v, nil
	}
}

func (p *pendingCmd) deliver(v any) {
	p.mu.Lock()
	ch := p.ch
	p.mu.Unlock()
	if ch != nil {
		select {
		case ch <- v:
		default:
		}
	}
}

type service struct {
	cfg        *config.WinetConfig
	properties map[string]string
	connMu     sync.RWMutex // protects conn
	conn       ws.Connection
	events     chan SessionEvent
	token      string
	logger     *zap.Logger
	storedData []byte
	ctx        context.Context // set in Connect; used by inverter commands

	deviceMu      sync.RWMutex
	currentDevice *model.Device

	publisher  publisher.DataPublisher
	pending    pendingCmd
	loginReady chan struct{} // closed by handleLoginMessage to start poll loop
	cancelPoll context.CancelFunc

	onDeviceStatuses func(statuses []model.DeviceStatus)
}

// SetDeviceStatusHook registers a callback that is invoked (from handleRealMessage) each time
// a fresh batch of device statuses arrives. The callback must be non-blocking — it should only
// update in-memory state and must not send inverter commands (which require the pending slot).
func (s *service) SetDeviceStatusHook(fn func(statuses []model.DeviceStatus)) {
	s.onDeviceStatuses = fn
}

func New(cfg *config.WinetConfig, pub publisher.DataPublisher) *service {
	return &service{
		cfg:        cfg,
		publisher:  pub,
		logger:     zap.L(),
		storedData: []byte{},
		events:     make(chan SessionEvent, 1),
	}
}

// getConn returns a snapshot of the current connection under a read lock.
// All callers that need to use s.conn should go through this method so that
// concurrent writes from reconnect() cannot produce a nil-pointer dereference.
func (s *service) getConn() ws.Connection {
	s.connMu.RLock()
	c := s.conn
	s.connMu.RUnlock()
	return c
}

// Events returns the channel on which session lifecycle events are delivered.
// Callers should select on this and ctx.Done() to detect login timeouts.
func (s *service) Events() <-chan SessionEvent {
	return s.events
}

// sendIfErr logs err and signals a reconnect via the events channel.
// It is non-blocking and suppressed during shutdown — it can never crash the app.
func (s *service) sendIfErr(err error) {
	if err == nil {
		return
	}
	s.logger.Error("failed due to an error", zap.Error(err))
	if s.ctx == nil || s.ctx.Err() != nil {
		return
	}
	select {
	case s.events <- SessionEvent{Err: err}:
	default: // reconnect already signalled
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
		s.logger.Debug("login timeout, signalling reconnect")
		// Signal the reconnect via the events channel only. startWinetService will
		// call Connect(), which cancels the poll loop before calling reconnect().
		// Calling reconnect() here directly raced with the poll-loop goroutine
		// reading s.conn and was the root cause of the nil-pointer panic on restart.
		select {
		case s.events <- SessionEvent{Err: ErrTimeout}:
		default:
		}
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
		s.handleDeviceListMessage(data, c)
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
	s.logger.Error("failed due to an error", zap.Error(err))
	if s.ctx != nil && s.ctx.Err() != nil {
		return
	}
	select {
	case s.events <- SessionEvent{Err: err}:
	default:
	}
}

func (s *service) reconnect(ctx context.Context) error {
	// Close the old connection before creating a new one. Without this, the old
	// connection's readLoop and setupPing goroutines keep running and continue
	// calling onMessage/onError on this service, racing with the new session.
	// Hold the write lock so getConn() callers on the poll loop goroutine see a
	// consistent nil (not a half-closed connection) during the transition.
	s.connMu.Lock()
	if s.conn != nil {
		if err := s.conn.Close(); err != nil {
			s.logger.Warn("error closing previous connection", zap.Error(err))
		}
		s.conn = nil
	}
	s.connMu.Unlock()

	var u url.URL
	if s.cfg.Ssl {
		u = url.URL{Scheme: "wss", Host: s.cfg.Host + ":443", Path: "/ws/home/overview"}
	} else {
		u = url.URL{Scheme: "ws", Host: s.cfg.Host + ":8082", Path: "/ws/home/overview"}
	}

	s.logger.Debug("connecting to", zap.String("url", u.String()))

	s.token = "" // clear it out just in case.

	newConn := ws.New(
		ws.OnConnected(s.onconnect),
		ws.OnMessage(s.onMessage),
		ws.OnError(s.onError),
		ws.InsecureSkipVerify(),
		ws.WithPingIntervalSec(8),
		ws.WithPingMsg([]byte("ping")),
	)

	// Dial without holding the lock — I/O must not block readers.
	// s.conn remains nil until the dial succeeds, so poll-loop callers that
	// observe nil via getConn() will exit cleanly rather than panic.
	if err := newConn.Dial(ctx, u.String(), ""); err != nil {
		s.logger.Error("failed to connect to", zap.String("url", u.String()), zap.Error(err))
		return err
	}

	s.connMu.Lock()
	s.conn = newConn
	s.connMu.Unlock()

	s.logger.Debug("successfully connected to", zap.String("url", u.String()))
	return nil
}

func (s *service) Connect(ctx context.Context) error {
	s.ctx = ctx

	// Cancel the previous poll loop, if any, before starting a fresh one.
	if s.cancelPoll != nil {
		s.cancelPoll()
		s.cancelPoll = nil
	}

	// Fresh loginReady channel for every Connect cycle so the poll loop blocks
	// until the login handshake completes.
	s.loginReady = make(chan struct{})

	if err := s.getProperties(ctx); err != nil {
		s.logger.Error("failed to get properties", zap.Error(err))
		return err
	}
	s.logger.Info("received properties")

	if err := s.reconnect(ctx); err != nil {
		return err
	}

	pollCtx, cancelPoll := context.WithCancel(ctx)
	s.cancelPoll = cancelPoll
	go s.runPollLoop(pollCtx)

	return nil
}
