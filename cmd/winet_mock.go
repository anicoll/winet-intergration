package cmd

import (
	"context"
	"errors"
	// "github.com/anicoll/winet-integration/internal/pkg/winet" // Avoid direct import if using interface
)

// MockWinetService is a mock implementation of the WinetService interface.
type MockWinetService struct {
	ConnectFunc            func(ctx context.Context) error
	SubscribeToTimeoutFunc func() chan error
	GetDeviceListFunc      func(ctx context.Context) any

	// Mock implementations for command methods
	SendSelfConsumptionCommandFunc       func() (bool, error)
	SendDischargeCommandFunc             func(dischargePower string) (bool, error)
	SendChargeCommandFunc                func(chargePower string) (bool, error)
	SendBatteryStopCommandFunc           func() (bool, error)
	SendInverterStateChangeCommandFunc   func(disable bool) (bool, error)
	SetFeedInLimitationFunc              func(feedinLimited bool) (bool, error)
}

func (m *MockWinetService) Connect(ctx context.Context) error {
	if m.ConnectFunc != nil {
		return m.ConnectFunc(ctx)
	}
	return nil
}

func (m *MockWinetService) SubscribeToTimeout() chan error {
	if m.SubscribeToTimeoutFunc != nil {
		return m.SubscribeToTimeoutFunc()
	}
	// Return a closed channel by default, or a channel that never sends,
	// to avoid blocking tests indefinitely if not specifically testing timeouts.
	ch := make(chan error)
	// close(ch) // Option 1: closed channel
	return ch // Option 2: channel that never sends (test must use timeout)
}

func (m *MockWinetService) GetDeviceList(ctx context.Context) any {
	if m.GetDeviceListFunc != nil {
		return m.GetDeviceListFunc(ctx)
	}
	return nil
}

func (m *MockWinetService) SendSelfConsumptionCommand() (bool, error) {
	if m.SendSelfConsumptionCommandFunc != nil {
		return m.SendSelfConsumptionCommandFunc()
	}
	return false, errors.New("mocked SendSelfConsumptionCommand not implemented")
}

func (m *MockWinetService) SendDischargeCommand(dischargePower string) (bool, error) {
	if m.SendDischargeCommandFunc != nil {
		return m.SendDischargeCommandFunc(dischargePower)
	}
	return false, errors.New("mocked SendDischargeCommand not implemented")
}

func (m *MockWinetService) SendChargeCommand(chargePower string) (bool, error) {
	if m.SendChargeCommandFunc != nil {
		return m.SendChargeCommandFunc(chargePower)
	}
	return false, errors.New("mocked SendChargeCommand not implemented")
}

func (m *MockWinetService) SendBatteryStopCommand() (bool, error) {
	if m.SendBatteryStopCommandFunc != nil {
		return m.SendBatteryStopCommandFunc()
	}
	return false, errors.New("mocked SendBatteryStopCommand not implemented")
}

func (m *MockWinetService) SendInverterStateChangeCommand(disable bool) (bool, error) {
	if m.SendInverterStateChangeCommandFunc != nil {
		return m.SendInverterStateChangeCommandFunc(disable)
	}
	return false, errors.New("mocked SendInverterStateChangeCommand not implemented")
}

func (m *MockWinetService) SetFeedInLimitation(feedinLimited bool) (bool, error) {
	if m.SetFeedInLimitationFunc != nil {
		return m.SetFeedInLimitationFunc(feedinLimited)
	}
	return false, errors.New("mocked SetFeedInLimitation not implemented")
}
