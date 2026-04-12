package winet

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anicoll/winet-integration/internal/pkg/config"
	"github.com/anicoll/winet-integration/internal/pkg/model"
	publishermocks "github.com/anicoll/winet-integration/mocks/publisher"
	socketsmocks "github.com/anicoll/winet-integration/mocks/sockets"
	ws "github.com/anicoll/winet-integration/pkg/sockets"
)

// noopPublisher satisfies publisher.DataPublisher without doing anything.
type noopPublisher struct{}

func (noopPublisher) PublishData(_ context.Context, _ map[model.Device][]model.DeviceStatus) error {
	return nil
}

func (noopPublisher) RegisterDevice(_ context.Context, _ *model.Device) error {
	return nil
}

// newTestService creates a service wired for unit tests.
func newTestService() *service {
	svc := New(&config.WinetConfig{Username: "user", Password: "pass"}, noopPublisher{})
	svc.ctx = context.Background()
	svc.properties = map[string]string{} // bypass HTTP fetch in getProperties
	svc.loginReady = make(chan struct{}) // pre-init so handlers don't panic
	return svc
}

// --- Initialisation ---

func TestNew_InitializesChannels(t *testing.T) {
	svc := New(&config.WinetConfig{}, noopPublisher{})
	assert.NotNil(t, svc.events, "events must be non-nil after New()")
}

func TestEvents_ReturnsSameChannel(t *testing.T) {
	svc := New(&config.WinetConfig{}, noopPublisher{})
	ch1 := svc.Events()
	ch2 := svc.Events()
	assert.Equal(t, ch1, ch2, "Events() must always return the same channel")
}

// --- pendingCmd ---

