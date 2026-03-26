package config

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// unsetenv ensures a key is absent during the test and restored afterward.
func unsetenv(t *testing.T, key string) {
	t.Helper()
	old, wasSet := os.LookupEnv(key)
	require.NoError(t, os.Unsetenv(key))
	t.Cleanup(func() {
		if wasSet {
			require.NoError(t, os.Setenv(key, old))
		} else {
			require.NoError(t, os.Unsetenv(key))
		}
	})
}

func setRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("WINET_HOST", "192.168.1.1")
	t.Setenv("WINET_USERNAME", "admin")
	t.Setenv("WINET_PASSWORD", "secret")
	t.Setenv("FUNCTION_INGESTION_URL", "https://example.azurewebsites.net")
	t.Setenv("FUNCTION_API_KEY", "test-api-key")
}

func TestLoad_Success(t *testing.T) {
	setRequiredEnv(t)

	cfg, err := Load()
	require.NoError(t, err)

	assert.Equal(t, "192.168.1.1", cfg.WinetCfg.Host)
	assert.Equal(t, "admin", cfg.WinetCfg.Username)
	assert.Equal(t, "secret", cfg.WinetCfg.Password)
	assert.Equal(t, "https://example.azurewebsites.net", cfg.FunctionCfg.IngestionURL)
	assert.Equal(t, "test-api-key", cfg.FunctionCfg.APIKey)
}

func TestLoad_Defaults(t *testing.T) {
	setRequiredEnv(t)

	cfg, err := Load()
	require.NoError(t, err)

	assert.Equal(t, "info", cfg.LogLevel)
	assert.Equal(t, "Australia/Adelaide", cfg.Timezone)
	assert.Equal(t, 30*time.Second, cfg.WinetCfg.PollInterval)
	assert.False(t, cfg.WinetCfg.Ssl)
}

func TestLoad_MissingWinetHost(t *testing.T) {
	setRequiredEnv(t)
	unsetenv(t, "WINET_HOST")

	_, err := Load()
	assert.Error(t, err)
}

func TestLoad_MissingWinetUsername(t *testing.T) {
	setRequiredEnv(t)
	unsetenv(t, "WINET_USERNAME")

	_, err := Load()
	assert.Error(t, err)
}

func TestLoad_MissingWinetPassword(t *testing.T) {
	setRequiredEnv(t)
	unsetenv(t, "WINET_PASSWORD")

	_, err := Load()
	assert.Error(t, err)
}

func TestLoad_MissingFunctionIngestionURL(t *testing.T) {
	setRequiredEnv(t)
	unsetenv(t, "FUNCTION_INGESTION_URL")

	_, err := Load()
	assert.Error(t, err)
}

func TestLoad_MissingFunctionAPIKey(t *testing.T) {
	setRequiredEnv(t)
	unsetenv(t, "FUNCTION_API_KEY")

	_, err := Load()
	assert.Error(t, err)
}

func TestLoad_CustomValues(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("TIMEZONE", "America/New_York")
	t.Setenv("WINET_POLL_INTERVAL", "60s")
	t.Setenv("WINET_SSL", "true")
	t.Setenv("MQTT_HOST", "mqtt://broker:1883")
	t.Setenv("MQTT_USERNAME", "mqttuser")
	t.Setenv("MQTT_PASSWORD", "mqttpass")
	t.Setenv("AMBER_HOST", "https://api.amber.com")
	t.Setenv("AMBER_TOKEN", "tok123")

	cfg, err := Load()
	require.NoError(t, err)

	assert.Equal(t, "debug", cfg.LogLevel)
	assert.Equal(t, "America/New_York", cfg.Timezone)
	assert.Equal(t, 60*time.Second, cfg.WinetCfg.PollInterval)
	assert.True(t, cfg.WinetCfg.Ssl)
	assert.Equal(t, "mqtt://broker:1883", cfg.MqttCfg.Host)
	assert.Equal(t, "mqttuser", cfg.MqttCfg.Username)
	assert.Equal(t, "mqttpass", cfg.MqttCfg.Password)
	assert.Equal(t, "https://api.amber.com", cfg.AmberCfg.Host)
	assert.Equal(t, "tok123", cfg.AmberCfg.Token)
}
