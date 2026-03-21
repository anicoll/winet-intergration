package winet

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anicoll/winet-integration/internal/pkg/config"
	"github.com/anicoll/winet-integration/internal/pkg/model"
	socketsmocks "github.com/anicoll/winet-integration/mocks/sockets"
	ws "github.com/anicoll/winet-integration/pkg/sockets"
)

// newTestService creates a service with a background context and large errChan buffer.
func newTestService() *service {
	svc := New(&config.WinetConfig{Username: "user", Password: "pass"}, make(chan error, 32))
	svc.ctx = context.Background()
	svc.properties = map[string]string{} // bypass HTTP fetch in getProperties
	return svc
}

// --- Initialisation ---

func TestNew_InitializesChannels(t *testing.T) {
	svc := New(&config.WinetConfig{}, make(chan error, 10))
	assert.NotNil(t, svc.timeoutErrChan, "timeoutErrChan must be non-nil after New()")
	assert.NotNil(t, svc.processed, "processed must be non-nil after New()")
}

func TestSubscribeToTimeout_ReturnsSameChannelEveryCall(t *testing.T) {
	svc := New(&config.WinetConfig{}, make(chan error, 10))
	ch1 := svc.SubscribeToTimeout()
	ch2 := svc.SubscribeToTimeout()
	assert.Equal(t, ch1, ch2, "SubscribeToTimeout must always return the same channel, not recreate it")
}

// --- waiter ---

