// Package auth provides authentication management for Kiro API.
// It handles token lifecycle: loading credentials, refreshing tokens, and expiration management.
package auth

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"kiro-go-proxy/config"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

// AuthType represents the type of authentication
type AuthType int

const (
	AuthTypeKiroDesktop AuthType = iota
	AuthTypeAWSSSOOIDC
)

func (a AuthType) String() string {
	switch a {
	case AuthTypeAWSSSOOIDC:
		return "aws_sso_oidc"
	default:
		return "kiro_desktop"
	}
}

// SQLite token keys (searched in priority order)
var sqliteTokenKeys = []string{
	"kirocli:social:token",
	"kirocli:odic:token",
	"codewhisperer:odic:token",
}

// SQLite registration keys
var sqliteRegistrationKeys = []string{
	"kirocli:odic:device-registration",
	"codewhisperer:odic:device-registration",
}

// TokenData represents token information stored in SQLite
type TokenData struct {
	AccessToken  string   `json:"access_token"`
	RefreshToken string   `json:"refresh_token"`
	ProfileArn   string   `json:"profile_arn"`
	Region       string   `json:"region"`
	ExpiresAt    string   `json:"expires_at"`
	Scopes       []string `json:"scopes"`
}

// DeviceRegistration represents device registration for AWS SSO OIDC
type DeviceRegistration struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	Region       string `json:"region"`
}

// Manager manages authentication for Kiro API
type Manager struct {
	cfg *config.Config

	// Credentials
	refreshToken string
	profileArn   string
	region       string
	credsFile    string
	sqliteDB     string

	// AWS SSO OIDC specific
	clientID     string
	clientSecret string
	scopes       []string
	ssoRegion    string

	// Enterprise Kiro IDE
	clientIDHash string

	// Token state
	accessToken string
	expiresAt   time.Time

	// Auth type
	authType AuthType

	// Tracking which SQLite key was used
	sqliteTokenKey string

	// URLs
	refreshURL string
	apiHost    string
	qHost      string

	// Fingerprint
	fingerprint string

	// Mutex for thread-safe operations
	mu sync.RWMutex
}

// NewManager creates a new authentication manager
func NewManager(cfg *config.Config) *Manager {
	m := &Manager{
		cfg:          cfg,
		refreshToken: cfg.RefreshToken,
		profileArn:   cfg.ProfileArn,
		region:       cfg.Region,
		credsFile:    cfg.KiroCredsFile,
		sqliteDB:     cfg.KiroCLIDBFile,
		fingerprint:  generateFingerprint(),
	}

	// Set URLs
	m.refreshURL = cfg.GetKiroRefreshURL()
	m.apiHost = cfg.GetKiroAPIHost()
	m.qHost = cfg.GetKiroQHost()

	// Load credentials from SQLite or file
	if m.sqliteDB != "" {
		m.loadCredentialsFromSQLite(m.sqliteDB)
	} else if m.credsFile != "" {
		m.loadCredentialsFromFile(m.credsFile)
	}

	// Detect auth type
	m.detectAuthType()

	log.Infof("Auth manager initialized: region=%s, api_host=%s, auth_type=%s",
		m.region, m.apiHost, m.authType)

	return m
}

// detectAuthType detects the authentication type based on available credentials
func (m *Manager) detectAuthType() {
	if m.clientID != "" && m.clientSecret != "" {
		m.authType = AuthTypeAWSSSOOIDC
		log.Info("Detected auth type: AWS SSO OIDC (kiro-cli)")
	} else {
		m.authType = AuthTypeKiroDesktop
		log.Info("Detected auth type: Kiro Desktop")
	}
}

