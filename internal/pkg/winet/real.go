package winet

import (
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/gosimple/slug"

	"github.com/anicoll/winet-integration/internal/pkg/contxt"
	"github.com/anicoll/winet-integration/internal/pkg/model"
	"github.com/anicoll/winet-integration/internal/pkg/publisher"
)

func (s *service) handleRealMessage(data []byte) {
	s.logger.Debug("handleRealMessage")
	res := model.ParsedResult[model.GenericReponse[model.GenericUnit]]{}
	err := json.Unmarshal(data, &res)
	s.sendIfErr(err)
	if s.currentDevice == nil {
		return
	}
	datapointsToPublish := make(map[model.Device][]model.DeviceStatus)
	datapoints := []model.DeviceStatus{}
	for _, device := range res.ResultData.List {
		name := device.DataName
		if n, exists := s.properties[device.DataName]; exists {
			name = n
		}
		dataPoint := model.DeviceStatus{
			Name:  name,
			Slug:  strings.Replace(slug.Make(name), "-", "_", -1),
			Unit:  string(device.DataUnit),
			Value: s.calculateValue(device),
			Dirty: true,
		}
		datapoints = append(datapoints, dataPoint)
	}
	datapointsToPublish[*s.currentDevice] = datapoints
	err = publisher.PublishData(contxt.NewContext(time.Second*5), datapointsToPublish)
	s.sendIfErr(err)
	s.processed <- struct{}{} // indicate we are done.
}

func (s *service) calculateValue(device model.GenericUnit) *string {
	if slices.Contains(model.NumericUnits, device.DataUnit) {
		if device.DataValue == "--" {
			return nil
		}
		return &device.DataValue
	}
	if strings.HasPrefix(device.DataValue, "I18N_") {
		v := s.properties[device.DataValue]
		return &v
	}
	return &device.DataValue
}
