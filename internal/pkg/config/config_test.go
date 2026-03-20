package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validConfig() *Config {
	return &Config{
		WinetCfg: &WinetConfig{
			Host:     "192.168.1.1",
			Username: "admin",
			Password: "secret",
		},
		DBDSN: "postgres://localhost/test",
	}
}

func TestValidate_ValidConfig(t *testing.T) {
	require.NoError(t, validConfig().Validate())
}

func TestValidate_MissingWinetCfg(t *testing.T) {
	cfg := validConfig()
	cfg.WinetCfg = nil
	assert.ErrorContains(t, cfg.Validate(), "winet config is required")
}

func TestValidate_MissingHost(t *testing.T) {
	cfg := validConfig()
	cfg.WinetCfg.Host = ""
	assert.ErrorContains(t, cfg.Validate(), "winet host is required")
}

func TestValidate_MissingUsername(t *testing.T) {
	cfg := validConfig()
	cfg.WinetCfg.Username = ""
	assert.ErrorContains(t, cfg.Validate(), "winet username is required")
}

func TestValidate_MissingPassword(t *testing.T) {
	cfg := validConfig()
	cfg.WinetCfg.Password = ""
	assert.ErrorContains(t, cfg.Validate(), "winet password is required")
}

func TestValidate_MissingDBDSN(t *testing.T) {
	cfg := validConfig()
	cfg.DBDSN = ""
	assert.ErrorContains(t, cfg.Validate(), "database DSN is required")
}