func TestPendingCmd_DeliverThenWait(t *testing.T) {
	var p pendingCmd
	want := "hello"

	go func() {
		time.Sleep(10 * time.Millisecond)
		p.deliver(want)
	}()

	got, err := p.wait(context.Background())
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestPendingCmd_WaitCancelledContext(t *testing.T) {
	var p pendingCmd
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := p.wait(ctx)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestPendingCmd_WaitShortTimeout(t *testing.T) {
	// Override waiterTimeout locally isn't possible, so use a cancelled context
	// to exercise the ctx.Done() path without waiting 30 s.
	var p pendingCmd
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := p.wait(ctx)
	require.Error(t, err, "wait must not block forever when context expires")
}

func TestPendingCmd_DeliverWithNoWaiter_DoesNotPanic(t *testing.T) {
	var p pendingCmd
	assert.NotPanics(t, func() {
		p.deliver("nobody is waiting")
	})
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

func TestHandleConnectMessage_InvalidJSON_SendsToEvents(t *testing.T) {
	svc := newTestService()
	conn := socketsmocks.NewConnection(t)
	// no Send expected — invalid JSON should signal reconnect via events, not attempt a send

	svc.handleConnectMessage([]byte("not-json"), conn)

	select {
	case event := <-svc.events:
		assert.Error(t, event.Err)
	default:
		t.Fatal("expected an error event on events channel for invalid JSON input")
	}
}

func TestHandleLoginMessage_ClosesLoginReady(t *testing.T) {
	svc := newTestService()

	loginResp, err := json.Marshal(model.ParsedResult[model.LoginResponse]{
		ResultCode:    1,
		ResultMessage: "success",
		ResultData:    model.LoginResponse{Token: "login-tok", Service: model.Login.String()},
	})
	require.NoError(t, err)

	conn := socketsmocks.NewConnection(t)
	// No Send expected — poll loop (not handler) sends the device list request now.

	svc.handleLoginMessage(loginResp, conn)

	assert.Equal(t, "login-tok", svc.token)
	select {
	case <-svc.loginReady:
		// closed as expected
	default:
		t.Fatal("loginReady should be closed after handleLoginMessage")
	}
}

// TestOnMessage_LoginTimeout_RoutesToEventsChan validates that the nil-channel panic
// (Bug #1 from the plan) is fixed and that a "login timeout" message is delivered
// safely to the Events() channel without panicking.
func TestOnMessage_LoginTimeout_RoutesToEventsChan(t *testing.T) {
	svc := New(&config.WinetConfig{Host: "127.0.0.1"}, noopPublisher{})
	svc.ctx = context.Background()
	svc.properties = map[string]string{}
	svc.loginReady = make(chan struct{})

	timeoutMsg := `{"result_code":1,"result_msg":"login timeout","result_Data":{"service":"connect"}}`

	// Call synchronously — previously this would panic on nil timeoutErrChan.
	svc.onMessage([]byte(timeoutMsg), nil)

	select {
	case event := <-svc.Events():
		assert.ErrorIs(t, event.Err, ErrTimeout)
	case <-time.After(time.Second):
		t.Fatal("expected ErrTimeout on Events() channel within 1s")
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
// failure triggers a reconnect event rather than panicking.
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
	case event := <-svc.events:
		assert.ErrorContains(t, event.Err, "network write failed")
	default:
		t.Fatal("expected send error on events channel")
	}
}

// startWaiting launches a goroutine that blocks on pending.wait and returns a
// channel that receives the delivered value. The 10 ms sleep gives the goroutine
// time to reach the wait before the caller triggers a deliver.
func startWaiting(svc *service) <-chan any {
	ch := make(chan any, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		v, _ := svc.pending.wait(ctx)
		ch <- v
	}()
	time.Sleep(10 * time.Millisecond)
	return ch
}

// --- handleDeviceListMessage ---

func TestHandleDeviceListMessage_ValidJSON_DeliversList(t *testing.T) {
	svc := newTestService()
	objects := []model.DeviceListObject{
		{DeviceID: 1, DevModel: "XH3000", DevSN: "SN001", DevType: model.DeviceTypeInverter},
	}
	body, err := json.Marshal(model.ParsedResult[model.GenericReponse[model.DeviceListObject]]{
		ResultCode: 1, ResultMessage: "success",
		ResultData: model.GenericReponse[model.DeviceListObject]{
			Count: 1, Service: model.DeviceList.String(), List: objects,
		},
	})
	require.NoError(t, err)

	received := startWaiting(svc)
	svc.handleDeviceListMessage(body, nil)

	select {
	case v := <-received:
		list, ok := v.([]model.DeviceListObject)
		require.True(t, ok, "expected []DeviceListObject from pending")
		require.Len(t, list, 1)
		assert.Equal(t, "SN001", list[0].DevSN)
	case <-time.After(time.Second):
		t.Fatal("pending not delivered within 1s")
	}
}

func TestHandleDeviceListMessage_InvalidJSON_SendsToEvents(t *testing.T) {
	svc := newTestService()
	svc.handleDeviceListMessage([]byte("not-json"), nil)
	select {
	case event := <-svc.events:
		assert.Error(t, event.Err)
	default:
		t.Fatal("expected error event on events channel")
	}
}

// --- sendDeviceListRequest ---

func TestSendDeviceListRequest_SendsCorrectPayload(t *testing.T) {
	svc := newTestService()
	svc.token = "test-token"

	conn := socketsmocks.NewConnection(t)
	var captured ws.Msg
	conn.EXPECT().Send(mock.Anything).RunAndReturn(func(msg ws.Msg) error {
		captured = msg
		return nil
	})

	svc.sendDeviceListRequest(context.Background(), conn)

	var req model.DeviceListRequest
	require.NoError(t, json.Unmarshal(captured.Body, &req))
	assert.Equal(t, model.DeviceList.String(), req.Service)
	assert.Equal(t, "test-token", req.Token)
	assert.Equal(t, "0", req.Type)
}

func TestSendDeviceListRequest_SendError_RoutesToEvents(t *testing.T) {
	svc := newTestService()
	conn := socketsmocks.NewConnection(t)
	conn.EXPECT().Send(mock.Anything).Return(errors.New("send failed"))

	svc.sendDeviceListRequest(context.Background(), conn)

	select {
	case event := <-svc.events:
		assert.ErrorContains(t, event.Err, "send failed")
	default:
		t.Fatal("expected error event on events channel")
	}
}

// --- reconnect / context-cancellation fixes ---

// TestSendDeviceListRequest_CancelledContext_SuppressesEvents covers the fix in login.go:
// when the poll context is cancelled (reconnect in progress), a Send failure must not
// signal a reconnect — the reconnect is already in progress.
func TestSendDeviceListRequest_CancelledContext_SuppressesEvents(t *testing.T) {
	svc := newTestService()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	conn := socketsmocks.NewConnection(t)
	conn.EXPECT().Send(mock.Anything).Return(errors.New("connection not established"))

	svc.sendDeviceListRequest(ctx, conn)

	select {
	case event := <-svc.events:
		t.Fatalf("events must be silent when context is cancelled, got: %v", event.Err)
	default:
	}
}

// TestQueryDevices_CancelledContext_SendQueryErrorSuppressed covers the fix in poller.go:
// a sendQueryRequest failure that occurs because the poll context was cancelled for
// reconnection must not signal another reconnect — one is already in progress.
func TestQueryDevices_CancelledContext_SendQueryErrorSuppressed(t *testing.T) {
	svc := newTestService()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	conn := socketsmocks.NewConnection(t)
	conn.EXPECT().Send(mock.Anything).Return(errors.New("connection not established"))
	svc.conn = conn

	svc.queryDevices(ctx, []model.DeviceListObject{
		{DeviceID: 1, DevModel: "XH3000", DevSN: "SN001", DevType: model.DeviceTypeInverter},
	})

	select {
	case event := <-svc.events:
		t.Fatalf("events must be silent when context is cancelled, got: %v", event.Err)
	default:
	}
}

// TestQueryDevices_CancelledContext_RegisterDeviceErrorSuppressed covers the fix in poller.go:
// a RegisterDevice failure (e.g. ORA-01013 from a cancelled DB context) must not signal
// a reconnect when the poll context is already cancelled — one is already in progress.
func TestQueryDevices_CancelledContext_RegisterDeviceErrorSuppressed(t *testing.T) {
	pub := publishermocks.NewDataPublisher(t)
	svc := New(&config.WinetConfig{}, pub)
	svc.ctx = context.Background()
	svc.properties = map[string]string{}
	svc.loginReady = make(chan struct{})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	pub.EXPECT().RegisterDevice(mock.Anything, mock.Anything).Return(errors.New("db context cancelled"))

	// conn is nil so the device-stages loop exits immediately after RegisterDevice.
	svc.queryDevices(ctx, []model.DeviceListObject{
		{DeviceID: 1, DevModel: "XH3000", DevSN: "SN001", DevType: model.DeviceTypeInverter},
	})

	select {
	case event := <-svc.events:
		t.Fatalf("events must be silent when context is cancelled, got: %v", event.Err)
	default:
	}
}

// TestOnError_CancelledMainContext_SuppressesEvents covers the fix in winet.go:
// when the main context is done (e.g. errgroup cancelled the app), errors from the
// sockets layer during Dial (e.g. "operation was canceled") must be suppressed entirely.
func TestOnError_CancelledMainContext_SuppressesEvents(t *testing.T) {
	svc := newTestService()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	svc.ctx = ctx

	svc.onError(errors.New("dial tcp 192.168.1.1:443: operation was canceled"))

	select {
	case event := <-svc.events:
		t.Fatalf("events must be silent when main context is cancelled, got: %v", event.Err)
	default:
	}
}

// TestOnError_ActiveContext_SendsToEvents ensures that onError routes connection
// errors to the events channel so startWinetService can reconnect.
func TestOnError_ActiveContext_SendsToEvents(t *testing.T) {
	svc := newTestService()

	svc.onError(errors.New("read tcp: connection reset by peer"))

	select {
	case event := <-svc.events:
		assert.ErrorContains(t, event.Err, "connection reset by peer")
	default:
		t.Fatal("expected error on events channel for active context")
	}
}

// --- handleParamMessage ---

func TestHandleParamMessage_ValidJSON_DeliversToPending(t *testing.T) {
	svc := newTestService()

	body, err := json.Marshal(model.ParsedResult[model.GenericReponse[model.InverterParamResponse]]{
		ResultCode: 1, ResultMessage: "success",
		ResultData: model.GenericReponse[model.InverterParamResponse]{
			Count: 1, Service: model.Param.String(),
			List: []model.InverterParamResponse{{ParamID: 1}},
		},
	})
	require.NoError(t, err)

	received := startWaiting(svc)
	svc.handleParamMessage(body, nil)

	select {
	case <-received:
	case <-time.After(time.Second):
		t.Fatal("pending not delivered for param message within 1s")
	}
}

func TestHandleParamMessage_InvalidJSON_SendsToEvents(t *testing.T) {
	svc := newTestService()
	svc.handleParamMessage([]byte("not-json"), nil)
	select {
	case event := <-svc.events:
		assert.Error(t, event.Err)
	default:
		t.Fatal("expected error event on events channel")
	}
}

// --- handleRealMessage ---

func TestHandleRealMessage_InvalidJSON_SendsToEvents(t *testing.T) {
	svc := newTestService()
	svc.handleRealMessage([]byte("not-json"))
	select {
	case event := <-svc.events:
		assert.Error(t, event.Err)
	default:
		t.Fatal("expected error event on events channel")
	}
}

func TestHandleRealMessage_NilCurrentDevice_DoesNotDeliver(t *testing.T) {
	svc := newTestService()
	// currentDevice is nil — handler must return early without delivering to pending.

	body, _ := json.Marshal(model.ParsedResult[model.GenericReponse[model.GenericUnit]]{
		ResultCode: 1, ResultMessage: "success",
		ResultData: model.GenericReponse[model.GenericUnit]{Service: model.Real.String()},
	})

	assert.NotPanics(t, func() { svc.handleRealMessage(body) })

	select {
	case event := <-svc.events:
		t.Fatalf("unexpected event: %v", event.Err)
	default:
	}
}

func TestHandleRealMessage_Valid_DeliversToPending(t *testing.T) {
	svc := newTestService()
	svc.deviceMu.Lock()
	svc.currentDevice = &model.Device{ID: "1", Model: "XH3000", SerialNumber: "SN001"}
	svc.deviceMu.Unlock()

	body, err := json.Marshal(model.ParsedResult[model.GenericReponse[model.GenericUnit]]{
		ResultCode: 1, ResultMessage: "success",
		ResultData: model.GenericReponse[model.GenericUnit]{
			Count: 1, Service: model.Real.String(),
			List: []model.GenericUnit{
				{DataName: "battery_power", DataValue: "5.0", DataUnit: model.NumericUnitKiloWatt},
			},
		},
	})
	require.NoError(t, err)

	received := startWaiting(svc)
	svc.handleRealMessage(body)

	select {
	case <-received:
	case <-time.After(time.Second):
		t.Fatal("handleRealMessage did not deliver to pending within 1s")
	}
}

// --- handleDirectMessage ---

func TestHandleDirectMessage_InvalidJSON_SendsToEvents(t *testing.T) {
	svc := newTestService()
	svc.handleDirectMessage([]byte("not-json"))
	select {
	case event := <-svc.events:
		assert.Error(t, event.Err)
	default:
		t.Fatal("expected error event on events channel")
	}
}

func TestHandleDirectMessage_NilCurrentDevice_DoesNotDeliver(t *testing.T) {
	svc := newTestService()

	body, _ := json.Marshal(model.ParsedResult[model.GenericReponse[model.DirectUnit]]{
		ResultCode: 1, ResultMessage: "success",
		ResultData: model.GenericReponse[model.DirectUnit]{Service: model.Direct.String()},
	})

	assert.NotPanics(t, func() { svc.handleDirectMessage(body) })

	select {
	case event := <-svc.events:
		t.Fatalf("unexpected event: %v", event.Err)
	default:
	}
}

func TestHandleDirectMessage_Valid_DeliversToPending(t *testing.T) {
	svc := newTestService()
	svc.deviceMu.Lock()
	svc.currentDevice = &model.Device{ID: "1", Model: "XH3000", SerialNumber: "SN001"}
	svc.deviceMu.Unlock()

	v, c := "12.5", "1.5"
	body, err := json.Marshal(model.ParsedResult[model.GenericReponse[model.DirectUnit]]{
		ResultCode: 1, ResultMessage: "success",
		ResultData: model.GenericReponse[model.DirectUnit]{
			Count: 1, Service: model.Direct.String(),
			List: []model.DirectUnit{
				{Name: "PV1", Voltage: v, VoltageUnit: model.NumericUnitVolt, Current: c, CurrentUnit: model.NumericUnitAmp},
			},
		},
	})
	require.NoError(t, err)

	received := startWaiting(svc)
	svc.handleDirectMessage(body)

	select {
	case <-received:
	case <-time.After(time.Second):
		t.Fatal("handleDirectMessage did not deliver to pending within 1s")
	}
}

// --- runPollLoop ---

func TestRunPollLoop_ContextCancelledBeforeLogin_ExitsCleanly(t *testing.T) {
	svc := newTestService()
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		svc.runPollLoop(ctx)
		close(done)
	}()

	cancel() // cancel before loginReady is ever closed

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("runPollLoop did not exit within 1s after context cancellation")
	}
}

func TestRunPollLoop_SendsDeviceListAfterLogin(t *testing.T) {
	svc := newTestService()
	svc.cfg.PollInterval = time.Hour // keep loop from cycling during the test

	conn := socketsmocks.NewConnection(t)
	sent := make(chan ws.Msg, 1)
	conn.Mock.On("Send", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		select {
		case sent <- args.Get(0).(ws.Msg):
		default:
		}
		// Deliver an empty device list after a short delay so pending.wait unblocks.
		time.AfterFunc(2*time.Millisecond, func() {
			svc.pending.deliver([]model.DeviceListObject{})
		})
	})
	svc.conn = conn

	ctx := t.Context()

	go svc.runPollLoop(ctx)
	close(svc.loginReady)

	var msg ws.Msg
	select {
	case msg = <-sent:
	case <-time.After(time.Second):
		t.Fatal("poll loop did not send within 1s after login")
	}

	var dlReq model.DeviceListRequest
	require.NoError(t, json.Unmarshal(msg.Body, &dlReq))
	assert.Equal(t, model.DeviceList.String(), dlReq.Service)
	assert.Equal(t, EnglishLang, dlReq.Lang)
}

