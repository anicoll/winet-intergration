package winet

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"

	"github.com/anicoll/winet-integration/internal/pkg/config"
	"github.com/anicoll/winet-integration/internal/pkg/model"
	ws "github.com/anicoll/winet-integration/pkg/sockets"
	"go.uber.org/zap"
)

var ErrTimeout = errors.New("timeout")
var ErrConnect = errors.New("winet connection failed") // New exported error

const EnglishLang string = "en_us"
const maxStoredDataSize = 1 * 1024 * 1024 // 1MB

type service struct {
	cfg            *config.WinetConfig
	properties     map[string]string
	conn           ws.Connection
	// errChan        chan error // Removed: No longer passed in or used to send general errors
	timeoutErrChan chan error
	token          string
	logger         *zap.Logger
	storedData     []byte

	currentDevice *model.Device
	// processed     chan any // REMOVED: This channel was fundamentally flawed in its usage.
}

func New(cfg *config.WinetConfig) *service { // Removed errChan from parameters
	return &service{
		cfg:        cfg,
		// errChan:    errChan, // Removed
		logger:     zap.L(), // returns the global logger.
		storedData: []byte{},
		// processed:  make(chan any), // REMOVED
	}
}

// sendIfErr now logs the error and does not send it to a channel.
// Critical errors should be returned by public methods and handled by the caller.
func (s *service) logIfErr(err error) {
	if err != nil {
		s.logger.Error("winet service error", zap.Error(err))
		// Previously: s.errChan <- err
	}
}

func (s. *service) onconnect(c ws.Connection) {
	s.logger.Debug("onconnect ws received")
	data, err := json.Marshal(model.ConnectRequest{
		Request: model.Request{
			Lang:    EnglishLang,
			Service: model.Connect.String(),
			Token:   s.token,
		},
	})
	s.logIfErr(err) // Changed from sendIfErr
	s.logger.Debug("sending msg", zap.ByteString("request", data), zap.String("query_stage", model.Connect.String()))
	err = c.Send(ws.Msg{
		Body: data,
	})
	s.logIfErr(err) // Changed from sendIfErr
	s.logger.Debug("msg sent", zap.String("query_stage", model.Connect.String()))
}

// returns bool if is json.SyntaxError
func (s *service) unmarshal(data []byte) (*model.GenericResult, bool) {
	result := model.GenericResult{}

	if err := json.Unmarshal(data, &result); err != nil {
		var serr *json.SyntaxError
		if errors.As(err, &serr) {
			// Log syntax error but don't send to central error channel, as it's handled locally by storing data
			s.logger.Debug("json syntax error during unmarshal", zap.Error(err))
			return nil, true
		}
		s.logIfErr(err) // Changed from sendIfErr for non-syntax errors
	}

	return &result, false
}