func TestWaiter_CancelledContext_ReturnsContextCanceled(t *testing.T) {
	svc := newTestService()
	ctx, cancel := context.WithCancel(context.Background())
	svc.ctx = ctx
	cancel() // pre-cancel so waiter returns immediately

	_, err := svc.waiter()

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestWaiter_ShortContextTimeout_ReturnsError(t *testing.T) {
	svc := newTestService()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	svc.ctx = ctx

	_, err := svc.waiter()

	require.Error(t, err, "waiter must not block forever when context expires")
}

func TestWaiter_ValueOnProcessed_ReturnsValue(t *testing.T) {
	svc := newTestService()
	want := "sensor-payload"

	go func() {
		time.Sleep(10 * time.Millisecond)
		svc.processed <- want
	}()

	got, err := svc.waiter()

	require.NoError(t, err)
	assert.Equal(t, want, got)
}

// --- reconnect closes the old connection ---

func TestReconnect_ClosesOldConnection(t *testing.T) {
	svc := newTestService()
	old := socketsmocks.NewConnection(t)
	old.EXPECT().Close().Return(nil)
	svc.conn = old

	svc.cfg.Host = "127.0.0.1" // port 8082 should be closed in CI
	_ = svc.reconnect(context.Background())

	// AssertExpectations (called by t.Cleanup) verifies Close was called
}

// --- protocol message handlers ---

func TestHandleConnectMessage_SendsLoginRequest(t *testing.T) {
	svc := newTestService()

	connectResp, err := json.Marshal(model.ParsedResult[model.ConnectResponse]{
		ResultCode:    1,
		ResultMessage: "success",
		ResultData:    model.ConnectResponse{Token: "tok-abc"},
	})
	require.NoError(t, err)

	conn := socketsmocks.NewConnection(t)
	var capturedMsg ws.Msg
	conn.EXPECT().Send(mock.Anything).RunAndReturn(func(msg ws.Msg) error {
		capturedMsg = msg
		return nil
	})

	svc.handleConnectMessage(connectResp, conn)

	var loginReq model.LoginRequest
	require.NoError(t, json.Unmarshal(capturedMsg.Body, &loginReq))
	assert.Equal(t, "user", loginReq.Username)
	assert.Equal(t, "pass", loginReq.Password)
	assert.Equal(t, "tok-abc", loginReq.Token)
	assert.Equal(t, model.Login.String(), loginReq.Service)
}

func TestHandleConnectMessage_InvalidJSON_SendsToErrChan(t *testing.T) {
	svc := newTestService()
	conn := socketsmocks.NewConnection(t)
	// no Send expected — invalid JSON should route to errChan, not attempt a send

	svc.handleConnectMessage([]byte("not-json"), conn)

	select {
	case err := <-svc.errChan:
		assert.Error(t, err)
	default:
		t.Fatal("expected an error on errChan for invalid JSON input")
	}
}

func TestHandleLoginMessage_SendsDeviceListRequest(t *testing.T) {
	svc := newTestService()

	loginResp, err := json.Marshal(model.ParsedResult[model.LoginResponse]{
		ResultCode:    1,
		ResultMessage: "success",
		ResultData:    model.LoginResponse{Token: "login-tok", Service: model.Login.String()},
	})
	require.NoError(t, err)

	conn := socketsmocks.NewConnection(t)
	var capturedMsg ws.Msg
	conn.EXPECT().Send(mock.Anything).RunAndReturn(func(msg ws.Msg) error {
		capturedMsg = msg
		return nil
	})

	svc.handleLoginMessage(loginResp, conn)

	var dlReq model.DeviceListRequest
	require.NoError(t, json.Unmarshal(capturedMsg.Body, &dlReq))
	assert.Equal(t, model.DeviceList.String(), dlReq.Service)
	assert.Equal(t, "login-tok", dlReq.Token)
}

// TestOnMessage_LoginTimeout_RoutesToTimeoutChan validates that the nil-channel panic (Bug #1
// from the plan) is fixed: timeoutErrChan is initialised in New() so a "login timeout" message
// can be delivered safely without panicking.
func TestOnMessage_LoginTimeout_RoutesToTimeoutChan(t *testing.T) {
	errChan := make(chan error, 32)
	svc := New(&config.WinetConfig{Host: "127.0.0.1"}, errChan)
	svc.ctx = context.Background()
	svc.properties = map[string]string{}

	timeoutMsg := `{"result_code":1,"result_msg":"login timeout","result_Data":{"service":"connect"}}`

	// Call synchronously — previously this would panic on nil timeoutErrChan.
	svc.onMessage([]byte(timeoutMsg), nil)

	select {
	case err := <-svc.SubscribeToTimeout():
		assert.ErrorIs(t, err, ErrTimeout)
	case <-time.After(time.Second):
		t.Fatal("expected ErrTimeout on timeoutErrChan within 1s")
	}
}

// TestOnMessage_UnknownMessage_DoesNotPanic ensures unrecognised messages are
// handled gracefully (no panic, no hang).
func TestOnMessage_UnknownMessage_DoesNotPanic(t *testing.T) {
	svc := newTestService()
	conn := socketsmocks.NewConnection(t)
	// no methods expected on conn for unknown messages
	msg := `{"result_code":1,"result_msg":"normal user limit","result_Data":{"service":"local"}}`

	assert.NotPanics(t, func() {
		svc.onMessage([]byte(msg), conn)
	})
}

// TestHandleConnectMessage_SendError_DoesNotPanic ensures that a connection send
// failure is routed to errChan rather than panicking.
func TestHandleConnectMessage_SendError_DoesNotPanic(t *testing.T) {
	svc := newTestService()

	connectResp, _ := json.Marshal(model.ParsedResult[model.ConnectResponse]{
		ResultCode: 1, ResultMessage: "success",
		ResultData: model.ConnectResponse{Token: "tok"},
	})

	conn := socketsmocks.NewConnection(t)
	conn.EXPECT().Send(mock.Anything).Return(errors.New("network write failed"))

	assert.NotPanics(t, func() {
		svc.handleConnectMessage(connectResp, conn)
	})

	select {
	case err := <-svc.errChan:
		assert.ErrorContains(t, err, "network write failed")
	default:
		t.Fatal("expected send error on errChan")
	}
}
