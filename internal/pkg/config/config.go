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
	OracleCfg        OracleConfig
	LogLevel         string   `env:"LOG_LEVEL"          envDefault:"info"`
	DBDriver         string   `env:"DB_DRIVER"          envDefault:"postgres"`
	DBDSN            string   `env:"DATABASE_URL"`
	MigrationsFolder string   `env:"MIGRATIONS_FOLDER"  envDefault:""`
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

// OracleConfig holds Oracle connection parameters for SSL connections.
// Used when DB_DRIVER=oracle.
type OracleConfig struct {
	Host       string `env:"ORACLE_HOST,required"`
	Port       int    `env:"ORACLE_PORT"        envDefault:"1522"`
	Service    string `env:"ORACLE_SERVICE,required"`
	User       string `env:"ORACLE_USER,required"`
	Password   string `env:"ORACLE_PASSWORD,required"`
	SSLVerify  bool   `env:"ORACLE_SSL_VERIFY"  envDefault:"true"`
	WalletPath string `env:"ORACLE_WALLET"`
}
