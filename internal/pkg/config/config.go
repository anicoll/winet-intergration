package config

type Config struct {
	WinetCfg *WinetConfig
	MqttCfg  *WinetConfig
	LogLevel string
}

type WinetConfig struct {
	Host     string
	Username string
	Password string
	Ssl      bool
}
