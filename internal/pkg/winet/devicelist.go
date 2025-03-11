package winet

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/anicoll/evtwebsocket"
	"github.com/anicoll/winet-integration/internal/pkg/model"
	"go.uber.org/zap"
)

func (s *service) handleDeviceListMessage(data []byte) {
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
		err = s.publisher.RegisterDevice(s.currentDevice)
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

			s.sendMessage(requestData)
			s.waiter()
		}
	}
	ticker := time.NewTicker(time.Second * s.cfg.PollInterval)
	<-ticker.C
	s.sendDeviceListRequest()
}

func (s *service) waiter() any {
	return <-s.processed
}

func (s *service) sendMessage(data []byte) {
	if s.conn == nil {
		s.logger.Error("connection is nil")
		return
	}
	if !s.conn.IsConnected() {
		s.logger.Error("connection not connected, not sending message")
		return
	}

	err := s.conn.Send(evtwebsocket.Msg{
		Body: data,
	})
	s.sendIfErr(err)
}