// loadCredentialsFromSQLite loads credentials from kiro-cli SQLite database
func (m *Manager) loadCredentialsFromSQLite(dbPath string) {
	path := expandPath(dbPath)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		log.Warnf("SQLite database not found: %s", dbPath)
		return
	}

	db, err := sql.Open("sqlite3", path)
	if err != nil {
		log.Errorf("Failed to open SQLite database: %v", err)
		return
	}
	defer db.Close()

	// Try all token keys
	var tokenRow *string
	for _, key := range sqliteTokenKeys {
		var value string
		err := db.QueryRow("SELECT value FROM auth_kv WHERE key = ?", key).Scan(&value)
		if err == nil {
			tokenRow = &value
			m.sqliteTokenKey = key
			log.Debugf("Loaded credentials from SQLite key: %s", key)
			break
		}
	}

	if tokenRow != nil {
		var tokenData TokenData
		if err := json.Unmarshal([]byte(*tokenRow), &tokenData); err == nil {
			if tokenData.AccessToken != "" {
				m.accessToken = tokenData.AccessToken
			}
			if tokenData.RefreshToken != "" {
				m.refreshToken = tokenData.RefreshToken
			}
			if tokenData.ProfileArn != "" {
				m.profileArn = tokenData.ProfileArn
			}
			if tokenData.Region != "" {
				m.ssoRegion = tokenData.Region
				log.Debugf("SSO region from SQLite: %s (API stays at %s)", m.ssoRegion, m.region)
			}
			if len(tokenData.Scopes) > 0 {
				m.scopes = tokenData.Scopes
			}
			if tokenData.ExpiresAt != "" {
				if t, err := parseTime(tokenData.ExpiresAt); err == nil {
					m.expiresAt = t
				}
			}
		}
	}

	// Load device registration
	var regRow *string
	for _, key := range sqliteRegistrationKeys {
		var value string
		err := db.QueryRow("SELECT value FROM auth_kv WHERE key = ?", key).Scan(&value)
		if err == nil {
			regRow = &value
			log.Debugf("Loaded device registration from SQLite key: %s", key)
			break
		}
	}

	if regRow != nil {
		var regData DeviceRegistration
		if err := json.Unmarshal([]byte(*regRow), &regData); err == nil {
			if regData.ClientID != "" {
				m.clientID = regData.ClientID
			}
			if regData.ClientSecret != "" {
				m.clientSecret = regData.ClientSecret
			}
			if regData.Region != "" && m.ssoRegion == "" {
				m.ssoRegion = regData.Region
			}
		}
	}

	log.Infof("Credentials loaded from SQLite database: %s", dbPath)
}

// loadCredentialsFromFile loads credentials from a JSON file
func (m *Manager) loadCredentialsFromFile(filePath string) {
	path := expandPath(filePath)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		log.Warnf("Credentials file not found: %s", filePath)
		return
	}

	data, err := os.ReadFile(path)
	if err != nil {
		log.Errorf("Error reading credentials file: %v", err)
		return
	}

	var creds struct {
		RefreshToken  string `json:"refreshToken"`
		AccessToken   string `json:"accessToken"`
		ProfileArn    string `json:"profileArn"`
		Region        string `json:"region"`
		ExpiresAt     string `json:"expiresAt"`
		ClientID      string `json:"clientId"`
		ClientSecret  string `json:"clientSecret"`
		ClientIDHash  string `json:"clientIdHash"`
	}

	if err := json.Unmarshal(data, &creds); err != nil {
		log.Errorf("Error parsing credentials file: %v", err)
		return
	}

	if creds.RefreshToken != "" {
		m.refreshToken = creds.RefreshToken
	}
	if creds.AccessToken != "" {
		m.accessToken = creds.AccessToken
	}
	if creds.ProfileArn != "" {
		m.profileArn = creds.ProfileArn
	}
	if creds.Region != "" {
		m.region = creds.Region
		m.refreshURL = config.GetKiroRefreshURLForRegion(m.region)
		m.apiHost = config.GetKiroAPIHostForRegion(m.region)
		m.qHost = config.GetKiroAPIHostForRegion(m.region)
	}
	if creds.ClientID != "" {
		m.clientID = creds.ClientID
	}
	if creds.ClientSecret != "" {
		m.clientSecret = creds.ClientSecret
	}
	if creds.ClientIDHash != "" {
		m.clientIDHash = creds.ClientIDHash
		m.loadEnterpriseDeviceRegistration(m.clientIDHash)
	}
	if creds.ExpiresAt != "" {
		if t, err := parseTime(creds.ExpiresAt); err == nil {
			m.expiresAt = t
		}
	}

	log.Infof("Credentials loaded from %s", filePath)
}

// loadEnterpriseDeviceRegistration loads device registration for Enterprise Kiro IDE
func (m *Manager) loadEnterpriseDeviceRegistration(clientIDHash string) {
	deviceRegPath := filepath.Join(os.Getenv("HOME"), ".aws", "sso", "cache", clientIDHash+".json")

	data, err := os.ReadFile(deviceRegPath)
	if err != nil {
		log.Warnf("Enterprise device registration file not found: %s", deviceRegPath)
		return
	}

	var reg DeviceRegistration
	if err := json.Unmarshal(data, &reg); err != nil {
		log.Errorf("Error parsing device registration: %v", err)
		return
	}

	if reg.ClientID != "" {
		m.clientID = reg.ClientID
	}
	if reg.ClientSecret != "" {
		m.clientSecret = reg.ClientSecret
	}

	log.Infof("Enterprise device registration loaded from %s", deviceRegPath)
}

// IsTokenExpiringSoon checks if the token is expiring soon
func (m *Manager) IsTokenExpiringSoon() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.expiresAt.IsZero() {
		return true
	}

	threshold := time.Duration(m.cfg.TokenRefreshThreshold) * time.Second
	return time.Now().Add(threshold).After(m.expiresAt)
}

