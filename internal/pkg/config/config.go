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
	AuthCfg          AuthConfig
	LogLevel         string   `env:"LOG_LEVEL"          envDefault:"info"`
	DBDSN            string   `env:"DATABASE_URL,required"`
	MigrationsFolder string   `env:"MIGRATIONS_FOLDER"  envDefault:"migrations"`
	Timezone         string   `env:"TIMEZONE"           envDefault:"Australia/Adelaide"`
	AllowedOrigins   []string `env:"ALLOWED_ORIGIN,required" envSeparator:","`
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

type AuthConfig struct {
	JWTSecret       string        `env:"JWT_SECRET,required"`
	AccessTokenTTL  time.Duration `env:"JWT_ACCESS_TTL"   envDefault:"15m"`
	RefreshTokenTTL time.Duration `env:"JWT_REFRESH_TTL"  envDefault:"720h"`
	SecureCookies   bool          `env:"SECURE_COOKIES"   envDefault:"true"`
}