// --- queryDevices ---

func TestQueryDevices_SkipsDevicesWithNoStages(t *testing.T) {
	svc := newTestService()
	conn := socketsmocks.NewConnection(t)
	// no Send expected — device type 0 has no stages
	svc.conn = conn

	svc.queryDevices(context.Background(), []model.DeviceListObject{
		{DeviceID: 99, DevModel: "Unknown", DevSN: "SN-X", DevType: 0},
	})

	svc.deviceMu.RLock()
	cd := svc.currentDevice
	svc.deviceMu.RUnlock()
	assert.Nil(t, cd, "currentDevice must not be set for unknown device types")
}

func TestQueryDevices_SendsOneRequestPerStage(t *testing.T) {
	svc := newTestService()
	svc.token = "tok"

	conn := socketsmocks.NewConnection(t)
	svc.conn = conn
	// DeviceTypeInverter has 3 stages: Real, RealBattery, Direct.
	// Each Send is followed by a tiny delay so pending.wait() can set up p.ch first.
	conn.Mock.On("Send", mock.Anything).Return(nil).Run(func(_ mock.Arguments) {
		time.AfterFunc(2*time.Millisecond, func() {
			svc.pending.deliver(struct{}{})
		})
	})

	svc.queryDevices(context.Background(), []model.DeviceListObject{
		{DeviceID: 1, DevModel: "XH3000", DevSN: "SN001", DevType: model.DeviceTypeInverter},
	})

	assert.Equal(t, 3, len(conn.Calls), "expected one Send per device stage (Real, RealBattery, Direct)")

	svc.deviceMu.RLock()
	cd := svc.currentDevice
	svc.deviceMu.RUnlock()
	require.NotNil(t, cd)
	assert.Equal(t, "SN001", cd.SerialNumber)
}

