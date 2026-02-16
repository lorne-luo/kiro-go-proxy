// Package config provides centralized configuration management for Kiro Gateway.
// It loads environment variables and provides typed access to all settings.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// Application metadata
const (
	AppVersion     = "2.3"
	AppTitle       = "Kiro Gateway (Go)"
	AppDescription = "Proxy gateway for Kiro API (Amazon Q Developer / AWS CodeWhisperer). OpenAI and Anthropic compatible."
)

// Config holds all configuration settings
type Config struct {
	// Server settings
	ServerHost string
	ServerPort int

	// Proxy settings
	ProxyAPIKey string
	VPNProxyURL string

	// Kiro credentials
	RefreshToken  string
	ProfileArn    string
	Region        string
	KiroCredsFile string
	KiroCLIDBFile string

	// Token settings
	TokenRefreshThreshold int

	// Retry configuration
	MaxRetries     int
	BaseRetryDelay float64

	// Model settings
	HiddenModels    map[string]string
	ModelAliases    map[string]string
	HiddenFromList  []string
	FallbackModels  []ModelInfo
	ModelCacheTTL   int
	MaxInputTokens  int

	// Tool settings
	ToolDescriptionMaxLength int

	// Truncation recovery
	TruncationRecovery bool

	// Logging
	LogLevel string

	// Timeout settings
	FirstTokenTimeout    float64
	StreamingReadTimeout float64
	FirstTokenMaxRetries int

	// Debug settings
	DebugMode string
	DebugDir  string

	// Fake reasoning settings
	FakeReasoningEnabled    bool
	FakeReasoningMaxTokens  int
	FakeReasoningHandling   string
	FakeReasoningOpenTags   []string
	FakeReasoningBufferSize int
}

// ModelInfo represents model information
type ModelInfo struct {
	ModelID string `json:"modelId"`
}

// Default values
var defaults = &Config{
	ServerHost:               "0.0.0.0",
	ServerPort:               8000,
	ProxyAPIKey:              "my-super-secret-password-123",
	VPNProxyURL:              "",
	Region:                   "us-east-1",
	TokenRefreshThreshold:    600,
	MaxRetries:               3,
	BaseRetryDelay:           1.0,
	ModelCacheTTL:            3600,
	MaxInputTokens:           200000,
	ToolDescriptionMaxLength: 10000,
	TruncationRecovery:       true,
	LogLevel:                 "INFO",
	FirstTokenTimeout:        15,
	StreamingReadTimeout:     300,
	FirstTokenMaxRetries:     3,
	DebugMode:                "off",
	DebugDir:                 "debug_logs",
	FakeReasoningEnabled:     true,
	FakeReasoningMaxTokens:   4000,
	FakeReasoningHandling:    "as_reasoning_content",
	FakeReasoningOpenTags:    []string{"<thinking>", "alettek", "<reasoning>", "<thought>"},
	FakeReasoningBufferSize:  20,
	HiddenModels: map[string]string{
		"claude-3.7-sonnet": "CLAUDE_3_7_SONNET_20250219_V1_0",
	},
	ModelAliases: map[string]string{
		"auto-kiro": "auto",
	},
	HiddenFromList: []string{"auto"},
	FallbackModels: []ModelInfo{
		{ModelID: "auto"},
		{ModelID: "claude-sonnet-4"},
		{ModelID: "claude-haiku-4.5"},
		{ModelID: "claude-sonnet-4.5"},
		{ModelID: "claude-opus-4.5"},
	},
}

var globalConfig *Config

