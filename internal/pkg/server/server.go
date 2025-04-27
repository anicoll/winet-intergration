package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/anicoll/winet-integration/internal/pkg/model"
	api "github.com/anicoll/winet-integration/pkg/server"
	"go.uber.org/zap"
)

var _ api.ServerInterface = (*server)(nil)

type winetService interface {
	SendSelfConsumptionCommand() (bool, error)
	SetFeedInLimitation(feedinLimited bool) (bool, error)
	// like 6.6
	SendDischargeCommand(dischargePower string) (bool, error)
	// like 6.6
	SendChargeCommand(chargePower string) (bool, error)
	SendInverterStateChangeCommand(disable bool) (bool, error)
}

type database interface {
	GetLatestProperties(ctx context.Context) (model.Properties, error)
	GetProperties(ctx context.Context, identifier, slug string, from, to *time.Time) (model.Properties, error)
}

type server struct {
	winets winetService
	db     database
	logger *zap.Logger
}

// GetProperties implements api.ServerInterface.
func (s *server) GetProperties(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	props, err := s.db.GetLatestProperties(ctx)
	if err != nil {
		handleError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(props); err != nil {
		handleError(w, err)
		return
	}
}

// GetPropertyIdentifierSlug implements api.ServerInterface.
func (s *server) GetPropertyIdentifierSlug(w http.ResponseWriter, r *http.Request, identifier string, slug string, params api.GetPropertyIdentifierSlugParams) {
	ctx := r.Context()
	props, err := s.db.GetProperties(ctx, identifier, slug, params.From, params.To)
	if err != nil {
		handleError(w, err)
		return
	}
	if err := json.NewEncoder(w).Encode(props); err != nil {
		handleError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
}

func New(ws winetService, db database) *server {
	return &server{
		winets: ws,
		logger: zap.L(),
		db:     db,
	}
}

func (s *server) PostBatteryState(w http.ResponseWriter, r *http.Request, state string) {
	changeStateReq, err := unmarshalPayload[api.ChangeBatteryStatePayload](r)
	if err != nil {
		handleError(w, err)
		return
	}

	if err := s.changeBatteryState(changeStateReq); err != nil {
		handleError(w, err)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("success"))
}

func (s *server) PostInverterFeedin(w http.ResponseWriter, r *http.Request) {
	feedinReq, err := unmarshalPayload[api.ChangeFeedinPayload](r)
	if err != nil {
		handleError(w, err)
		return
	}

	success, err := s.winets.SetFeedInLimitation(feedinReq.Disable)
	if err != nil {
		handleError(w, err)
		return
	}
	if !success {
		handleError(w, errors.New("failed to set feedin limitation"))
		return
	}
	s.logger.Info("limit feed in switched", zap.Bool("disable_feedin", feedinReq.Disable))
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("success"))
}

func (s *server) PostInverterState(w http.ResponseWriter, r *http.Request, state string) {
	success, err := s.winets.SendInverterStateChangeCommand(state == string(api.Off))
	if err != nil {
		handleError(w, err)
		return
	}
	if !success {
		err = errors.New("failed to change inverter state to " + state)
		handleError(w, err)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("success"))
}

func (s *server) changeBatteryState(req *api.ChangeBatteryStatePayload) error {
	switch req.State {
	case api.SelfConsumption:
		s.logger.Info("switching battery to", zap.String("state", string(req.State)))
		success, err := s.winets.SendSelfConsumptionCommand()
		if err != nil {
			return err
		}
		if !success {
			return errors.New("failed to send discharge command")
		}
	case api.Charge:
		if req.Power == nil {
			return errors.New("power param cannot be empty")
		}
		s.logger.Info("switching battery to", zap.String("state", string(req.State)), zap.String("power", *req.Power))
		success, err := s.winets.SendChargeCommand(*req.Power)
		if err != nil {
			return err
		}
		if !success {
			return errors.New("failed to send discharge command")
		}
		// handle Charge power request
	case api.Discharge:
		if req.Power == nil {
			return errors.New("power param cannot be empty")
		}
		s.logger.Info("switching battery to", zap.String("state", string(req.State)), zap.String("power", *req.Power))
		success, err := s.winets.SendDischargeCommand(*req.Power)
		if err != nil {
			return err
		}
		if !success {
			return errors.New("failed to send discharge command")
		}
	}
	return nil
}

func handleError(w http.ResponseWriter, err error) {
	w.WriteHeader(http.StatusInternalServerError)
	w.Write([]byte(err.Error()))
}

func unmarshalPayload[T any](r *http.Request) (*T, error) {
	data, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	var out T
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
