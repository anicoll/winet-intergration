package winet

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
// LocalReponse returns Local Information of System
// LocalReponse to be merged with ParsedResult
type LocalReponse struct {
	Count   int           `json:"count"`
	Service string        `json:"service"`
	List    []GenericUnit `json:"list"`
}

type GenericUnit struct {
	DataName  string `json:"data_name"`
	DataValue string `json:"data_value"`
	DataUnit  string `json:"data_unit"`
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
type DeviceListResponse struct {
	Count   int                `json:"count"`
	Service string             `json:"service"`
	List    []DeviceListObject `json:"list"`
}

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

// RealResponse to be merged with ParsedResult
type RealResponse struct {
	Count   int           `json:"count"`
	Service int           `json:"service"`
	List    []GenericUnit `json:"list"`
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

// ################################

// ################################
// QueryStage.Connect
type ConnectRequest struct {
	Request
}

type ConnectResponse LoginResponse

// ################################
