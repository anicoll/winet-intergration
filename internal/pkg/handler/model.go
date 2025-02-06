package handler

type (
	BatteryState  string
	InverterState string
)

func (s BatteryState) String() string {
	return string(s)
}

func (s InverterState) String() string {
	return string(s)
}

const (
	SelfConsumptionState BatteryState = "self_consumption"
	ChargeState          BatteryState = "charge"
	DischargeState       BatteryState = "discharge"

	InverterOnState  InverterState = "on"  // turn inverter on.
	InverterOffState InverterState = "off" // turn inverter off.
)

type UpdateBatteryRequest struct {
	State BatteryState `json:"state"` // BatteryState enum.
	Power string       `json:"power"` // like 6.6 for 6.6kw
}

type UpdateInverterRequest struct {
	State       *InverterState `json:"state"`         // State enum.
	LimitFeedIn *bool          `json:"limit_feed_in"` // tells the server to either allow feedin or not.
}
