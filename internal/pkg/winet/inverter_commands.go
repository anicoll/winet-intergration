package winet

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/anicoll/winet-integration/internal/pkg/model"
	ws "github.com/anicoll/winet-integration/pkg/sockets"
	"go.uber.org/zap"
)

// handle a message to force self consumption

// handle a missage to force discharge

// handle a message to force a charge at power.

func (s *service) SendSelfConsumptionCommand() (bool, error) {
	nowTime := fmt.Sprintf("%d", time.Now().UnixMilli())
	data, err := json.Marshal(model.InverterUpdateRequest{
		Request: model.Request{
			Lang:    EnglishLang,
			Service: model.Param.String(),
			Token:   s.token,
		},
		Time:           nowTime,
		ParkSerial:     nowTime,
		DevCode:        3344,
		DevType:        model.DeviceTypeInverter,
		DevIDArray:     []string{"1"},
		Type:           "9",
		Count:          "1",
		CurrentPackNum: 1,
		PackNumTotal:   1,
		List: []model.InverterParamRequest{
			{
				Accuracy:   0,
				ParamAddr:  33146,
				ParamID:    1,
				ParamType:  1,
				ParamValue: "0",
				ParamName:  "Energy Management Mode",
			},
		},
	})
	if err != nil {
		return false, err
	}
	if s.conn == nil {
		return false, fmt.Errorf("connection is nil, cannot send command")
	}
	if err = s.conn.Send(ws.Msg{Body: data}); err != nil {
		return false, err
	}
	res, err := s.pending.wait(s.ctx)
	if err != nil {
		return false, err
	}
	result, ok := res.(model.ParsedResult[model.GenericReponse[model.InverterParamResponse]])
	if !ok {
		return false, fmt.Errorf("unexpected response type: %T", res)
	}
	s.logger.Info("SendSelfConsumptionCommand", zap.Any("any", result))
	return result.ResultMessage == "success", nil
}

func (s *service) SendDischargeCommand(dischargePower string) (bool, error) {
	nowTime := fmt.Sprintf("%d", time.Now().UnixMilli())
	data, err := json.Marshal(model.InverterUpdateRequest{
		Request: model.Request{
			Lang:    EnglishLang,
			Service: model.Param.String(),
			Token:   s.token,
		},
		Time:           nowTime,
		ParkSerial:     nowTime,
		DevCode:        3344,
		DevType:        model.DeviceTypeInverter,
		DevIDArray:     []string{"1"},
		Type:           "9",
		Count:          "1",
		CurrentPackNum: 1,
		PackNumTotal:   1,
		List: []model.InverterParamRequest{
			{
				Accuracy:   0,
				ParamAddr:  33146,
				ParamID:    1,
				ParamType:  1,
				ParamValue: "2",
				ParamName:  "Energy Management Mode",
			},
			{
				Accuracy:   0,
				ParamAddr:  33147,
				ParamID:    2,
				ParamName:  "Charging/Discharging Command",
				ParamType:  1,
				ParamValue: "187",
			},
			{
				Accuracy:   2,
				ParamAddr:  33148,
				ParamID:    3,
				ParamName:  "Charging/Discharging Power",
				ParamType:  2,
				ParamValue: dischargePower,
			},
		},
	})
	if err != nil {
		return false, err
	}
	if s.conn == nil {
		return false, fmt.Errorf("connection is nil, cannot send command")
	}
	if err = s.conn.Send(ws.Msg{Body: data}); err != nil {
		return false, err
	}
	res, err := s.pending.wait(s.ctx)
	if err != nil {
		return false, err
	}
	result, ok := res.(model.ParsedResult[model.GenericReponse[model.InverterParamResponse]])
	if !ok {
		return false, fmt.Errorf("unexpected response type: %T", res)
	}
	s.logger.Info("SendDischargeCommand", zap.Any("any", result))
	return result.ResultMessage == "success", nil
}

func (s *service) SendChargeCommand(chargePower string) (bool, error) {
	nowTime := fmt.Sprintf("%d", time.Now().UnixMilli())
	data, err := json.Marshal(model.InverterUpdateRequest{
		Request: model.Request{
			Lang:    EnglishLang,
			Service: model.Param.String(),
			Token:   s.token,
		},
		Time:           nowTime,
		ParkSerial:     nowTime,
		DevCode:        3344,
		DevType:        model.DeviceTypeInverter,
		DevIDArray:     []string{"1"},
		Type:           "9",
		Count:          "1",
		CurrentPackNum: 1,
		PackNumTotal:   1,
		List: []model.InverterParamRequest{
			{
				Accuracy:   0,
				ParamAddr:  33146,
				ParamID:    1,
				ParamName:  "Energy Management Mode",
				ParamType:  1,
				ParamValue: "2",
			},
			{
				Accuracy:   0,
				ParamAddr:  33147,
				ParamID:    2,
				ParamName:  "Charging/Discharging Command",
				ParamType:  1,
				ParamValue: "170",
			},
			{
				Accuracy:   2,
				ParamAddr:  33148,
				ParamID:    3,
				ParamName:  "Charging/Discharging Power",
				ParamType:  2,
				ParamValue: chargePower,
			},
		},
	})
	if err != nil {
		return false, err
	}
	if s.conn == nil {
		return false, fmt.Errorf("connection is nil, cannot send command")
	}
	if err = s.conn.Send(ws.Msg{Body: data}); err != nil {
		return false, err
	}
	res, err := s.pending.wait(s.ctx)
	if err != nil {
		return false, err
	}
	result, ok := res.(model.ParsedResult[model.GenericReponse[model.InverterParamResponse]])
	if !ok {
		return false, fmt.Errorf("unexpected response type: %T", res)
	}
	s.logger.Info("SendChargeCommand", zap.Any("any", result))
	return result.ResultMessage == "success", nil
}

