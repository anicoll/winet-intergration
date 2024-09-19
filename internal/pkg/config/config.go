package config

type Config struct {
	WinetCfg *WinetConfig
	LogLevel string
}

type WinetConfig struct {
	HostPort string
	Username string
	Password string
}
