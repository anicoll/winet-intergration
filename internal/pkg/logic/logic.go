package logic

import (
	"context"
	"iter"
	"time"

	"github.com/samber/lo"

	dbpkg "github.com/anicoll/winet-integration/internal/pkg/database/db"
)

type WinetCommands interface {
	SendSelfConsumptionCommand() (bool, error)
	SendBatteryStopCommand() (bool, error)
	SetFeedInLimitation(feedinLimited bool) (bool, error)
	// like 6.6
	SendDischargeCommand(dischargePower string) (bool, error)
	// like 6.6
	SendChargeCommand(chargePower string) (bool, error)
	SendInverterStateChangeCommand(disable bool) (bool, error)
}

type Database interface {
	GetLatestProperties(ctx context.Context) (iter.Seq[dbpkg.Property], error)
	GetAmberPrices(ctx context.Context, from, to time.Time, site *string) ([]dbpkg.Amberprice, error)
}

type logic struct {
	inverter WinetCommands
	db       Database
}

func NewLogicSvc(wsvc WinetCommands, db Database) *logic {
	return &logic{
		inverter: wsvc,
		db:       db,
	}
}

func getCurrentPrice(prices []dbpkg.Amberprice, channelType string) (dbpkg.Amberprice, bool) {
	now := time.Now().UTC()
	return lo.Find(prices, func(p dbpkg.Amberprice) bool {
		if p.ChannelType != channelType {
			return false
		}
		utcStartTime := p.StartTime.UTC()
		utcEndTime := p.EndTime.UTC()
		if !p.Forecast && utcStartTime.Before(now) && utcEndTime.After(now) {
			return true
		}
		return false
	})
}

func (l *logic) NextBestAction(ctx context.Context) error {
	prices, err := l.db.GetAmberPrices(ctx, time.Now().Add(-time.Hour), time.Now().Add(time.Hour), nil)
	if err != nil {
		return err
	}

	currentGeneralPrice, found := getCurrentPrice(prices, "general")
	if !found {
		// log warning: no general-in price found
		return nil // No general-in price found
	}

	switch pkwh := currentGeneralPrice.PerKwh; {
	case pkwh < 0:
		// full charge
		success, err := l.inverter.SendChargeCommand("6.6")
		if err != nil {
			return err
		}
		if !success {
			// log warning: failed to disable feed-in
			return nil // Failed to disable feed-in
		}
	case pkwh >= 0:
		success, err := l.inverter.SendSelfConsumptionCommand()
		if err != nil {
			return err
		}
		if !success {
			// log warning: failed to disable feed-in
			return nil // Failed to disable feed-in
		}
	}

	currentFeedinPrice, found := getCurrentPrice(prices, "feedin")
	if !found {
		// log warning: no feed-in price found
		return nil // No feed-in price found
	}

	switch pkwh := currentFeedinPrice.PerKwh; {
	case pkwh < 0:
		// disable feed-in
		success, err := l.inverter.SetFeedInLimitation(true)
		if err != nil {
			return err
		}
		if !success {
			// log warning: failed to disable feed-in
			return nil // Failed to disable feed-in
		}
	case pkwh >= 0:
		// disable feed-in
		success, err := l.inverter.SetFeedInLimitation(false)
		if err != nil {
			return err
		}
		if !success {
			// log warning: failed to disable feed-in
			return nil // Failed to disable feed-in
		}
	}

	return nil
}