func (s *service) SendBatteryStopCommand() (bool, error) {
	nowTime := fmt.Sprintf("%d", time.Now().UnixMilli())
	data, err := json.Marshal(model.InverterUpdateRequest{
		Request: model.Request{
			Lang:    EnglishLang,
			Service: model.Param.String(),
			Token:   s.token,
		},
		Time:           nowTime,
		ParkSerial:     nowTime,
		DevCode:        3344,
		DevType:        model.DeviceTypeInverter,
		DevIDArray:     []string{"1"},
		Type:           "9",
		Count:          "1",
		CurrentPackNum: 1,
		PackNumTotal:   1,
		List: []model.InverterParamRequest{
			{
				Accuracy:   0,
				ParamAddr:  33146,
				ParamID:    1,
				ParamName:  "Energy Management Mode",
				ParamType:  1,
				ParamValue: "2",
			},
			{
				Accuracy:   0,
				ParamAddr:  33147,
				ParamID:    2,
				ParamName:  "Charging/Discharging Command",
				ParamType:  1,
				ParamValue: "204",
			},
		},
	})
	if err != nil {
		return false, err
	}
	if s.conn == nil {
		return false, fmt.Errorf("connection is nil, cannot send command")
	}
	if err = s.conn.Send(ws.Msg{Body: data}); err != nil {
		return false, err
	}
	res, err := s.pending.wait(s.ctx)
	if err != nil {
		return false, err
	}
	result, ok := res.(model.ParsedResult[model.GenericReponse[model.InverterParamResponse]])
	if !ok {
		return false, fmt.Errorf("unexpected response type: %T", res)
	}
	s.logger.Info("SendBatteryStopCommand", zap.Any("any", result))
	return result.ResultMessage == "success", nil
}

func (s *service) SendInverterStateChangeCommand(disable bool) (bool, error) {
	data, err := json.Marshal(model.DisableInverterRequest{
		Request: model.Request{
			Lang:    EnglishLang,
			Service: model.Param.String(),
			Token:   s.token,
		},
		DevCode:    3344,
		DevType:    model.DeviceTypeInverter,
		DevIDArray: []string{"1"},
		Type:       "3",
		Count:      "1",
		List: []struct {
			PowerSwitch string "json:\"power_switch\""
		}{
			{
				PowerSwitch: func(d bool) string {
					if d {
						return "0"
					}
					return "1"
				}(disable),
			},
		},
	})
	if err != nil {
		return false, err
	}
	if s.conn == nil {
		return false, fmt.Errorf("connection is nil, cannot send command")
	}
	if err = s.conn.Send(ws.Msg{Body: data}); err != nil {
		return false, err
	}
	res, err := s.pending.wait(s.ctx)
	if err != nil {
		return false, err
	}
	result, ok := res.(model.ParsedResult[model.GenericReponse[model.InverterParamResponse]])
	if !ok {
		return false, fmt.Errorf("unexpected response type: %T", res)
	}
	s.logger.Info("SendInverterStateChangeCommand", zap.Any("any", result))
	return result.ResultMessage == "success", nil
}

func (s *service) SetFeedInLimitation(feedinLimited bool) (bool, error) {
	paramRequests := []model.InverterParamRequest{{
		Accuracy:   0,
		ParamAddr:  31221,
		ParamID:    13,
		ParamType:  1,
		ParamValue: "170",
		ParamName:  "Feed-in Limitation",
	}, {
		Accuracy:   2,
		ParamAddr:  31222,
		ParamID:    14,
		ParamName:  "Feed-in Limitation Value",
		ParamType:  2,
		ParamValue: "0.00",
	}}

	if !feedinLimited {
		paramRequests = []model.InverterParamRequest{
			{
				Accuracy:   0,
				ParamAddr:  31221,
				ParamID:    13,
				ParamName:  "Feed-in Limitation",
				ParamType:  1,
				ParamValue: "85",
			},
		}
	}
	nowTime := fmt.Sprintf("%d", time.Now().UnixMilli())
	data, err := json.Marshal(model.InverterUpdateRequest{
		Request: model.Request{
			Lang:    EnglishLang,
			Service: model.Param.String(),
			Token:   s.token,
		},
		Time:           nowTime,
		ParkSerial:     nowTime,
		DevCode:        3344,
		DevType:        model.DeviceTypeInverter,
		DevIDArray:     []string{"1"},
		Type:           "7",
		Count:          "1",
		CurrentPackNum: 1,
		PackNumTotal:   1,
		List:           paramRequests,
	})
	if err != nil {
		return false, err
	}
	if s.conn == nil {
		return false, fmt.Errorf("connection is nil, cannot send command")
	}
	if err = s.conn.Send(ws.Msg{Body: data}); err != nil {
		return false, err
	}
	res, err := s.pending.wait(s.ctx)
	if err != nil {
		return false, err
	}
	result, ok := res.(model.ParsedResult[model.GenericReponse[model.InverterParamResponse]])
	if !ok {
		return false, fmt.Errorf("unexpected response type: %T", res)
	}
	s.logger.Info("SetFeedInLimitation", zap.Any("any", result))
	return result.ResultMessage == "success", nil
}

func (s *service) handleParamMessage(data []byte, _ ws.Connection) {
	res := model.ParsedResult[model.GenericReponse[model.InverterParamResponse]]{}
	if err := json.Unmarshal(data, &res); err != nil {
		s.sendIfErr(err)
		return
	}
	s.logger.Info("param_message", zap.Any("payload", res))

	s.pending.deliver(res)
}
