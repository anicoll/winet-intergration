package config

type Config struct {
	WinetCfg *WinetConfig
}

type WinetConfig struct {
	HostPort string
	Username string
	Password string
}
