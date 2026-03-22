package winet

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/gosimple/slug"

	"github.com/anicoll/winet-integration/internal/pkg/contxt"
	"github.com/anicoll/winet-integration/internal/pkg/model"
)

func (s *service) handleDirectMessage(data []byte) {
	s.logger.Debug("handleDirectMessage")
	res := model.ParsedResult[model.GenericReponse[model.DirectUnit]]{}
	if err := json.Unmarshal(data, &res); err != nil {
		s.sendIfErr(err)
		return
	}
	s.deviceMu.RLock()
	currentDevice := s.currentDevice
	s.deviceMu.RUnlock()
	if currentDevice == nil {
		return
	}

	datapointsToPublish := make(map[model.Device][]model.DeviceStatus)
	datapoints := []model.DeviceStatus{}
	for _, unit := range res.ResultData.List {
		nameV := unit.Name + " Voltage"
		nameA := unit.Name + " Current"
		nameW := unit.Name + " Power"

		var valueV *string
		if unit.Voltage != "--" {
			valueV = &unit.Voltage
		}
		datapoints = append(datapoints, model.DeviceStatus{
			Name:  nameV,
			Slug:  strings.ReplaceAll(slug.Make(nameV), "-", "_"),
			Value: valueV,
			Unit:  string(unit.VoltageUnit),
			Dirty: true,
		})

		var valueA *string
		if unit.Current != "--" {
			valueA = &unit.Current
		}
		datapoints = append(datapoints, model.DeviceStatus{
			Name:  nameA,
			Slug:  strings.ReplaceAll(slug.Make(nameA), "-", "_"),
			Value: valueA,
			Unit:  string(unit.CurrentUnit),
			Dirty: true,
		})

		// Compute power (W) only when both voltage and current are valid.
		var valueW *string
		if unit.Current != "--" && unit.Voltage != "--" {
			current, err := strconv.ParseFloat(unit.Current, 64)
			if err != nil {
				s.sendIfErr(err)
				return
			}
			voltage, err := strconv.ParseFloat(unit.Voltage, 64)
			if err != nil {
				s.sendIfErr(err)
				return
			}
			w := strconv.FormatFloat(current*voltage, 'f', 4, 64)
			valueW = &w
		}
		datapoints = append(datapoints, model.DeviceStatus{
			Name:  nameW,
			Slug:  strings.ReplaceAll(slug.Make(nameW), "-", "_"),
			Value: valueW,
			Unit:  "W", // was incorrectly using CurrentUnit before
			Dirty: true,
		})
	}

	datapointsToPublish[*currentDevice] = datapoints
	if err := s.publisher.PublishData(contxt.NewContext(time.Second*5), datapointsToPublish); err != nil {
		s.sendIfErr(err)
		// still signal processed so waiter unblocks — the publish error is non-fatal
	}
	s.pending.deliver(struct{}{}) // unblock the poll loop
}
