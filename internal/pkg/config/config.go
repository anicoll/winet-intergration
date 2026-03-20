package config

import (
	"time"

	"github.com/caarlos0/env/v11"
)

// Config holds all application configuration populated from environment variables.
type Config struct {
	WinetCfg         WinetConfig
	MqttCfg          MQTTConfig
	AmberCfg         AmberConfig
	LogLevel         string `env:"LOG_LEVEL"          envDefault:"info"`
	DBDSN            string `env:"DATABASE_URL,required"`
	MigrationsFolder string `env:"MIGRATIONS_FOLDER"  envDefault:"migrations"`
	Timezone         string `env:"TIMEZONE"           envDefault:"Australia/Adelaide"`
}

// Load parses configuration from environment variables.
// Returns an error if any required variable is missing or malformed.
func Load() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
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
