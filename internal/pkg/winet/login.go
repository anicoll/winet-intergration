package winet

import (
	"encoding/json"

	"github.com/anicoll/winet-integration/internal/pkg/model"
	ws "github.com/anicoll/winet-integration/pkg/sockets"
	"go.uber.org/zap"
)

func (s *service) sendDeviceListRequest(c ws.Connection) {
	request, err := json.Marshal(model.DeviceListRequest{
		IsCheckToken: "0",
		Request: model.Request{
			Lang:    EnglishLang,
			Service: model.DeviceList.String(),
			Token:   s.token,
		},
		Type: "0",
	})
	if err != nil {
		s.sendIfErr(err)
		return
	}
	if err = c.Send(ws.Msg{Body: request}); err != nil {
		s.sendIfErr(err)
		return
	}
	s.logger.Debug("sent msg", zap.String("query_stage", model.DeviceList.String()))
}

// handleLoginMessage consumes Login response and calls DeviceList.
func (s *service) handleLoginMessage(data []byte, c ws.Connection) {
	loginRes := model.ParsedResult[model.LoginResponse]{}
	if err := json.Unmarshal(data, &loginRes); err != nil {
		s.sendIfErr(err)
		return
	}
	s.token = loginRes.ResultData.Token
	s.processed = make(chan any) // recreate the channel to signal when we are done.
	s.sendDeviceListRequest(c)
}
