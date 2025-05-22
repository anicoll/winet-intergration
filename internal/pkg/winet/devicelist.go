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

		s.currentDevice = &model.Device{
			ID:           strconv.Itoa(device.DeviceID),
			Model:        device.DevModel,
			SerialNumber: device.DevSN,
		}
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
			s.sendIfErr(err)

			s.sendIfErr(c.Send(ws.Msg{
				Body: requestData,
			}))
			// s.waiter() // REMOVED: s.processed channel and waiter() are being removed.
						// This loop will now send requests without waiting for the flawed sync point.
		}
	}
	ticker := time.NewTicker(time.Second * s.cfg.PollInterval)
	<-ticker.C
	s.sendDeviceListRequest(c)
}

// func (s *service) waiter() any { // REMOVED: s.processed channel is being removed.
// 	return <-s.processed
// }

// GetDeviceList was the original sole receiver of s.processed.
// It's now modified to reflect that s.processed is removed.
// A proper implementation for GetDeviceList to get data is needed.
func (s *service) GetDeviceList(ctx context.Context) any {
	s.logger.Warn("GetDeviceList called, but s.processed channel is removed. Returning nil. Functionality needs redesign.")
	return nil // Placeholder, as the original mechanism is gone.
}
