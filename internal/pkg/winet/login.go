package winet

import (
	"encoding/json"

	ws "github.com/anicoll/evtwebsocket"
	"go.uber.org/zap"
)

// handleLoginMessage consumes Login response and calls DeviceList.
func (s *service) handleLoginMessage(data []byte, c ws.Connection) {
	loginRes := ParsedResult[LoginResponse]{}
	err := json.Unmarshal(data, &loginRes)
	s.sendIfErr(err)
	s.token = loginRes.ResultData.Token

	request, err := json.Marshal(DeviceListRequest{
		IsCheckToken: "0",
		Request: Request{
			Lang:    EnglishLang,
			Service: DeviceList.String(),
			Token:   s.token,
		},
		Type: "0",
	})
	s.sendIfErr(err)
	err = c.Send(ws.Msg{
		Body: request,
	})

	s.sendIfErr(err)
	s.logger.Debug("sent msg", zap.String("query_stage", DeviceList.String()))
}