// IsTokenExpired checks if the token has actually expired
func (m *Manager) IsTokenExpired() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.expiresAt.IsZero() {
		return true
	}
	return time.Now().After(m.expiresAt)
}

// GetAccessToken returns a valid access token, refreshing if necessary
func (m *Manager) GetAccessToken() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Token is valid and not expiring soon
	if m.accessToken != "" && !m.isTokenExpiringSoonUnlocked() {
		return m.accessToken, nil
	}

	// SQLite mode: reload credentials first
	if m.sqliteDB != "" && m.isTokenExpiringSoonUnlocked() {
		log.Debug("SQLite mode: reloading credentials before refresh attempt")
		m.loadCredentialsFromSQLite(m.sqliteDB)
		if m.accessToken != "" && !m.isTokenExpiringSoonUnlocked() {
			log.Debug("SQLite reload provided fresh token, no refresh needed")
			return m.accessToken, nil
		}
	}

	// Try to refresh
	if err := m.refreshTokenRequest(); err != nil {
		// Graceful degradation for SQLite mode
		if m.sqliteDB != "" && m.accessToken != "" && !m.isTokenExpiredUnlocked() {
			log.Warn("Token refresh failed, using existing token until it expires")
			return m.accessToken, nil
		}
		return "", fmt.Errorf("failed to obtain access token: %w", err)
	}

	return m.accessToken, nil
}

// ForceRefresh forces a token refresh
func (m *Manager) ForceRefresh() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.refreshTokenRequest(); err != nil {
		return "", err
	}
	return m.accessToken, nil
}

func (m *Manager) isTokenExpiringSoonUnlocked() bool {
	if m.expiresAt.IsZero() {
		return true
	}
	threshold := time.Duration(m.cfg.TokenRefreshThreshold) * time.Second
	return time.Now().Add(threshold).After(m.expiresAt)
}

func (m *Manager) isTokenExpiredUnlocked() bool {
	if m.expiresAt.IsZero() {
		return true
	}
	return time.Now().After(m.expiresAt)
}

// refreshTokenRequest performs a token refresh request
func (m *Manager) refreshTokenRequest() error {
	if m.authType == AuthTypeAWSSSOOIDC {
		return m.refreshTokenAWSSSOOIDC()
	}
	return m.refreshTokenKiroDesktop()
}

// refreshTokenKiroDesktop refreshes token using Kiro Desktop Auth
func (m *Manager) refreshTokenKiroDesktop() error {
	if m.refreshToken == "" {
		return fmt.Errorf("refresh token is not set")
	}

	log.Info("Refreshing Kiro token via Kiro Desktop Auth...")

	payload := map[string]string{"refreshToken": m.refreshToken}
	jsonData, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", m.refreshURL, strings.NewReader(string(jsonData)))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", fmt.Sprintf("KiroIDE-0.7.45-%s", m.fingerprint))

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("token refresh failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
		ExpiresIn    int    `json:"expiresIn"`
		ProfileArn   string `json:"profileArn"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	if result.AccessToken == "" {
		return fmt.Errorf("response does not contain accessToken")
	}

	m.accessToken = result.AccessToken
	if result.RefreshToken != "" {
		m.refreshToken = result.RefreshToken
	}
	if result.ProfileArn != "" {
		m.profileArn = result.ProfileArn
	}

	// Calculate expiration time with buffer
	m.expiresAt = time.Now().Add(time.Duration(result.ExpiresIn-60) * time.Second)

	log.Infof("Token refreshed via Kiro Desktop Auth, expires: %s", m.expiresAt.Format(time.RFC3339))

	// Save credentials
	if m.sqliteDB != "" {
		m.saveCredentialsToSQLite()
	} else {
		m.saveCredentialsToFile()
	}

	return nil
}

// refreshTokenAWSSSOOIDC refreshes token using AWS SSO OIDC
func (m *Manager) refreshTokenAWSSSOOIDC() error {
	if m.refreshToken == "" {
		return fmt.Errorf("refresh token is not set")
	}
	if m.clientID == "" {
		return fmt.Errorf("client ID is not set (required for AWS SSO OIDC)")
	}
	if m.clientSecret == "" {
		return fmt.Errorf("client secret is not set (required for AWS SSO OIDC)")
	}

	log.Info("Refreshing Kiro token via AWS SSO OIDC...")

	// Use SSO region for OIDC endpoint
	ssoRegion := m.ssoRegion
	if ssoRegion == "" {
		ssoRegion = m.region
	}
	refreshURL := config.GetAWSSSOOIDCURLForRegion(ssoRegion)

	payload := map[string]interface{}{
		"grantType":    "refresh_token",
		"clientId":     m.clientID,
		"clientSecret": m.clientSecret,
		"refreshToken": m.refreshToken,
	}
	jsonData, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", refreshURL, strings.NewReader(string(jsonData)))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	log.Debugf("AWS SSO OIDC refresh request: url=%s, sso_region=%s, api_region=%s",
		refreshURL, ssoRegion, m.region)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Errorf("AWS SSO OIDC refresh failed: status=%d, body=%s", resp.StatusCode, string(body))
		return fmt.Errorf("token refresh failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
		ExpiresIn    int    `json:"expiresIn"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	if result.AccessToken == "" {
		return fmt.Errorf("AWS SSO OIDC response does not contain accessToken")
	}

	m.accessToken = result.AccessToken
	if result.RefreshToken != "" {
		m.refreshToken = result.RefreshToken
	}

	m.expiresAt = time.Now().Add(time.Duration(result.ExpiresIn-60) * time.Second)

	log.Infof("Token refreshed via AWS SSO OIDC, expires: %s", m.expiresAt.Format(time.RFC3339))

	// Save credentials
	if m.sqliteDB != "" {
		m.saveCredentialsToSQLite()
	} else {
		m.saveCredentialsToFile()
	}

	return nil
}

