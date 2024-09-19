package winet

// Use this to know which service to respond to.
type GenericResult struct {
	ResultCode    int    `json:"result_code"`
	ResultMessage string `json:"result_msg"`
	ResultData    struct {
		Service WebSocketService `json:"service"`
	} `json:"result_Data"`
}

type ParsedResult[T any] struct {
	ResultCode    int    `json:"result_code"`
	ResultMessage string `json:"result_msg"`
	ResultData    T      `json:"result_Data"`
}

// ################################
// WebSocketService.Local
// LocalMessage returns Local Information of System
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
// WebSocketService.DeviceList
type DeviceListRequest struct {
	IsCheckToken string `json:"is_check_token"`
	Lang         string `json:"lang"`
	Service      string `json:"service"`
	Token        string `json:"token"`
	Type         string `json:"type"`
}

type DeviceListResponse struct {
	Count   int                `json:"count"`
	Service string             `json:"service"`
	List    []DeviceListObject `json:"list"`
}

type DeviceListObject struct {
	ID              int        `json:"id"`
	DevID           int        `json:"dev_id"`
	DevCode         int        `json:"dev_code"`
	DevType         int        `json:"dev_type"`
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
// WebSocketService.Real
type RealRequest struct {
	DeviceID string `json:"dev_id"`
	Lang     string `json:"lang"`
	Service  string `json:"service"`
	Token    string `json:"token"`
	Time     string `json:"time123456"`
}

type RealResponse struct {
	Count   int           `json:"count"`
	Service int           `json:"service"`
	List    []GenericUnit `json:"list"`
}

// ################################

// ################################
// WebSocketService.Login
type LoginRequest struct {
	Password string `json:"passwd"`
	Lang     string `json:"lang"`
	Service  string `json:"service"`
	Token    string `json:"token"`
	Username string `json:"username"`
}

type LoginResponse struct {
	ResultCode    int    `json:"result_code"`
	ResultMessage string `json:"result_msg"`
	ResultData    struct {
		Service           string `json:"service"`
		Token             string `json:"token"`
		Uid               int    `json:"uid"`
		TipsDisable       int    `json:"tips_disable"`
		VirginFlag        int    `json:"virgin_flag"`
		IsFirstLogin      int    `json:"isFirstLogin"`
		ForceModifyPasswd int    `json:"forceModifyPasswd"`
	} `json:"result_data"`
}

type LoginObject struct {
	Service           string `json:"service"`
	Token             string `json:"token"`
	Uid               int    `json:"uid"`
	Role              int    `json:"role"`
	TipsDisable       int    `json:"tips_disable"`
	VirginFlag        int    `json:"virgin_flag"`
	IsFirstLogin      int    `json:"isFirstLogin"`
	ForceModifyPasswd int    `json:"forceModifyPasswd"`
}

// ################################

// ################################
// WebSocketService.Connect
type ConnectRequest struct {
	Lang    string `json:"lang"`
	Service string `json:"service"`
	Token   string `json:"token"`
}

type ConnectResponse LoginResponse

// ################################
