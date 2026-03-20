package config

import (
	"errors"
	"time"
)

type Config struct {
	WinetCfg         *WinetConfig
	MqttCfg          *MQTTConfig
	AmberCfg         *AmberConfig
	LogLevel         string
	DBDSN            string
	MigrationsFolder string
	Timezone         string
}

// Validate checks that all required fields are present.
func (c *Config) Validate() error {
	if c.WinetCfg == nil {
		return errors.New("winet config is required")
	}
	if c.WinetCfg.Host == "" {
		return errors.New("winet host is required")
	}
	if c.WinetCfg.Username == "" {
		return errors.New("winet username is required")
	}
	if c.WinetCfg.Password == "" {
		return errors.New("winet password is required")
	}
	if c.DBDSN == "" {
		return errors.New("database DSN is required")
	}
	return nil
}

type WinetConfig struct {
	Host         string
	Username     string
	Password     string
	Ssl          bool
	PollInterval time.Duration
}

type MQTTConfig struct {
	Host     string
	Username string
	Password string
}

type AmberConfig struct {
	Host  string
	Token string
}