// --- publisher integration ---

func TestQueryDevices_CallsRegisterDeviceOnPublisher(t *testing.T) {
	pub := publishermocks.NewDataPublisher(t)
	svc := New(&config.WinetConfig{}, pub)
	svc.ctx = context.Background()
	svc.properties = map[string]string{}
	svc.loginReady = make(chan struct{})

	conn := socketsmocks.NewConnection(t)
	svc.conn = conn
	conn.Mock.On("Send", mock.Anything).Return(nil).Run(func(_ mock.Arguments) {
		time.AfterFunc(2*time.Millisecond, func() { svc.pending.deliver(struct{}{}) })
	})

	pub.EXPECT().RegisterDevice(mock.Anything, mock.MatchedBy(func(d *model.Device) bool {
		return d.SerialNumber == "SN001"
	})).Return(nil)

	svc.queryDevices(context.Background(), []model.DeviceListObject{
		{DeviceID: 1, DevModel: "XH3000", DevSN: "SN001", DevType: model.DeviceTypeInverter},
	})
}

func TestHandleRealMessage_CallsPublishData(t *testing.T) {
	pub := publishermocks.NewDataPublisher(t)
	svc := New(&config.WinetConfig{}, pub)
	svc.ctx = context.Background()
	svc.properties = map[string]string{}
	svc.loginReady = make(chan struct{})
	svc.deviceMu.Lock()
	svc.currentDevice = &model.Device{ID: "1", Model: "XH3000", SerialNumber: "SN001"}
	svc.deviceMu.Unlock()

	body, err := json.Marshal(model.ParsedResult[model.GenericReponse[model.GenericUnit]]{
		ResultCode: 1, ResultMessage: "success",
		ResultData: model.GenericReponse[model.GenericUnit]{
			Count: 1, Service: model.Real.String(),
			List: []model.GenericUnit{
				{DataName: "battery_power", DataValue: "5.0", DataUnit: model.NumericUnitKiloWatt},
			},
		},
	})
	require.NoError(t, err)

	pub.EXPECT().PublishData(mock.Anything, mock.MatchedBy(func(m map[model.Device][]model.DeviceStatus) bool {
		return len(m) == 1
	})).Return(nil)

	received := startWaiting(svc)
	svc.handleRealMessage(body)

	select {
	case <-received:
	case <-time.After(time.Second):
		t.Fatal("pending not delivered within 1s")
	}
}

