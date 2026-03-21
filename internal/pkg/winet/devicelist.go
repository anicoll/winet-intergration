package winet

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/anicoll/winet-integration/internal/pkg/model"
	"github.com/anicoll/winet-integration/internal/pkg/publisher"
	ws "github.com/anicoll/winet-integration/pkg/sockets"
	"go.uber.org/zap"
)

func (s *service) handleDeviceListMessage(data []byte, c ws.Connection) {
	s.logger.Debug("handleDeviceListMessage")
	res := model.ParsedResult[model.GenericReponse[model.DeviceListObject]]{}
	err := json.Unmarshal(data, &res)
	s.sendIfErr(err)

	for _, device := range res.ResultData.List {
		if len(model.DeviceStages[device.DevType]) == 0 {
			continue
		}

		s.deviceMu.Lock()
		s.currentDevice = &model.Device{
			ID:           strconv.Itoa(device.DeviceID),
			Model:        device.DevModel,
			SerialNumber: device.DevSN,
		}
		s.deviceMu.Unlock()
		err = publisher.RegisterDevice(s.currentDevice)
		s.sendIfErr(err)
		s.logger.Debug("detected device", zap.Any("device", device), zap.Error(err))
		for _, qs := range model.DeviceStages[device.DevType] {
			s.logger.Debug("querying for device", zap.Any("device", device), zap.String("query_stage", qs.String()), zap.String("token", s.token))
			requestData, err := json.Marshal(model.RealRequest{
				DeviceID: fmt.Sprintf("%d", device.DeviceID),
				Time:     fmt.Sprintf("%d", time.Now().UnixMilli()),
				Request: model.Request{
					Lang:    EnglishLang,
					Service: qs.String(),
					Token:   s.token,
				},
			})
			if err != nil {
				s.sendIfErr(err)
				return
			}
			if err = c.Send(ws.Msg{Body: requestData}); err != nil {
				s.sendIfErr(err)
				return
			}
			if _, err = s.waiter(); err != nil {
				s.sendIfErr(err)
				return
			}
		}
	}
	ticker := time.NewTicker(time.Second * s.cfg.PollInterval)
	defer ticker.Stop()
	<-ticker.C
	s.sendDeviceListRequest(c)
}

