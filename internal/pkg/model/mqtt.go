package model

type RegisterDevice struct {
	Name         string   `json:"name"`
	Identifiers  []string `json:"identifiers"`
	Model        string   `json:"model"`
	Manufacturer string   `json:"manufacturer"`
}

type RegisterMessage struct {
	Tilda      string         `json:"~"`
	Name       string         `json:"name"`
	ID         string         `json:"unique_id"`
	StateTopic string         `json:"state_topic"`
	Device     RegisterDevice `json:"device"`
}

type Device struct {
	ID           string
	Model        string
	SerialNumber string
}

type DeviceStatus struct {
	Name  string  `json:"name"`
	Slug  string  `json:"slug"`
	Value *string `json:"value"`
	Unit  string  `json:"unit"`
	Dirty bool    `json:"dirty"`
}
