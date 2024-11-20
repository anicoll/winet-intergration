package model

type QueryStage string

func (qs QueryStage) String() string {
	return string(qs)
}

const (
	Connect     QueryStage = "connect"
	Login       QueryStage = "login"
	DeviceList  QueryStage = "devicelist"
	Direct      QueryStage = "direct"
	Local       QueryStage = "local"
	Notice      QueryStage = "notice"
	Statistics  QueryStage = "statistics"
	Real        QueryStage = "real"         /// time123456 (epoch)
	RealBattery QueryStage = "real_battery" /// time123456 (epoch)
)

type NumericUnit string

const (
	NumericUnitAmp                    NumericUnit = "A"
	NumericUnitPercent                NumericUnit = "%"
	NumericUnitKiloWatt               NumericUnit = "kW"
	NumericUnitWatt                   NumericUnit = "W"
	NumericUnitKiloWattHour           NumericUnit = "kWh"
	NumericUnitDegreeC                NumericUnit = "℃"
	NumericUnitVolt                   NumericUnit = "V"
	NumericUnitKilovoltAmpereReactive NumericUnit = "kvar"
	NumericUnitVoltAmpereReactive     NumericUnit = "var"
	NumericUnitHertz                  NumericUnit = "Hz"
	NumericUnitKiloVoltAmpere         NumericUnit = "kVA"
	NumericUnitKiloOhm                NumericUnit = "kΩ"
)

var NumericUnits = []NumericUnit{
	NumericUnitAmp,
	NumericUnitPercent,
	NumericUnitKiloWatt,
	NumericUnitKiloWattHour,
	NumericUnitDegreeC,
	NumericUnitVolt,
	NumericUnitKilovoltAmpereReactive,
	NumericUnitVoltAmpereReactive,
	NumericUnitHertz,
	NumericUnitKiloVoltAmpere,
	NumericUnitKiloOhm,
}

type DeviceType int

const (
	DeviceTypeInverter DeviceType = 35
	DeviceTypeBattery  DeviceType = 44
)

var DeviceStages = map[DeviceType][]QueryStage{
	DeviceTypeBattery: {
		Real,
	},
	DeviceTypeInverter: {
		Real,
		RealBattery,
		Direct,
	},
}

type (
	TextSensor  string
	TextSensorz []TextSensor
)

const (
	BatteryOperatorTextSensor TextSensor = "battery_operation_status"
	RunningStatusTextSensor   TextSensor = "running_status"
)

func (t TextSensor) String() string {
	return string(t)
}

func (ts TextSensorz) HasSlug(slug string) bool {
	for _, t := range ts {
		if t.String() == slug {
			return true
		}
	}
	return false
}

var TextSensors TextSensorz = TextSensorz{
	BatteryOperatorTextSensor,
	RunningStatusTextSensor,
}