// Load loads configuration from environment and .env file
func Load() *Config {
	// Load .env file if exists
	godotenv.Load()

	cfg := &Config{
		ServerHost:               getEnvString("SERVER_HOST", defaults.ServerHost),
		ServerPort:               getEnvInt("SERVER_PORT", defaults.ServerPort),
		ProxyAPIKey:              getEnvString("PROXY_API_KEY", defaults.ProxyAPIKey),
		VPNProxyURL:              getEnvString("VPN_PROXY_URL", defaults.VPNProxyURL),
		RefreshToken:             getEnvString("REFRESH_TOKEN", ""),
		ProfileArn:               getEnvString("PROFILE_ARN", ""),
		Region:                   getEnvString("KIRO_REGION", defaults.Region),
		KiroCredsFile:            getEnvString("KIRO_CREDS_FILE", ""),
		KiroCLIDBFile:            getEnvString("KIRO_CLI_DB_FILE", ""),
		TokenRefreshThreshold:    getEnvInt("TOKEN_REFRESH_THRESHOLD", defaults.TokenRefreshThreshold),
		MaxRetries:               getEnvInt("MAX_RETRIES", defaults.MaxRetries),
		BaseRetryDelay:           getEnvFloat("BASE_RETRY_DELAY", defaults.BaseRetryDelay),
		ModelCacheTTL:            getEnvInt("MODEL_CACHE_TTL", defaults.ModelCacheTTL),
		MaxInputTokens:           getEnvInt("DEFAULT_MAX_INPUT_TOKENS", defaults.MaxInputTokens),
		ToolDescriptionMaxLength: getEnvInt("TOOL_DESCRIPTION_MAX_LENGTH", defaults.ToolDescriptionMaxLength),
		TruncationRecovery:       getEnvBool("TRUNCATION_RECOVERY", defaults.TruncationRecovery),
		LogLevel:                 getEnvString("LOG_LEVEL", defaults.LogLevel),
		FirstTokenTimeout:        getEnvFloat("FIRST_TOKEN_TIMEOUT", defaults.FirstTokenTimeout),
		StreamingReadTimeout:     getEnvFloat("STREAMING_READ_TIMEOUT", defaults.StreamingReadTimeout),
		FirstTokenMaxRetries:     getEnvInt("FIRST_TOKEN_MAX_RETRIES", defaults.FirstTokenMaxRetries),
		DebugMode:                getEnvString("DEBUG_MODE", defaults.DebugMode),
		DebugDir:                 getEnvString("DEBUG_DIR", defaults.DebugDir),
		FakeReasoningEnabled:     getEnvBool("FAKE_REASONING", defaults.FakeReasoningEnabled),
		FakeReasoningMaxTokens:   getEnvInt("FAKE_REASONING_MAX_TOKENS", defaults.FakeReasoningMaxTokens),
		FakeReasoningHandling:    getEnvString("FAKE_REASONING_HANDLING", defaults.FakeReasoningHandling),
		FakeReasoningBufferSize:  getEnvInt("FAKE_REASONING_INITIAL_BUFFER_SIZE", defaults.FakeReasoningBufferSize),
	}

	// Copy maps and slices
	cfg.HiddenModels = make(map[string]string)
	for k, v := range defaults.HiddenModels {
		cfg.HiddenModels[k] = v
	}
	cfg.ModelAliases = make(map[string]string)
	for k, v := range defaults.ModelAliases {
		cfg.ModelAliases[k] = v
	}
	cfg.HiddenFromList = make([]string, len(defaults.HiddenFromList))
	copy(cfg.HiddenFromList, defaults.HiddenFromList)
	cfg.FallbackModels = make([]ModelInfo, len(defaults.FallbackModels))
	copy(cfg.FallbackModels, defaults.FallbackModels)
	cfg.FakeReasoningOpenTags = make([]string, len(defaults.FakeReasoningOpenTags))
	copy(cfg.FakeReasoningOpenTags, defaults.FakeReasoningOpenTags)

	globalConfig = cfg
	return cfg
}

// Get returns the global configuration
func Get() *Config {
	if globalConfig == nil {
		return Load()
	}
	return globalConfig
}

// URL templates
const (
	KiroRefreshURLTemplate = "https://prod.{region}.auth.desktop.kiro.dev/refreshToken"
	AWSSSOOIDCURLTemplate  = "https://oidc.{region}.amazonaws.com/token"
	KiroAPIHostTemplate    = "https://q.{region}.amazonaws.com"
	KiroQHostTemplate      = "https://q.{region}.amazonaws.com"
)

// GetKiroRefreshURL returns the Kiro Desktop Auth token refresh URL
func (c *Config) GetKiroRefreshURL() string {
	return strings.ReplaceAll(KiroRefreshURLTemplate, "{region}", c.Region)
}

// GetAWSSSOOIDCURL returns the AWS SSO OIDC token URL
func (c *Config) GetAWSSSOOIDCURL() string {
	return strings.ReplaceAll(AWSSSOOIDCURLTemplate, "{region}", c.Region)
}

// GetKiroAPIHost returns the API host
func (c *Config) GetKiroAPIHost() string {
	return strings.ReplaceAll(KiroAPIHostTemplate, "{region}", c.Region)
}

// GetKiroQHost returns the Q API host
func (c *Config) GetKiroQHost() string {
	return strings.ReplaceAll(KiroQHostTemplate, "{region}", c.Region)
}

// GetKiroRefreshURLForRegion returns the refresh URL for a specific region
func GetKiroRefreshURLForRegion(region string) string {
	return strings.ReplaceAll(KiroRefreshURLTemplate, "{region}", region)
}

// GetAWSSSOOIDCURLForRegion returns the OIDC URL for a specific region
func GetAWSSSOOIDCURLForRegion(region string) string {
	return strings.ReplaceAll(AWSSSOOIDCURLTemplate, "{region}", region)
}

// GetKiroAPIHostForRegion returns the API host for a specific region
func GetKiroAPIHostForRegion(region string) string {
	return strings.ReplaceAll(KiroAPIHostTemplate, "{region}", region)
}

// Helper functions
func getEnvString(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func getEnvFloat(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		if floatVal, err := strconv.ParseFloat(value, 64); err == nil {
			return floatVal
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		lower := strings.ToLower(value)
		return lower == "true" || lower == "1" || lower == "yes"
	}
	return defaultValue
}

// Validate checks if required configuration is present
func (c *Config) Validate() error {
	hasRefreshToken := c.RefreshToken != ""
	hasCredsFile := c.KiroCredsFile != ""
	hasCLIDB := c.KiroCLIDBFile != ""

	if !hasRefreshToken && !hasCredsFile && !hasCLIDB {
		return fmt.Errorf("no Kiro credentials configured. Set REFRESH_TOKEN, KIRO_CREDS_FILE, or KIRO_CLI_DB_FILE")
	}
	return nil
}
