package winet

import (
	"encoding/json"

	"github.com/anicoll/winet-integration/internal/pkg/model"
	ws "github.com/anicoll/winet-integration/pkg/sockets"
)

// handleDeviceListMessage parses the device list response and delivers it to
// the poll loop via pending. Device iteration, querying, and polling scheduling
// are all owned by runPollLoop — not this handler.
func (s *service) handleDeviceListMessage(data []byte, _ ws.Connection) {
	s.logger.Debug("handleDeviceListMessage")
	res := model.ParsedResult[model.GenericReponse[model.DeviceListObject]]{}
	if err := json.Unmarshal(data, &res); err != nil {
		s.sendIfErr(err)
		return
	}
	s.pending.deliver(res.ResultData.List)
}
