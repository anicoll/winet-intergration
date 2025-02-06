package config

import "time"

type Config struct {
	WinetCfg *WinetConfig
	MqttCfg  *WinetConfig
	LogLevel string
}

type WinetConfig struct {
	Host         string
	Username     string
	Password     string
	Ssl          bool
	PollInterval time.Duration
}
