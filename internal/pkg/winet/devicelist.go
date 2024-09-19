package winet

import (
	"encoding/json"
	"fmt"
	"time"

	ws "github.com/anicoll/evtwebsocket"
	"go.uber.org/zap"
)

func (s *service) handleDeviceListMessage(data []byte, c ws.Connection) {
	res := ParsedResult[DeviceListResponse]{}
	err := json.Unmarshal(data, &res)
	s.sendIfErr(err)
	for _, device := range res.ResultData.List {
		for _, qs := range DeviceStages[device.DevType] {
			s.logger.Debug("querying for device", zap.Any("device", device))
			requestData, err := json.Marshal(RealRequest{
				DeviceID: fmt.Sprintf("%d", device.DeviceID),
				Time:     fmt.Sprintf("%d", time.Now().UnixMilli()),
				Request: Request{
					Lang:    EnglishLang,
					Service: qs.String(),
					Token:   s.token,
				},
			})
			s.sendIfErr(err)
			s.sendIfErr(c.Send(ws.Msg{
				Body: requestData,
			}))
			time.Sleep(time.Second) // sleep for a second to allow processing of events
		}
	}
}
