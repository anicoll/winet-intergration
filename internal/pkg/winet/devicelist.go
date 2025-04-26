package winet

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	ws "github.com/anicoll/evtwebsocket"
	"github.com/anicoll/winet-integration/internal/pkg/model"
	"github.com/anicoll/winet-integration/internal/pkg/publisher"
	"go.uber.org/zap"
)

func (s *service) handleDeviceListMessage(data []byte, c ws.Connection) {
	res := model.ParsedResult[model.GenericReponse[model.DeviceListObject]]{}
	err := json.Unmarshal(data, &res)
	s.sendIfErr(err)

	for _, device := range res.ResultData.List {
		if len(model.DeviceStages[device.DevType]) == 0 {
			continue
		}

		s.currentDevice = &model.Device{
			ID:           strconv.Itoa(device.DeviceID),
			Model:        device.DevModel,
			SerialNumber: device.DevSN,
		}
		err = publisher.RegisterDevice(s.currentDevice)
		s.sendIfErr(err)
		s.logger.Debug("detected device", zap.Any("device", device), zap.Error(err))
		for _, qs := range model.DeviceStages[device.DevType] {
			s.logger.Debug("querying for device", zap.Any("device", device))
			requestData, err := json.Marshal(model.RealRequest{
				DeviceID: fmt.Sprintf("%d", device.DeviceID),
				Time:     fmt.Sprintf("%d", time.Now().UnixMilli()),
				Request: model.Request{
					Lang:    EnglishLang,
					Service: qs.String(),
					Token:   s.token,
				},
			})
			s.sendIfErr(err)

			s.sendIfErr(c.Send(ws.Msg{
				Body: requestData,
			}))
			s.waiter()
		}
	}
	ticker := time.NewTicker(time.Second * s.cfg.PollInterval)
	<-ticker.C
	s.sendDeviceListRequest(c)
}

func (s *service) waiter() any {
	return <-s.processed
}
