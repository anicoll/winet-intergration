package config

import (
	"errors"
	"time"
)

	"github.com/caarlos0/env/v11"
)

// Config holds all application configuration populated from environment variables.
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
	Host         string        `env:"WINET_HOST,required"`
	Username     string        `env:"WINET_USERNAME,required"`
	Password     string        `env:"WINET_PASSWORD,required"`
	Ssl          bool          `env:"WINET_SSL"`
	PollInterval time.Duration `env:"WINET_POLL_INTERVAL" envDefault:"30s"`
}

type MQTTConfig struct {
	Host     string `env:"MQTT_HOST"`
	Username string `env:"MQTT_USERNAME"`
	Password string `env:"MQTT_PASSWORD"`
}

type AmberConfig struct {
	Host  string `env:"AMBER_HOST"`
	Token string `env:"AMBER_TOKEN"`
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
