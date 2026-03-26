package main

import (
	"context"
	"encoding/json"
	"fmt"

	"connectrpc.com/connect"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/timestamppb"

	winetv1 "github.com/anicoll/winet-integration/gen/winet/v1"
)

// service implements both IngestionServiceHandler and CommandServiceHandler.
type service struct {
	store  *store
	logger *zap.Logger
}

func newService(s *store, logger *zap.Logger) *service {
	return &service{store: s, logger: logger}
}

// --- IngestionService ---------------------------------------------------------

func (s *service) IngestData(
	ctx context.Context,
	req *connect.Request[winetv1.IngestDataRequest],
) (*connect.Response[winetv1.IngestDataResponse], error) {
	rows := make([]propertyRow, 0, len(req.Msg.DataPoints))
	for _, dp := range req.Msg.DataPoints {
		if dp.Timestamp == nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("data point missing timestamp"))
		}
		rows = append(rows, propertyRow{
			Timestamp:         dp.Timestamp.AsTime(),
			UnitOfMeasurement: dp.UnitOfMeasurement,
			Value:             dp.Value,
			Identifier:        dp.Identifier,
			Slug:              dp.Slug,
		})
	}
	if err := s.store.insertProperties(ctx, rows); err != nil {
		s.logger.Error("IngestData: db error", zap.Error(err))
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	s.logger.Info("IngestData: ok", zap.Int("count", len(rows)))
	return connect.NewResponse(&winetv1.IngestDataResponse{}), nil
}

func (s *service) RegisterDevice(
	ctx context.Context,
	req *connect.Request[winetv1.RegisterDeviceRequest],
) (*connect.Response[winetv1.RegisterDeviceResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("device id is required"))
	}
	if err := s.store.upsertDevice(ctx, req.Msg.Id, req.Msg.Model, req.Msg.SerialNumber); err != nil {
		s.logger.Error("RegisterDevice: db error", zap.Error(err))
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	s.logger.Info("RegisterDevice: ok", zap.String("id", req.Msg.Id))
	return connect.NewResponse(&winetv1.RegisterDeviceResponse{}), nil
}

func (s *service) IngestAmberPrices(
	ctx context.Context,
	req *connect.Request[winetv1.IngestAmberPricesRequest],
) (*connect.Response[winetv1.IngestAmberPricesResponse], error) {
	rows := make([]amberPriceRow, 0, len(req.Msg.Prices))
	for _, p := range req.Msg.Prices {
		if p.StartTime == nil || p.EndTime == nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("amber price missing time fields"))
		}
		rows = append(rows, amberPriceRow{
			PerKwh:      p.PerKwh,
			SpotPerKwh:  p.SpotPerKwh,
			StartTime:   p.StartTime.AsTime(),
			EndTime:     p.EndTime.AsTime(),
			Duration:    p.Duration,
			Forecast:    p.Forecast,
			ChannelType: p.ChannelType,
		})
	}
	if err := s.store.upsertAmberPrices(ctx, rows); err != nil {
		s.logger.Error("IngestAmberPrices: db error", zap.Error(err))
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	s.logger.Info("IngestAmberPrices: ok", zap.Int("count", len(rows)))
	return connect.NewResponse(&winetv1.IngestAmberPricesResponse{}), nil
}

func (s *service) IngestAmberUsage(
	ctx context.Context,
	req *connect.Request[winetv1.IngestAmberUsageRequest],
) (*connect.Response[winetv1.IngestAmberUsageResponse], error) {
	rows := make([]amberUsageRow, 0, len(req.Msg.Usage))
	for _, u := range req.Msg.Usage {
		if u.StartTime == nil || u.EndTime == nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("amber usage missing time fields"))
		}
		rows = append(rows, amberUsageRow{
			PerKwh:            u.PerKwh,
			SpotPerKwh:        u.SpotPerKwh,
			StartTime:         u.StartTime.AsTime(),
			EndTime:           u.EndTime.AsTime(),
			Duration:          u.Duration,
			ChannelType:       u.ChannelType,
			ChannelIdentifier: u.ChannelIdentifier,
			Kwh:               u.Kwh,
			Quality:           u.Quality,
			Cost:              u.Cost,
		})
	}
	if err := s.store.upsertAmberUsage(ctx, rows); err != nil {
		s.logger.Error("IngestAmberUsage: db error", zap.Error(err))
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	s.logger.Info("IngestAmberUsage: ok", zap.Int("count", len(rows)))
	return connect.NewResponse(&winetv1.IngestAmberUsageResponse{}), nil
}

