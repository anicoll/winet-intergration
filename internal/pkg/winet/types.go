package winet

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

type DeviceType int

const (
	DeviceTypeInverter DeviceType = 35
	DeviceTypeBattery  DeviceType = 44
)

var DeviceStages = map[DeviceType][]QueryStage{
	DeviceTypeBattery: []QueryStage{
		Real,
	},
	DeviceTypeInverter: []QueryStage{
		Real,
		RealBattery,
		Direct,
	},
}
