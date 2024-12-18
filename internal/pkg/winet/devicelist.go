package winet

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	ws "github.com/anicoll/evtwebsocket"
	"github.com/anicoll/winet-integration/internal/pkg/model"
	"go.uber.org/zap"
)

func (s *service) handleDeviceListMessage(data []byte, c ws.Connection) {
	res := model.ParsedResult[model.DeviceListResponse]{}
	err := json.Unmarshal(data, &res)
	s.sendIfErr(err)

	for _, device := range res.ResultData.List {
		if len(model.DeviceStages[device.DevType]) == 0 {
			continue
		}
		// regx:=regexp.MustCompile("[^a-zA-Z0-9]")
		// regx.ReplaceAll()
		s.currentDevice = &model.Device{
			ID:           strconv.Itoa(device.DeviceID),
			Model:        device.DevModel,
			SerialNumber: device.DevSN,
		}
		s.publisher.RegisterDevice(s.currentDevice)
		s.logger.Info("detected device")
		for _, qs := range model.DeviceStages[device.DevType] {
			s.logger.Info("querying for device", zap.Any("device", device))
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
			s.logger.Info("HEREE")
		}
	}
	ticker := time.NewTicker(time.Second * 5)
	<-ticker.C
	s.sendDeviceListRequest(c)
}

func (s *service) waiter() {
	select {
	case <-s.processed:
		return
	}
}
