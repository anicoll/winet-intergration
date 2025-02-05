package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"go.uber.org/zap"
)

type winetService interface {
	SendSelfConsumptionCommand() (bool, error)
}

// Battery handles requests for charging and discharging.
func Battery(winet winetService) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			handleError(w, err)
		}

		batteryRequest := UpdateBatteryRequest{}
		if err := json.Unmarshal(data, &batteryRequest); err != nil {
			handleError(w, err)
			return
		}
		handleBatterRequest(r.Context(), winet, batteryRequest)
	}
}

func handleError(w http.ResponseWriter, err error) {
	w.WriteHeader(http.StatusInternalServerError)
	w.Write([]byte(err.Error()))
}

func handleBatterRequest(_ context.Context, winet winetService, updateBatteryReq UpdateBatteryRequest) error {
	logger := zap.L()

	switch updateBatteryReq.State {
	case SelfConsumptionState:
		logger.Info("switching battery to", zap.String("state", updateBatteryReq.State.String()))
		success, err := winet.SendSelfConsumptionCommand()
		if err != nil {
			return err
		}
		_ = success
		// change to self consumption
	case ChargeState:
		logger.Info("switching battery to", zap.String("state", updateBatteryReq.State.String()), zap.Int("power", updateBatteryReq.Power))
		// handle Charge power request
	case DischargeState:
		logger.Info("switching battery to", zap.String("state", updateBatteryReq.State.String()), zap.Int("power", updateBatteryReq.Power))
		// handle disCharge power request
	}
	return nil
}

// Inverter handles requests for inverter actions.
func Inverter(winet winetService) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		data := []byte{}
		if _, err := r.Body.Read(data); err != nil {
			handleError(w, err)
			return
		}
		inverterRequest := UpdateInverterRequest{}
		if err := json.Unmarshal(data, &inverterRequest); err != nil {
			handleError(w, err)
			return
		}
		handleInverterRequest(r.Context(), winet, inverterRequest)
	}
}

func handleInverterRequest(_ context.Context, winet winetService, inverterRequest UpdateInverterRequest) error {
	logger := zap.L()
	if inverterRequest.State != nil {
		logger.Info("switching inverter to", zap.String("state", inverterRequest.State.String()))
		return nil
	}

	logger.Info("limit feed in switched", zap.Bool("limit_feed_in", *inverterRequest.LimitFeedIn))

	return nil
}
