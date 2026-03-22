package winet

import (
	"encoding/json"

	"go.uber.org/zap"

	"github.com/anicoll/winet-integration/internal/pkg/model"
	ws "github.com/anicoll/winet-integration/pkg/sockets"
)

func (s *service) handleConnectMessage(data []byte, c ws.Connection) {
	res := model.ParsedResult[model.ConnectResponse]{}
	if err := json.Unmarshal(data, &res); err != nil {
		s.sendIfErr(err)
		return
	}
	s.token = res.ResultData.Token

	// login now
	loginData, err := json.Marshal(model.LoginRequest{
		Request: model.Request{
			Lang:    EnglishLang,
			Service: model.Login.String(),
			Token:   s.token,
		},
		Password: s.cfg.Password,
		Username: s.cfg.Username,
	})
	if err != nil {
		s.sendIfErr(err)
		return
	}

	if err = c.Send(ws.Msg{Body: loginData}); err != nil {
		s.sendIfErr(err)
		return
	}
	s.logger.Debug("sent msg", zap.String("query_stage", model.Login.String()))
}