func TestHandleDirectMessage_CallsPublishData(t *testing.T) {
	pub := publishermocks.NewDataPublisher(t)
	svc := New(&config.WinetConfig{}, pub)
	svc.ctx = context.Background()
	svc.properties = map[string]string{}
	svc.loginReady = make(chan struct{})
	svc.deviceMu.Lock()
	svc.currentDevice = &model.Device{ID: "1", Model: "XH3000", SerialNumber: "SN001"}
	svc.deviceMu.Unlock()

	v, c := "12.5", "1.5"
	body, err := json.Marshal(model.ParsedResult[model.GenericReponse[model.DirectUnit]]{
		ResultCode: 1, ResultMessage: "success",
		ResultData: model.GenericReponse[model.DirectUnit]{
			Count: 1, Service: model.Direct.String(),
			List: []model.DirectUnit{
				{Name: "PV1", Voltage: v, VoltageUnit: model.NumericUnitVolt, Current: c, CurrentUnit: model.NumericUnitAmp},
			},
		},
	})
	require.NoError(t, err)

	pub.EXPECT().PublishData(mock.Anything, mock.MatchedBy(func(m map[model.Device][]model.DeviceStatus) bool {
		return len(m) == 1
	})).Return(nil)

	received := startWaiting(svc)
	svc.handleDirectMessage(body)

	select {
	case <-received:
	case <-time.After(time.Second):
		t.Fatal("pending not delivered within 1s")
	}
}

