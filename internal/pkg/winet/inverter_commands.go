package winet

import (
	"encoding/json"
	"fmt"
	"time"

	ws "github.com/anicoll/evtwebsocket"
	"github.com/anicoll/winet-integration/internal/pkg/model"
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
	s.sendIfErr(err)
	s.conn.Send(ws.Msg{
		Body: data,
	})
	res := s.waiter()

	s.logger.Info("INVERTER", zap.Any("any", res))
	return false, nil
}

func (s *service) handleParamMessage(data []byte, _ ws.Connection) {
	res := model.ParsedResult[model.GenericReponse[model.InverterParamResponse]]{}
	err := json.Unmarshal(data, &res)
	s.sendIfErr(err)
	s.logger.Info(string(data), zap.Any("payload", res))

	s.processed <- res
}
