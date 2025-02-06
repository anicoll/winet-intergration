package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"go.uber.org/zap"
)

type winetService interface {
	SendSelfConsumptionCommand() (bool, error)
	SetFeedInLimitation(feedinLimited bool) (bool, error)
	// like 6.6
	SendDischargeCommand(dischargePower string) (bool, error)
	// like 6.6
	SendChargeCommand(chargePower string) (bool, error)
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
		if err := handleBatterRequest(r.Context(), winet, batteryRequest); err != nil {
			handleError(w, err)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
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
		if !success {
			return errors.New("failed to send discharge command")
		}
	case ChargeState:
		logger.Info("switching battery to", zap.String("state", updateBatteryReq.State.String()), zap.String("power", updateBatteryReq.Power))
		success, err := winet.SendChargeCommand("6.6")
		if err != nil {
			return err
		}
		if !success {
			return errors.New("failed to send discharge command")
		}
		// handle Charge power request
	case DischargeState:
		logger.Info("switching battery to", zap.String("state", updateBatteryReq.State.String()), zap.String("power", updateBatteryReq.Power))
		success, err := winet.SendDischargeCommand(updateBatteryReq.Power)
		if err != nil {
			return err
		}
		if !success {
			return errors.New("failed to send discharge command")
		}
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
		if err := handleInverterRequest(r.Context(), winet, inverterRequest); err != nil {
			handleError(w, err)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	}
}

func handleInverterRequest(_ context.Context, winet winetService, inverterRequest UpdateInverterRequest) error {
	logger := zap.L()
	if inverterRequest.State != nil {
		logger.Info("switching inverter to", zap.String("state", inverterRequest.State.String()))
		return nil
	}

	logger.Info("limit feed in switched", zap.Bool("limit_feed_in", *inverterRequest.LimitFeedIn))
	if *inverterRequest.LimitFeedIn {
		success, err := winet.SetFeedInLimitation(true)
		if err != nil {
			return err
		}
		if !success {
			return errors.New("failed to set feedin limitation")
		}
	}
	success, err := winet.SetFeedInLimitation(false)
	if err != nil {
		return err
	}
	if !success {
		return errors.New("failed to set feedin limitation")
	}
	return nil
}
