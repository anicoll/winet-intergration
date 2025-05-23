package winet

import (
	"encoding/json"

	"github.com/anicoll/winet-integration/internal/pkg/model"
	ws "github.com/anicoll/winet-integration/pkg/sockets"
	"go.uber.org/zap"
)

func (s *service) handleConnectMessage(data []byte, c ws.Connection) {
	res := model.ParsedResult[model.ConnectResponse]{}
	err := json.Unmarshal(data, &res)
	s.sendIfErr(err)
	s.token = res.ResultData.Token

	// login now
	data, err = json.Marshal(model.LoginRequest{
		Request: model.Request{
			Lang:    EnglishLang,
			Service: model.Login.String(),
			Token:   s.token,
		},
		Password: s.cfg.Password,
		Username: s.cfg.Username,
	})
	s.sendIfErr(err)

	err = c.Send(ws.Msg{
		Body: data,
	})
	s.sendIfErr(err)
	s.logger.Debug("sent msg", zap.String("query_stage", model.Login.String()), zap.Error(err))
}