// --- sendIfErr crashloop regression ---

// TestSendIfErr_RoutesToEventsNeverErrChan is the canonical regression test for the
// crashloop bug: any internal winet error must route to the events channel so
// startWinetService can reconnect — it must NEVER block-send to a fatal errChan
// that causes handleErrors to shut down the entire application.
func TestSendIfErr_RoutesToEventsNeverErrChan(t *testing.T) {
	svc := newTestService()
	svc.sendIfErr(errors.New("connection dropped unexpectedly"))

	select {
	case event := <-svc.events:
		assert.Error(t, event.Err)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("sendIfErr must deliver to events channel — got nothing")
	}
}

// --- nil-conn / restart race regression tests ---

// TestSendQueryRequest_NilConn_ReturnsError is the primary regression test for the
// restart nil-pointer panic. Before the fix, sendQueryRequest called s.conn.Send()
// without a nil check; a concurrent reconnect() that set s.conn=nil would panic.
// After the fix it must return a descriptive error instead.
func TestSendQueryRequest_NilConn_ReturnsError(t *testing.T) {
	svc := newTestService()
	// conn is intentionally left nil — simulates the window during reconnect.

	err := svc.sendQueryRequest(model.Real, 1)

	require.Error(t, err)
	assert.ErrorContains(t, err, "connection is nil")
}

