package winet

import (
	"encoding/json"
	"strconv"

	"github.com/gosimple/slug"

	"github.com/anicoll/winet-integration/internal/pkg/model"
)

func (s *service) handleDirectMessage(data []byte) {
	res := model.ParsedResult[model.DirectResponse]{}
	err := json.Unmarshal(data, &res)
	s.sendIfErr(err)
	if s.currentDevice == nil {
		return
	}

	datapointsToPublish := make(map[model.Device][]model.DeviceStatus)
	datapoints := []model.DeviceStatus{}
	// mpptTotalW := float32(0)
	for _, data := range res.ResultData.List {
		nameV := data.Name + " Voltage"
		nameA := data.Name + " Current"
		nameW := data.Name + " Power"

		var valueV *string = nil
		if data.Voltage != "--" {
			valueV = &data.Voltage
		}

		dataPointV := model.DeviceStatus{
			Name:  nameV,
			Slug:  slug.Make(nameV),
			Value: valueV,
			Unit:  string(data.VoltageUnit),
			Dirty: true,
		}
		datapoints = append(datapoints, dataPointV)

		var valueA *string = nil
		if data.Current != "--" {
			valueA = &data.Current
		}

		dataPointA := model.DeviceStatus{
			Name:  nameA,
			Slug:  slug.Make(nameA),
			Value: valueA,
			Unit:  string(data.CurrentUnit),
			Dirty: true,
		}
		datapoints = append(datapoints, dataPointA)

		var valueW *string = nil
		if data.Current != "--" {
			current, err := strconv.ParseFloat(data.Current, 32)
			s.sendIfErr(err)
			voltage, err := strconv.ParseFloat(data.Voltage, 32)
			s.sendIfErr(err)
			total := current * voltage * 100
			total = total / 100
			valueW = func() *string {
				s := strconv.FormatFloat(total, 'f', 10, 64)
				return &s
			}()
		}

		dataPointW := model.DeviceStatus{
			Name:  nameW,
			Slug:  slug.Make(nameW),
			Value: valueW,
			Unit:  string(data.CurrentUnit),
			Dirty: true,
		}
		datapoints = append(datapoints, dataPointW)
		// mpptTotalW += dataPointW.Value
	}
	//  dataPointTotalW:= model.DeviceStatus{
	// 	Name: "MPPT Total Power",
	// 	Slug: "mppt_total_power",
	// 	Value: Math.round(mpptTotalW * 100) / 100,
	// 	Unit: string(model.NumericUnitWatt),
	// 	Dirty: true,
	// };

	datapointsToPublish[*s.currentDevice] = datapoints
	s.processed <- struct{}{} // indicate we are done.
}
