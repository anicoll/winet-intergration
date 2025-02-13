package model

// Use this to know which service to respond to.
type GenericResult struct {
	ResultCode    int    `json:"result_code"`
	ResultMessage string `json:"result_msg"`
	ResultData    struct {
		Service QueryStage `json:"service"`
	} `json:"result_Data"`
}

type ParsedResult[T any] struct {
	ResultCode    int    `json:"result_code"`
	ResultMessage string `json:"result_msg"`
	ResultData    T      `json:"result_Data"`
}

type Request struct {
	Lang    string `json:"lang"`
	Service string `json:"service"`
	Token   string `json:"token"`
}

// ################################
// QueryStage.Local

type GenericReponse[T any] struct {
	Count   int    `json:"count"`
	Service string `json:"service"`
	List    []T    `json:"list"`
}

type GenericUnit struct {
	DataName  string      `json:"data_name"`
	DataValue string      `json:"data_value"`
	DataUnit  NumericUnit `json:"data_unit"`
}

// ################################

// ################################
// QueryStage.DeviceList
type DeviceListRequest struct {
	IsCheckToken string `json:"is_check_token"`
	Type         string `json:"type"`
	Request
}

// DeviceListResponse to be merged with ParsedResult

type DeviceListObject struct {
	ID              int        `json:"id"`
	DeviceID        int        `json:"dev_id"`
	DevCode         int        `json:"dev_code"`
	DevType         DeviceType `json:"dev_type"`
	DevProtocol     int        `json:"dev_protocol"`
	InverterType    int        `json:"inv_type"`
	DevSN           string     `json:"dev_sn"`
	DevName         string     `json:"dev_name"`
	DevModel        string     `json:"dev_model"`
	PortName        string     `json:"port_name"`
	PhysicalAddress string     `json:"phys_addr"`
	LogicalAddress  string     `json:"logc_addr"`
	LinkStatus      int        `json:"link_status"`
	InitStatus      int        `json:"init_status"`
	DevSpecial      string     `json:"dev_special"`
	List            []struct{} `json:"list"`
}

// ################################

// ################################
// QueryStage.Real
type RealRequest struct {
	DeviceID string `json:"dev_id"`
	Time     string `json:"time123456"`
	Request
}

// ################################

// ################################
// QueryStage.Login
type LoginRequest struct {
	Password string `json:"passwd"`
	Username string `json:"username"`
	Request
}

// LoginResponse to be merged with ParsedResult
type LoginResponse struct {
	Service           string `json:"service"`
	Token             string `json:"token"`
	Uid               int    `json:"uid"`
	TipsDisable       int    `json:"tips_disable"`
	VirginFlag        int    `json:"virgin_flag"`
	IsFirstLogin      int    `json:"isFirstLogin"`
	ForceModifyPasswd int    `json:"forceModifyPasswd"`
}

type DirectUnit struct {
	Name        string      `json:"name"`
	Voltage     string      `json:"voltage"`
	VoltageUnit NumericUnit `json:"voltage_unit"`
	Current     string      `json:"current"`
	CurrentUnit NumericUnit `json:"current_unit"`
}

// ################################

// ################################
// QueryStage.Connect
type ConnectRequest struct {
	Request
}

type ConnectResponse LoginResponse

// ################################

// Inverter Energy Management Request

type InverterUpdateRequest struct {
	Request
	Time           string                 `json:"time123456"`
	ParkSerial     string                 `json:"park_serial"` // same as timestamp
	DevCode        int                    `json:"dev_code"`
	DevType        DeviceType             `json:"dev_type"`
	DevIDArray     []string               `json:"devid_array"`
	Type           string                 `json:"type"`
	Count          string                 `json:"count"`
	CurrentPackNum int                    `json:"current_pack_num"`
	PackNumTotal   int                    `json:"pack_num_total"`
	List           []InverterParamRequest `json:"list"`
}

type DisableInverterRequest struct {
	Request
	DevCode    int        `json:"dev_code"`
	DevType    DeviceType `json:"dev_type"`
	DevIDArray []string   `json:"devid_array"`
	Type       string     `json:"type"`
	Count      string     `json:"count"`
	List       []struct {
		PowerSwitch string `json:"power_switch"`
	} `json:"list"`
}

type InverterParamRequest struct {
	Accuracy   int    `json:"accuracy"`
	ParamAddr  int    `json:"param_addr"`
	ParamID    int    `json:"param_id"`
	ParamType  int    `json:"param_type"`
	ParamValue string `json:"param_value"`
	ParamName  string `json:"param_name"`
}

type InverterParamResponse struct {
	Accuracy  int    `json:"result"`
	ParamAddr int    `json:"param_pid"`
	ParamID   int    `json:"param_id"`
	ParamName string `json:"param_name"`
}
