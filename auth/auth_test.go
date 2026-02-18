// Package auth provides tests for authentication management.
package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"kiro-go-proxy/config"
)

// =============================================================================
// TestAuthType
// Original: /code/github/kiro-gateway/tests/unit/test_auth_manager.py::TestKiroAuthManagerInitialization
// =============================================================================

func TestAuthType(t *testing.T) {
	t.Run("AuthTypeKiroDesktop string representation", func(t *testing.T) {
		// Original: test_initialization_stores_credentials
		authType := AuthTypeKiroDesktop
		assert.Equal(t, "kiro_desktop", authType.String())
	})

	t.Run("AuthTypeAWSSSOOIDC string representation", func(t *testing.T) {
		// Original: test_initialization_sets_correct_urls_for_region
		authType := AuthTypeAWSSSOOIDC
		assert.Equal(t, "aws_sso_oidc", authType.String())
	})
}

// =============================================================================
// TestManagerInitialization
// Original: /code/github/kiro-gateway/tests/unit/test_auth_manager.py::TestKiroAuthManagerInitialization
// =============================================================================

func TestManagerInitialization(t *testing.T) {
	t.Run("initialization with refresh token", func(t *testing.T) {
		// Original: test_initialization_stores_credentials
		cfg := &config.Config{
			RefreshToken: "test_refresh_123",
			ProfileArn:   "arn:aws:codewhisperer:us-east-1:123456789:profile/test",
			Region:       "us-east-1",
		}
		manager := NewManager(cfg)

		assert.NotNil(t, manager)
	})

	t.Run("initialization with region", func(t *testing.T) {
		// Original: test_initialization_sets_correct_urls_for_region
		cfg := &config.Config{
			RefreshToken: "test_token",
			Region:       "eu-west-1",
		}
		manager := NewManager(cfg)

		assert.NotNil(t, manager)
	})

	t.Run("initialization with empty config", func(t *testing.T) {
		// Original: test_initialization_generates_fingerprint
		cfg := &config.Config{}
		manager := NewManager(cfg)

		assert.NotNil(t, manager)
	})
}

// =============================================================================
// TestTokenData
// Tests for token data structure
// =============================================================================

func TestTokenData(t *testing.T) {
	t.Run("creates token data", func(t *testing.T) {
		data := TokenData{
			AccessToken:  "access_123",
			RefreshToken: "refresh_456",
			ProfileArn:   "arn:aws:test",
			Region:       "us-east-1",
			ExpiresAt:    "2024-01-01T00:00:00Z",
			Scopes:       []string{"scope1", "scope2"},
		}

		assert.Equal(t, "access_123", data.AccessToken)
		assert.Equal(t, "refresh_456", data.RefreshToken)
		assert.Equal(t, "arn:aws:test", data.ProfileArn)
		assert.Equal(t, "us-east-1", data.Region)
		assert.Len(t, data.Scopes, 2)
	})
}

// =============================================================================
// TestDeviceRegistration
// Tests for device registration structure
// =============================================================================

func TestDeviceRegistration(t *testing.T) {
	t.Run("creates device registration", func(t *testing.T) {
		reg := DeviceRegistration{
			ClientID:     "client_123",
			ClientSecret: "secret_456",
			Region:       "us-east-1",
		}

		assert.Equal(t, "client_123", reg.ClientID)
		assert.Equal(t, "secret_456", reg.ClientSecret)
		assert.Equal(t, "us-east-1", reg.Region)
	})
}

// =============================================================================
// TestManagerAccessToken
// Tests for access token retrieval
// =============================================================================

func TestManagerAccessToken(t *testing.T) {
	t.Run("returns empty token when not set", func(t *testing.T) {
		cfg := &config.Config{
			RefreshToken: "test_refresh",
		}
		manager := NewManager(cfg)

		token := manager.AccessToken()
		// When no access token is set, should return empty
		assert.Equal(t, "", token)
	})
}

// =============================================================================
// TestManagerIsTokenExpired
// Tests for token expiration checking
// =============================================================================

func TestManagerIsTokenExpired(t *testing.T) {
	t.Run("returns true when no token", func(t *testing.T) {
		cfg := &config.Config{
			RefreshToken: "test_refresh",
		}
		manager := NewManager(cfg)

		// Without setting a token, should be considered expired
		assert.True(t, manager.IsTokenExpired())
	})
}

// =============================================================================
// TestManagerMethods
// Tests for manager methods
// =============================================================================

func TestManagerMethods(t *testing.T) {
	t.Run("RefreshToken returns empty when not set", func(t *testing.T) {
		cfg := &config.Config{}
		manager := NewManager(cfg)

		token := manager.RefreshToken()
		assert.Equal(t, "", token)
	})

	t.Run("RefreshToken returns token when set", func(t *testing.T) {
		cfg := &config.Config{
			RefreshToken: "test_refresh_token",
		}
		manager := NewManager(cfg)

		token := manager.RefreshToken()
		assert.Equal(t, "test_refresh_token", token)
	})

	t.Run("Region returns region when set", func(t *testing.T) {
		cfg := &config.Config{
			Region: "us-west-2",
		}
		manager := NewManager(cfg)

		region := manager.Region()
		assert.Equal(t, "us-west-2", region)
	})

	t.Run("ProfileArn returns profile arn when set", func(t *testing.T) {
		cfg := &config.Config{
			ProfileArn: "arn:aws:test:profile",
		}
		manager := NewManager(cfg)

		arn := manager.ProfileArn()
		assert.Equal(t, "arn:aws:test:profile", arn)
	})

	t.Run("Fingerprint is generated", func(t *testing.T) {
		cfg := &config.Config{}
		manager := NewManager(cfg)

		fp := manager.Fingerprint()
		assert.NotEmpty(t, fp)
	})

	t.Run("AuthType returns correct type", func(t *testing.T) {
		cfg := &config.Config{
			RefreshToken: "test",
		}
		manager := NewManager(cfg)

		authType := manager.AuthType()
		// Default should be KiroDesktop
		assert.Equal(t, AuthTypeKiroDesktop, authType)
	})
}