// TestSendQueryRequest_ConcurrentReconnect_DoesNotPanic reproduces the exact race that
// caused the production nil-pointer panic: one goroutine calls sendQueryRequest while
// another toggles s.conn between nil and a live mock (simulating reconnect()).
// Run with -race to confirm no data races remain.
func TestSendQueryRequest_ConcurrentReconnect_DoesNotPanic(t *testing.T) {
	svc := newTestService()

	conn := socketsmocks.NewConnection(t)
	conn.Mock.On("Send", mock.Anything).Return(nil).Maybe()
	conn.Mock.On("Close").Return(nil).Maybe()

	svc.connMu.Lock()
	svc.conn = conn
	svc.connMu.Unlock()

	const iterations = 500
	var wg sync.WaitGroup

	// Goroutine 1: continuously send queries — must never panic.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			_ = svc.sendQueryRequest(model.Real, 1)
		}
	}()

	// Goroutine 2: simulate reconnect toggling conn nil → non-nil under the lock.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			svc.connMu.Lock()
			svc.conn = nil
			svc.connMu.Unlock()
			svc.connMu.Lock()
			svc.conn = conn
			svc.connMu.Unlock()
		}
	}()

	assert.NotPanics(t, func() { wg.Wait() })
}

// TestOnMessage_LoginTimeout_NoInBandReconnect verifies the other half of the fix:
// a "login timeout" message must NOT call reconnect() in-band. Previously the
// in-band reconnect() set s.conn=nil while the poll-loop goroutine was actively
// reading it, causing the panic. Now only the events channel signal is sent and
// the existing connection must remain untouched until startWinetService calls
// Connect() with the poll loop properly cancelled.
func TestOnMessage_LoginTimeout_NoInBandReconnect(t *testing.T) {
	svc := New(&config.WinetConfig{Host: "127.0.0.1"}, noopPublisher{})
	svc.ctx = context.Background()
	svc.properties = map[string]string{}
	svc.loginReady = make(chan struct{})

	conn := socketsmocks.NewConnection(t)
	// Close must NOT be called — the in-band reconnect() would have called it.
	// testify/mock's AssertExpectations (via t.Cleanup) will fail if Close is called.
	svc.conn = conn

	timeoutMsg := `{"result_code":1,"result_msg":"login timeout","result_Data":{"service":"connect"}}`
	svc.onMessage([]byte(timeoutMsg), nil)

	// The ErrTimeout event must still be signalled so startWinetService reconnects.
	select {
	case event := <-svc.Events():
		assert.ErrorIs(t, event.Err, ErrTimeout)
	case <-time.After(time.Second):
		t.Fatal("expected ErrTimeout on Events() channel within 1s")
	}

	// conn must still be non-nil — reconnect() was not called in-band.
	svc.connMu.RLock()
	currentConn := svc.conn
	svc.connMu.RUnlock()
	assert.Same(t, conn, currentConn, "conn must not be replaced by an in-band reconnect")
}
