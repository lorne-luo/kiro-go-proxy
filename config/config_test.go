// Package config provides tests for configuration management.
package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

// =============================================================================
// TestConfigDefaults
// Tests for default configuration values
// =============================================================================

func TestConfigDefaults(t *testing.T) {
	// Save current env and restore after test
	oldEnv := map[string]string{}
	envKeys := []string{
		"SERVER_HOST", "SERVER_PORT", "PROXY_API_KEY", "KIRO_REGION",
		"TOKEN_REFRESH_THRESHOLD", "MAX_RETRIES", "MODEL_CACHE_TTL",
		"FIRST_TOKEN_TIMEOUT", "FAKE_REASONING",
	}
	for _, key := range envKeys {
		oldEnv[key] = os.Getenv(key)
		os.Unsetenv(key)
	}
	defer func() {
		for key, val := range oldEnv {
			if val == "" {
				os.Unsetenv(key)
			} else {
				os.Setenv(key, val)
			}
		}
	}()

	// Reset global config
	globalConfig = nil
	cfg := Load()

	t.Run("default server settings", func(t *testing.T) {
		assert.Equal(t, "0.0.0.0", cfg.ServerHost)
		assert.Equal(t, 8000, cfg.ServerPort)
	})

	t.Run("default region", func(t *testing.T) {
		assert.Equal(t, "us-east-1", cfg.Region)
	})

	t.Run("default timeout settings", func(t *testing.T) {
		assert.Equal(t, 600, cfg.TokenRefreshThreshold)
		assert.Equal(t, 3, cfg.MaxRetries)
		assert.Equal(t, 3600, cfg.ModelCacheTTL)
		assert.Equal(t, 15.0, cfg.FirstTokenTimeout)
	})

	t.Run("default fake reasoning settings", func(t *testing.T) {
		assert.True(t, cfg.FakeReasoningEnabled)
		assert.Equal(t, 4000, cfg.FakeReasoningMaxTokens)
		assert.Equal(t, "as_reasoning_content", cfg.FakeReasoningHandling)
		assert.Equal(t, 20, cfg.FakeReasoningBufferSize)
	})

	t.Run("default hidden models", func(t *testing.T) {
		assert.Contains(t, cfg.HiddenModels, "claude-3.7-sonnet")
	})

	t.Run("default model aliases", func(t *testing.T) {
		assert.Contains(t, cfg.ModelAliases, "auto-kiro")
		assert.Equal(t, "auto", cfg.ModelAliases["auto-kiro"])
	})
}

// =============================================================================
// TestConfigEnvOverride
// Tests for environment variable overrides
// =============================================================================

func TestConfigEnvOverride(t *testing.T) {
	// Save and restore env
	oldHost := os.Getenv("SERVER_HOST")
	oldPort := os.Getenv("SERVER_PORT")
	oldRegion := os.Getenv("KIRO_REGION")
	defer func() {
		if oldHost == "" {
			os.Unsetenv("SERVER_HOST")
		} else {
			os.Setenv("SERVER_HOST", oldHost)
		}
		if oldPort == "" {
			os.Unsetenv("SERVER_PORT")
		} else {
			os.Setenv("SERVER_PORT", oldPort)
		}
		if oldRegion == "" {
			os.Unsetenv("KIRO_REGION")
		} else {
			os.Setenv("KIRO_REGION", oldRegion)
		}
	}()

	t.Run("overrides server host from env", func(t *testing.T) {
		os.Setenv("SERVER_HOST", "127.0.0.1")
		globalConfig = nil
		cfg := Load()
		assert.Equal(t, "127.0.0.1", cfg.ServerHost)
	})

	t.Run("overrides server port from env", func(t *testing.T) {
		os.Setenv("SERVER_PORT", "3000")
		globalConfig = nil
		cfg := Load()
		assert.Equal(t, 3000, cfg.ServerPort)
	})

	t.Run("overrides region from env", func(t *testing.T) {
		os.Setenv("KIRO_REGION", "eu-west-1")
		globalConfig = nil
		cfg := Load()
		assert.Equal(t, "eu-west-1", cfg.Region)
	})
}

// =============================================================================
// TestURLGeneration
// Tests for URL template methods
// =============================================================================

func TestURLGeneration(t *testing.T) {
	cfg := &Config{Region: "us-west-2"}

	t.Run("generates kiro refresh URL", func(t *testing.T) {
		url := cfg.GetKiroRefreshURL()
		assert.Contains(t, url, "us-west-2")
		assert.Contains(t, url, "auth.desktop.kiro.dev")
	})

	t.Run("generates AWS SSO OIDC URL", func(t *testing.T) {
		url := cfg.GetAWSSSOOIDCURL()
		assert.Contains(t, url, "us-west-2")
		assert.Contains(t, url, "oidc")
		assert.Contains(t, url, "amazonaws.com")
	})

	t.Run("generates Kiro API host", func(t *testing.T) {
		host := cfg.GetKiroAPIHost()
		assert.Contains(t, host, "us-west-2")
		assert.Contains(t, host, "q.")
		assert.Contains(t, host, "amazonaws.com")
	})

	t.Run("generates Kiro Q host", func(t *testing.T) {
		host := cfg.GetKiroQHost()
		assert.Contains(t, host, "us-west-2")
		assert.Contains(t, host, "amazonaws.com")
	})
}

