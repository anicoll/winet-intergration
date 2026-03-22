package winet

import (
	"encoding/json"

	"go.uber.org/zap"

	"github.com/anicoll/winet-integration/internal/pkg/model"
	ws "github.com/anicoll/winet-integration/pkg/sockets"
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

// handleLoginMessage saves the session token and signals the poll loop that
// login is complete. The poll loop sends the first device list request.
func (s *service) handleLoginMessage(data []byte, _ ws.Connection) {
	loginRes := model.ParsedResult[model.LoginResponse]{}
	if err := json.Unmarshal(data, &loginRes); err != nil {
		s.sendIfErr(err)
		return
	}
	s.token = loginRes.ResultData.Token
	close(s.loginReady) // unblock runPollLoop
}
