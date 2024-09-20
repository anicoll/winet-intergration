package winet

import (
	"encoding/json"

	ws "github.com/anicoll/evtwebsocket"
	"go.uber.org/zap"
)

func (s *service) handleConnectMessage(data []byte, c ws.Connection) {
	res := ParsedResult[ConnectResponse]{}
	err := json.Unmarshal(data, &res)
	s.sendIfErr(err)
	s.token = res.ResultData.Token

	// login now
	data, err = json.Marshal(LoginRequest{
		Request: Request{
			Lang:    EnglishLang,
			Service: Login.String(),
			Token:   s.token,
		},
		Password: s.cfg.Password,
		Username: s.cfg.Username,
	})
	s.sendIfErr(err)

	err = c.Send(ws.Msg{
		Body: data,
	})
	s.logger.Debug("sent msg", zap.String("query_stage", Login.String()))
	s.sendIfErr(err)
}
