// Package client provides HTTP client with retry logic for Kiro API.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"kiro-go-proxy/auth"
	"kiro-go-proxy/config"

	log "github.com/sirupsen/logrus"
)

// Client wraps http.Client with retry logic
type Client struct {
	httpClient     *http.Client
	cfg            *config.Config
	authManager    *auth.Manager
	proxyURL       string
}

// NewClient creates a new HTTP client
func NewClient(cfg *config.Config, authManager *auth.Manager) *Client {
	// Configure transport
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 20,
		IdleConnTimeout:     30 * time.Second,
	}

	// Configure proxy if set
	proxyURL := cfg.VPNProxyURL
	if proxyURL != "" {
		if !strings.Contains(proxyURL, "://") {
			proxyURL = "http://" + proxyURL
		}
		if proxy, err := url.Parse(proxyURL); err == nil {
			transport.Proxy = http.ProxyURL(proxy)
			log.Infof("Proxy configured: %s", proxyURL)
		}
	}

	return &Client{
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   time.Duration(cfg.StreamingReadTimeout) * time.Second,
		},
		cfg:         cfg,
		authManager: authManager,
		proxyURL:    proxyURL,
	}
}

// RequestWithRetry makes an HTTP request with retry logic
func (c *Client) RequestWithRetry(ctx context.Context, method, url string, payload interface{}, stream bool) (*http.Response, error) {
	var lastErr error

	for attempt := 0; attempt < c.cfg.MaxRetries; attempt++ {
		if attempt > 0 {
			delay := time.Duration(c.cfg.BaseRetryDelay*float64(int(1)<<uint(attempt))) * time.Second
			log.Warnf("Retry attempt %d/%d after %v", attempt+1, c.cfg.MaxRetries, delay)
			time.Sleep(delay)
		}

		resp, err := c.doRequest(ctx, method, url, payload, stream)
		if err != nil {
			lastErr = err
			continue
		}

		// Check for retryable status codes
		if resp.StatusCode == http.StatusForbidden {
			log.Info("Received 403, attempting token refresh...")
			if _, refreshErr := c.authManager.ForceRefresh(); refreshErr != nil {
				log.Errorf("Token refresh failed: %v", refreshErr)
			}
			resp.Body.Close()
			continue
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			log.Warn("Rate limited (429), waiting before retry...")
			resp.Body.Close()
			continue
		}

		if resp.StatusCode >= 500 {
			log.Warnf("Server error (%d), retrying...", resp.StatusCode)
			resp.Body.Close()
			continue
		}

		return resp, nil
	}

	return nil, fmt.Errorf("all %d retry attempts failed: %w", c.cfg.MaxRetries, lastErr)
}

func (c *Client) doRequest(ctx context.Context, method, url string, payload interface{}, stream bool) (*http.Response, error) {
	// Get access token
	token, err := c.authManager.GetAccessToken()
	if err != nil {
		return nil, fmt.Errorf("failed to get access token: %w", err)
	}

	// Prepare request body
	var body io.Reader
	if payload != nil {
		jsonData, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal payload: %w", err)
		}
		body = bytes.NewReader(jsonData)
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("User-Agent", fmt.Sprintf("KiroGateway-Go/%s", config.AppVersion))

	if stream {
		req.Header.Set("Accept", "text/event-stream")
	}

	// Add profile ARN if available
	if c.authManager.ProfileArn() != "" {
		req.Header.Set("X-Amz-Profile-Arn", c.authManager.ProfileArn())
	}

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	return resp, nil
}

// DoRequest performs a simple HTTP request without retry logic
func (c *Client) DoRequest(ctx context.Context, method, url string, payload interface{}) (*http.Response, error) {
	return c.doRequest(ctx, method, url, payload, false)
}

// Get performs a GET request
func (c *Client) Get(ctx context.Context, url string) (*http.Response, error) {
	return c.RequestWithRetry(ctx, "GET", url, nil, false)
}

// Post performs a POST request
func (c *Client) Post(ctx context.Context, url string, payload interface{}) (*http.Response, error) {
	return c.RequestWithRetry(ctx, "POST", url, payload, false)
}

// PostStream performs a POST request expecting a streaming response
func (c *Client) PostStream(ctx context.Context, url string, payload interface{}) (*http.Response, error) {
	return c.RequestWithRetry(ctx, "POST", url, payload, true)
}

// ReadErrorBody reads and returns the error body from a response
func ReadErrorBody(resp *http.Response) string {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Sprintf("failed to read body: %v", err)
	}
	return string(body)
}

// Close ensures the response body is properly closed
func Close(resp *http.Response) {
	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}
}
