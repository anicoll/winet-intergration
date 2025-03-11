package winet

import (
	"encoding/json"

	"github.com/anicoll/winet-integration/internal/pkg/model"
	"go.uber.org/zap"
)

func (s *service) sendDeviceListRequest() {
	request, err := json.Marshal(model.DeviceListRequest{
		IsCheckToken: "0",
		Request: model.Request{
			Lang:    EnglishLang,
			Service: model.DeviceList.String(),
			Token:   s.token,
		},
		Type: "0",
	})
	s.sendIfErr(err)
	s.sendMessage(request)
	s.logger.Debug("sent msg", zap.String("query_stage", model.DeviceList.String()))
}

// handleLoginMessage consumes Login response and calls DeviceList.
func (s *service) handleLoginMessage(data []byte) {
	loginRes := model.ParsedResult[model.LoginResponse]{}
	err := json.Unmarshal(data, &loginRes)
	s.sendIfErr(err)
	s.token = loginRes.ResultData.Token
	s.sendDeviceListRequest()
}
