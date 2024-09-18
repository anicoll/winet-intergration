package winet

type WebSocketService string

func (wss WebSocketService) String() string {
	return string(wss)
}

const (
	Connect     WebSocketService = "connect"
	Login       WebSocketService = "login"
	DeviceList  WebSocketService = "devicelist"
	Direct      WebSocketService = "direct"
	Local       WebSocketService = "local"
	Notice      WebSocketService = "notice"
	Statistics  WebSocketService = "statistics"
	Real        WebSocketService = "real"         /// time123456 (epoch)
	RealBattery WebSocketService = "real_battery" /// time123456 (epoch)
)