// --- CommandService -----------------------------------------------------------

func (s *service) GetPendingCommands(
	ctx context.Context,
	req *connect.Request[winetv1.GetPendingCommandsRequest],
) (*connect.Response[winetv1.GetPendingCommandsResponse], error) {
	if req.Msg.DeviceId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("device_id is required"))
	}
	dbRows, err := s.store.getPendingCommands(ctx, req.Msg.DeviceId)
	if err != nil {
		s.logger.Error("GetPendingCommands: db error", zap.Error(err))
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	cmds := make([]*winetv1.InverterCommand, 0, len(dbRows))
	for _, row := range dbRows {
		cmd, err := dbRowToCommand(row)
		if err != nil {
			s.logger.Warn("GetPendingCommands: skipping malformed command",
				zap.String("id", row.ID), zap.Error(err))
			continue
		}
		cmds = append(cmds, cmd)
	}

	return connect.NewResponse(&winetv1.GetPendingCommandsResponse{Commands: cmds}), nil
}

func (s *service) AckCommand(
	ctx context.Context,
	req *connect.Request[winetv1.AckCommandRequest],
) (*connect.Response[winetv1.AckCommandResponse], error) {
	if req.Msg.CommandId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("command_id is required"))
	}
	if err := s.store.ackCommand(ctx, req.Msg.CommandId, req.Msg.Success); err != nil {
		s.logger.Error("AckCommand: db error", zap.Error(err))
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	s.logger.Info("AckCommand: ok",
		zap.String("command_id", req.Msg.CommandId),
		zap.Bool("success", req.Msg.Success),
	)
	return connect.NewResponse(&winetv1.AckCommandResponse{}), nil
}

// dbRowToCommand converts a pending_commands row into a proto InverterCommand.
// The command_type field names match the oneof field names in commands.proto.
func dbRowToCommand(row pendingCommandRow) (*winetv1.InverterCommand, error) {
	cmd := &winetv1.InverterCommand{
		Id:        row.ID,
		CreatedAt: timestamppb.New(row.CreatedAt),
	}
	switch row.CommandType {
	case "self_consumption":
		cmd.Command = &winetv1.InverterCommand_SelfConsumption{
			SelfConsumption: &winetv1.SelfConsumptionCommand{},
		}
	case "battery_stop":
		cmd.Command = &winetv1.InverterCommand_BatteryStop{
			BatteryStop: &winetv1.BatteryStopCommand{},
		}
	case "discharge":
		var p struct {
			DischargePower string `json:"discharge_power"`
		}
		if err := json.Unmarshal([]byte(row.Payload), &p); err != nil {
			return nil, fmt.Errorf("discharge payload: %w", err)
		}
		cmd.Command = &winetv1.InverterCommand_Discharge{
			Discharge: &winetv1.DischargeCommand{DischargePower: p.DischargePower},
		}
	case "charge":
		var p struct {
			ChargePower string `json:"charge_power"`
		}
		if err := json.Unmarshal([]byte(row.Payload), &p); err != nil {
			return nil, fmt.Errorf("charge payload: %w", err)
		}
		cmd.Command = &winetv1.InverterCommand_Charge{
			Charge: &winetv1.ChargeCommand{ChargePower: p.ChargePower},
		}
	case "inverter_state_change":
		var p struct {
			Disable bool `json:"disable"`
		}
		if err := json.Unmarshal([]byte(row.Payload), &p); err != nil {
			return nil, fmt.Errorf("inverter_state_change payload: %w", err)
		}
		cmd.Command = &winetv1.InverterCommand_InverterStateChange{
			InverterStateChange: &winetv1.InverterStateChangeCommand{Disable: p.Disable},
		}
	case "set_feed_in_limitation":
		var p struct {
			Limited bool `json:"limited"`
		}
		if err := json.Unmarshal([]byte(row.Payload), &p); err != nil {
			return nil, fmt.Errorf("set_feed_in_limitation payload: %w", err)
		}
		cmd.Command = &winetv1.InverterCommand_SetFeedInLimitation{
			SetFeedInLimitation: &winetv1.SetFeedInLimitationCommand{Limited: p.Limited},
		}
	default:
		return nil, fmt.Errorf("unknown command_type %q", row.CommandType)
	}
	return cmd, nil
}