// onMessage is called when a message is received from the WebSocket.
// It handles potential JSON syntax errors by storing partial data and attempting to re-unmarshal.
func (s *service) onMessage(data []byte, c ws.Connection) {
	// Attempt to unmarshal the incoming data
	result, isSyntaxErr := s.unmarshal(data)

	if isSyntaxErr {
		// Data is partial or has a syntax error; buffer it.
		s.logger.Debug("Received partial/syntax error message, appending to internal buffer.",
			zap.Int("incoming_size", len(data)),
			zap.Int("current_buffer_size", len(s.storedData)))

		// Check if appending new data would exceed the maximum size for s.storedData.
		if len(s.storedData)+len(data) > maxStoredDataSize {
			s.logger.Warn("s.storedData is about to exceed max size. Clearing buffer before appending. Some message data may be lost.",
				zap.Int("current_stored_size", len(s.storedData)),
				zap.Int("incoming_data_size", len(data)),
				zap.Int("max_size", maxStoredDataSize))
			s.storedData = []byte{} // Clear the buffer.
		}
		s.storedData = append(s.storedData, data...) // Append new partial data.

		// Try to unmarshal the newly combined buffer.
		combinedResult, combinedIsSyntaxErr := s.unmarshal(s.storedData)
		if combinedIsSyntaxErr {
			// Still a syntax error even after combining. More data likely needed.
			// Additional check: if s.storedData itself is now over the limit (e.g. a single fragment was too big and caused append to exceed)
			if len(s.storedData) > maxStoredDataSize {
				s.logger.Warn("s.storedData (after appending a fragment) exceeds max size. Clearing buffer. This fragment might be too large or part of a problematic sequence.",
					zap.Int("stored_data_size", len(s.storedData)),
					zap.Int("max_size", maxStoredDataSize))
				s.storedData = []byte{} // Clear due to oversized fragment or problematic sequence.
			}
			s.logger.Debug("Combined data in s.storedData still results in syntax error. Awaiting more data.", zap.Int("current_buffer_size", len(s.storedData)))
			return // Wait for more data (or for buffer to be cleared if consistently problematic).
		}

		// If the combined data unmarshalled successfully (not a syntax error):
		if combinedResult != nil {
			s.logger.Debug("Successfully unmarshalled combined data from s.storedData.")
			result = combinedResult    // This is the result to process.
			data = s.storedData        // The 'data' for logging/downstream is the entire processed buffer.
			s.storedData = []byte{}    // Clear the buffer as it's now fully processed.
		} else {
			// Unmarshal of combined data was successful (not a syntax error) but yielded a nil result.
			// This could mean a validation error or other issue within the unmarshal logic for a complete message.
			// s.unmarshal would have logged the specific error via logIfErr.
			s.logger.Debug("Unmarshalling s.storedData was successful but resulted in nil. Clearing buffer as it's considered processed or unrecoverable.", zap.Int("stored_data_size", len(s.storedData)))
			s.storedData = []byte{} // Clear buffer.
			return                  // Cannot proceed with this message.
		}
	} else if result == nil {
		// Initial unmarshal of `data` was not a syntax error, but failed (e.g., validation error logged by `unmarshal`, or other nil result).
		// No buffering logic applies here as the message wasn't partial.
		s.logger.Debug("Initial unmarshal of data failed (non-syntax error or nil result). Not buffering.", zap.ByteString("original_data", data))
		return // Cannot proceed with this message.
	}
	// If we reach here, 'result' is non-nil and contains the successfully parsed message.
	// This could be from the initial 'data' directly, or from processing 's.storedData'.
	// If 's.storedData' was used, it has been cleared.
	// 'data' has been updated to reflect the actual byte slice that was successfully parsed.

	// Proceed with processing the 'result'.
	s.logger.Debug("received message", zap.String("result", result.ResultMessage), zap.String("query_stage", result.ResultData.Service.String()), zap.ByteString("processed_data", data))
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
		s.logger.Debug("onError: EOF received, likely clean disconnect")
		return
	}
	s.logIfErr(err) // Changed from sendIfErr
}

func (s *service) reconnect(ctx context.Context) error {
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
		ws.WithPingIntervalSec(8),
		ws.WithPingMsg([]byte("ping")),
	)

	if err := s.conn.Dial(ctx, u.String(), ""); err != nil {
		s.logger.Error("failed to connect to", zap.String("url", u.String()), zap.Error(err))
		return errors.Join(ErrConnect, err) // Wrap and return ErrConnect
	}
	s.logger.Debug("successfully connected to", zap.String("url", u.String()))
	return nil
}

func (s *service) Connect(ctx context.Context) error {
	if err := s.getProperties(ctx); err != nil {
		s.logger.Error("failed to get properties", zap.Error(err))
		return errors.Join(ErrConnect, err) // Wrap and return ErrConnect
	}
	err := s.reconnect(ctx)
	// s.reconnect already wraps with ErrConnect if dial fails.
	// No need to re-wrap here if s.reconnect returns an error that's already ErrConnect.
	// However, if s.reconnect could return other errors not wrapped with ErrConnect,
	// and those should also be treated as connection failures, then wrapping here is appropriate.
	// For now, assuming s.reconnect only fails on dial or returns nil.
	if err != nil {
		// If s.reconnect fails, it already returns a wrapped ErrConnect
		return err
	}
	return nil
}

func (s *service) SubscribeToTimeout() chan error {
	s.timeoutErrChan = make(chan error, 1)
	return s.timeoutErrChan
}