func TestURLGenerationForRegion(t *testing.T) {
	t.Run("generates refresh URL for region", func(t *testing.T) {
		url := GetKiroRefreshURLForRegion("ap-southeast-1")
		assert.Contains(t, url, "ap-southeast-1")
		assert.Contains(t, url, "auth.desktop.kiro.dev")
	})

	t.Run("generates OIDC URL for region", func(t *testing.T) {
		url := GetAWSSSOOIDCURLForRegion("eu-central-1")
		assert.Contains(t, url, "eu-central-1")
		assert.Contains(t, url, "oidc")
	})

	t.Run("generates API host for region", func(t *testing.T) {
		host := GetKiroAPIHostForRegion("us-east-1")
		assert.Contains(t, host, "us-east-1")
		assert.Contains(t, host, "amazonaws.com")
	})
}

// =============================================================================
// TestConfigValidate
// Tests for configuration validation
// =============================================================================

func TestConfigValidate(t *testing.T) {
	t.Run("fails without credentials", func(t *testing.T) {
		cfg := &Config{
			RefreshToken:  "",
			KiroCredsFile: "",
			KiroCLIDBFile: "",
		}
		err := cfg.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no Kiro credentials")
	})

	t.Run("passes with refresh token", func(t *testing.T) {
		cfg := &Config{
			RefreshToken:  "test-token",
			KiroCredsFile: "",
			KiroCLIDBFile: "",
		}
		err := cfg.Validate()
		assert.NoError(t, err)
	})

	t.Run("passes with creds file", func(t *testing.T) {
		cfg := &Config{
			RefreshToken:  "",
			KiroCredsFile: "/path/to/creds.json",
			KiroCLIDBFile: "",
		}
		err := cfg.Validate()
		assert.NoError(t, err)
	})

	t.Run("passes with CLI DB file", func(t *testing.T) {
		cfg := &Config{
			RefreshToken:  "",
			KiroCredsFile: "",
			KiroCLIDBFile: "/path/to/kiro.db",
		}
		err := cfg.Validate()
		assert.NoError(t, err)
	})
}

// =============================================================================
// TestEnvHelpers
// Tests for environment helper functions
// =============================================================================

func TestEnvHelpers(t *testing.T) {
	t.Run("getEnvString returns default when not set", func(t *testing.T) {
		os.Unsetenv("TEST_STRING")
		result := getEnvString("TEST_STRING", "default")
		assert.Equal(t, "default", result)
	})

	t.Run("getEnvString returns value when set", func(t *testing.T) {
		os.Setenv("TEST_STRING", "value")
		defer os.Unsetenv("TEST_STRING")
		result := getEnvString("TEST_STRING", "default")
		assert.Equal(t, "value", result)
	})

	t.Run("getEnvInt returns default when not set", func(t *testing.T) {
		os.Unsetenv("TEST_INT")
		result := getEnvInt("TEST_INT", 42)
		assert.Equal(t, 42, result)
	})

	t.Run("getEnvInt returns value when set", func(t *testing.T) {
		os.Setenv("TEST_INT", "100")
		defer os.Unsetenv("TEST_INT")
		result := getEnvInt("TEST_INT", 42)
		assert.Equal(t, 100, result)
	})

	t.Run("getEnvInt returns default for invalid value", func(t *testing.T) {
		os.Setenv("TEST_INT", "not-a-number")
		defer os.Unsetenv("TEST_INT")
		result := getEnvInt("TEST_INT", 42)
		assert.Equal(t, 42, result)
	})

	t.Run("getEnvFloat returns default when not set", func(t *testing.T) {
		os.Unsetenv("TEST_FLOAT")
		result := getEnvFloat("TEST_FLOAT", 3.14)
		assert.Equal(t, 3.14, result)
	})

	t.Run("getEnvFloat returns value when set", func(t *testing.T) {
		os.Setenv("TEST_FLOAT", "2.5")
		defer os.Unsetenv("TEST_FLOAT")
		result := getEnvFloat("TEST_FLOAT", 3.14)
		assert.Equal(t, 2.5, result)
	})

	t.Run("getEnvBool returns default when not set", func(t *testing.T) {
		os.Unsetenv("TEST_BOOL")
		result := getEnvBool("TEST_BOOL", true)
		assert.True(t, result)
	})

	t.Run("getEnvBool parses true values", func(t *testing.T) {
		tests := []string{"true", "TRUE", "True", "1", "yes", "YES"}
		for _, val := range tests {
			os.Setenv("TEST_BOOL", val)
			result := getEnvBool("TEST_BOOL", false)
			assert.True(t, result, "Expected true for value: %s", val)
		}
		os.Unsetenv("TEST_BOOL")
	})

	t.Run("getEnvBool returns false for other values", func(t *testing.T) {
		os.Setenv("TEST_BOOL", "false")
		result := getEnvBool("TEST_BOOL", true)
		assert.False(t, result)
		os.Unsetenv("TEST_BOOL")
	})
}

// =============================================================================
// TestGetGlobalConfig
// Tests for global configuration access
// =============================================================================

func TestGetGlobalConfig(t *testing.T) {
	t.Run("Get returns loaded config", func(t *testing.T) {
		globalConfig = nil
		cfg := Get()
		assert.NotNil(t, cfg)
	})

	t.Run("Get returns same instance", func(t *testing.T) {
		globalConfig = nil
		cfg1 := Get()
		cfg2 := Get()
		assert.Equal(t, cfg1, cfg2)
	})
}
