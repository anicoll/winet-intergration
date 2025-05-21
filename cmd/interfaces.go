package cmd

import (
	"context"
)

// WinetService defines the interface that cmd.run expects from a winet service.
type WinetService interface {
	Connect(ctx context.Context) error
	SubscribeToTimeout() chan error
	// Methods needed by server.New(winetSvc, db)
	GetDeviceList(ctx context.Context) any
	SendSelfConsumptionCommand() (bool, error)
	SendDischargeCommand(dischargePower string) (bool, error)
	SendChargeCommand(chargePower string) (bool, error)
	SendBatteryStopCommand() (bool, error)
	SendInverterStateChangeCommand(disable bool) (bool, error)
	SetFeedInLimitation(feedinLimited bool) (bool, error)
}
