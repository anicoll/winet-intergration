package logic_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anicoll/winet-integration/internal/pkg/logic"
	"github.com/anicoll/winet-integration/internal/pkg/store"
	logicmocks "github.com/anicoll/winet-integration/mocks/logic"
)

// currentPrice builds a non-forecast price active right now.
func currentPrice(channelType string, perKwh float64) store.Amberprice {
	now := time.Now().UTC()
	return store.Amberprice{
		ChannelType: channelType,
		StartTime:   now.Add(-5 * time.Minute),
		EndTime:     now.Add(5 * time.Minute),
		PerKwh:      perKwh,
		Forecast:    false,
	}
}

func TestNextBestAction_NoPrices_NoAction(t *testing.T) {
	w := logicmocks.NewWinetCommands(t)
	db := logicmocks.NewDatabase(t)
	db.EXPECT().GetAmberPrices(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, nil)

	svc := logic.NewLogicSvc(w, db)
	require.NoError(t, svc.NextBestAction(context.Background()))
	// No winet commands expected — mock verifies via AssertExpectations in t.Cleanup
}

func TestNextBestAction_NegativeGeneralPrice_SendsCharge(t *testing.T) {
	w := logicmocks.NewWinetCommands(t)
	db := logicmocks.NewDatabase(t)
	prices := []store.Amberprice{
		currentPrice("general", -0.05),
		currentPrice("feedin", 0.10),
	}
	db.EXPECT().GetAmberPrices(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(prices, nil)
	w.EXPECT().SendChargeCommand("6.6").Return(true, nil)
	w.EXPECT().SetFeedInLimitation(false).Return(true, nil)

	svc := logic.NewLogicSvc(w, db)
	require.NoError(t, svc.NextBestAction(context.Background()))
}

func TestNextBestAction_ZeroGeneralPrice_SendsSelfConsumption(t *testing.T) {
	w := logicmocks.NewWinetCommands(t)
	db := logicmocks.NewDatabase(t)
	prices := []store.Amberprice{
		currentPrice("general", 0),
		currentPrice("feedin", 0.10),
	}
	db.EXPECT().GetAmberPrices(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(prices, nil)
	w.EXPECT().SendSelfConsumptionCommand().Return(true, nil)
	w.EXPECT().SetFeedInLimitation(false).Return(true, nil)

	svc := logic.NewLogicSvc(w, db)
	require.NoError(t, svc.NextBestAction(context.Background()))
}

func TestNextBestAction_PositiveGeneralPrice_SendsSelfConsumption(t *testing.T) {
	w := logicmocks.NewWinetCommands(t)
	db := logicmocks.NewDatabase(t)
	prices := []store.Amberprice{
		currentPrice("general", 0.25),
		currentPrice("feedin", 0.10),
	}
	db.EXPECT().GetAmberPrices(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(prices, nil)
	w.EXPECT().SendSelfConsumptionCommand().Return(true, nil)
	w.EXPECT().SetFeedInLimitation(false).Return(true, nil)

	svc := logic.NewLogicSvc(w, db)
	require.NoError(t, svc.NextBestAction(context.Background()))
}

func TestNextBestAction_NegativeFeedinPrice_LimitsFeedin(t *testing.T) {
	w := logicmocks.NewWinetCommands(t)
	db := logicmocks.NewDatabase(t)
	prices := []store.Amberprice{
		currentPrice("general", 0.20),
		currentPrice("feedin", -0.10),
	}
	db.EXPECT().GetAmberPrices(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(prices, nil)
	w.EXPECT().SendSelfConsumptionCommand().Return(true, nil)
	w.EXPECT().SetFeedInLimitation(true).Return(true, nil)

	svc := logic.NewLogicSvc(w, db)
	require.NoError(t, svc.NextBestAction(context.Background()))
}

func TestNextBestAction_PositiveFeedinPrice_EnablesFeedin(t *testing.T) {
	w := logicmocks.NewWinetCommands(t)
	db := logicmocks.NewDatabase(t)
	prices := []store.Amberprice{
		currentPrice("general", 0.20),
		currentPrice("feedin", 0.10),
	}
	db.EXPECT().GetAmberPrices(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(prices, nil)
	w.EXPECT().SendSelfConsumptionCommand().Return(true, nil)
	w.EXPECT().SetFeedInLimitation(false).Return(true, nil)

	svc := logic.NewLogicSvc(w, db)
	require.NoError(t, svc.NextBestAction(context.Background()))
}

func TestNextBestAction_ForecastPricesAreIgnored(t *testing.T) {
	w := logicmocks.NewWinetCommands(t)
	db := logicmocks.NewDatabase(t)
	now := time.Now().UTC()
	prices := []store.Amberprice{
		{
			ChannelType: "general",
			StartTime:   now.Add(-5 * time.Minute),
			EndTime:     now.Add(5 * time.Minute),
			PerKwh:      -0.05,
			Forecast:    true,
		},
	}
	db.EXPECT().GetAmberPrices(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(prices, nil)

	svc := logic.NewLogicSvc(w, db)
	require.NoError(t, svc.NextBestAction(context.Background()))
	// No winet commands expected — forecast price must be ignored
}

func TestNextBestAction_DBError_ReturnsError(t *testing.T) {
	w := logicmocks.NewWinetCommands(t)
	db := logicmocks.NewDatabase(t)
	db.EXPECT().GetAmberPrices(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.New("db connection lost"))

	svc := logic.NewLogicSvc(w, db)
	require.Error(t, svc.NextBestAction(context.Background()))
}