// saveCredentialsToFile saves credentials to JSON file
func (m *Manager) saveCredentialsToFile() {
	if m.credsFile == "" {
		return
	}

	path := expandPath(m.credsFile)

	// Read existing data
	existingData := make(map[string]interface{})
	if data, err := os.ReadFile(path); err == nil {
		json.Unmarshal(data, &existingData)
	}

	// Update data
	existingData["accessToken"] = m.accessToken
	existingData["refreshToken"] = m.refreshToken
	if !m.expiresAt.IsZero() {
		existingData["expiresAt"] = m.expiresAt.Format(time.RFC3339)
	}
	if m.profileArn != "" {
		existingData["profileArn"] = m.profileArn
	}

	// Save
	jsonData, _ := json.MarshalIndent(existingData, "", "  ")
	if err := os.WriteFile(path, jsonData, 0600); err != nil {
		log.Errorf("Error saving credentials: %v", err)
		return
	}

	log.Debugf("Credentials saved to %s", m.credsFile)
}

// saveCredentialsToSQLite saves credentials to SQLite database
func (m *Manager) saveCredentialsToSQLite() {
	if m.sqliteDB == "" {
		return
	}

	path := expandPath(m.sqliteDB)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		log.Warnf("SQLite database not found for writing: %s", m.sqliteDB)
		return
	}

	db, err := sql.Open("sqlite3", path)
	if err != nil {
		log.Errorf("Failed to open SQLite database: %v", err)
		return
	}
	defer db.Close()

	// Prepare token data
	tokenData := map[string]interface{}{
		"access_token":  m.accessToken,
		"refresh_token": m.refreshToken,
		"expires_at":    m.expiresAt.Format(time.RFC3339),
		"region":        m.ssoRegion,
	}
	if len(m.scopes) > 0 {
		tokenData["scopes"] = m.scopes
	}

	jsonData, _ := json.Marshal(tokenData)

	// Update the key we loaded from
	if m.sqliteTokenKey != "" {
		result, err := db.Exec("UPDATE auth_kv SET value = ? WHERE key = ?", string(jsonData), m.sqliteTokenKey)
		if err == nil {
			if rows, _ := result.RowsAffected(); rows > 0 {
				log.Debugf("Credentials saved to SQLite key: %s", m.sqliteTokenKey)
				return
			}
		}
	}

	// Fallback: try all keys
	for _, key := range sqliteTokenKeys {
		result, err := db.Exec("UPDATE auth_kv SET value = ? WHERE key = ?", string(jsonData), key)
		if err == nil {
			if rows, _ := result.RowsAffected(); rows > 0 {
				log.Debugf("Credentials saved to SQLite key: %s (fallback)", key)
				return
			}
		}
	}

	log.Warn("Failed to save credentials to SQLite: no matching keys found")
}

// Properties
func (m *Manager) ProfileArn() string    { return m.profileArn }
func (m *Manager) Region() string        { return m.region }
func (m *Manager) APIHost() string       { return m.apiHost }
func (m *Manager) QHost() string         { return m.qHost }
func (m *Manager) Fingerprint() string   { return m.fingerprint }
func (m *Manager) AuthType() AuthType    { return m.authType }
func (m *Manager) AccessToken() string   { return m.accessToken }
func (m *Manager) RefreshToken() string  { return m.refreshToken }

// Helper functions
func generateFingerprint() string {
	return uuid.New().String()[:8]
}

func expandPath(path string) string {
	if strings.HasPrefix(path, "~") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[1:])
	}
	return path
}

func parseTime(s string) (time.Time, error) {
	if strings.HasSuffix(s, "Z") {
		s = s[:len(s)-1] + "+00:00"
	}
	return time.Parse(time.RFC3339, s)
}
