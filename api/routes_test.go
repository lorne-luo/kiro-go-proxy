// Package api provides tests for HTTP routes.
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"kiro-go-proxy/auth"
	"kiro-go-proxy/config"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// Helper function to create test server
func newTestServer(proxyAPIKey string) (*Server, *gin.Engine) {
	cfg := &config.Config{
		ProxyAPIKey: proxyAPIKey,
	}
	authManager := &auth.Manager{}
	server := NewServer(cfg, authManager)

	router := gin.New()
	server.SetupRoutes(router)

	return server, router
}

// =============================================================================
// TestAuthMiddleware
// Original: /code/github/kiro-gateway/tests/unit/test_routes_anthropic.py::TestVerifyAnthropicApiKey
// =============================================================================

func TestAuthMiddleware(t *testing.T) {
	t.Run("accepts valid Bearer token", func(t *testing.T) {
		// Original: test_valid_bearer_token_returns_true
		_, router := newTestServer("test-api-key")

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/models", nil)
		req.Header.Set("Authorization", "Bearer test-api-key")
		router.ServeHTTP(w, req)

		assert.NotEqual(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("accepts valid API key without Bearer prefix", func(t *testing.T) {
		// Original: test_valid_x_api_key_returns_true
		_, router := newTestServer("test-api-key")

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/models", nil)
		req.Header.Set("Authorization", "test-api-key")
		router.ServeHTTP(w, req)

		assert.NotEqual(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("rejects invalid API key", func(t *testing.T) {
		// Original: test_invalid_x_api_key_raises_401
		_, router := newTestServer("correct-key")

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/models", nil)
		req.Header.Set("Authorization", "Bearer wrong-key")
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.Contains(t, w.Body.String(), "Invalid API key")
	})

	t.Run("rejects missing authorization header", func(t *testing.T) {
		// Original: test_missing_both_headers_raises_401
		_, router := newTestServer("test-api-key")

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/models", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.Contains(t, w.Body.String(), "Missing Authorization header")
	})

	t.Run("rejects empty authorization header", func(t *testing.T) {
		// Original: test_empty_x_api_key_raises_401
		_, router := newTestServer("test-api-key")

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/models", nil)
		req.Header.Set("Authorization", "")
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

// =============================================================================
// TestHealthHandler
// Tests for health check endpoint
// =============================================================================

func TestHealthHandler(t *testing.T) {
	t.Run("returns ok status", func(t *testing.T) {
		_, router := newTestServer("test-key")

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/health", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "ok")
	})

	t.Run("root path returns health", func(t *testing.T) {
		_, router := newTestServer("test-key")

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "ok")
	})

	t.Run("health does not require auth", func(t *testing.T) {
		_, router := newTestServer("test-key")

		// No auth header needed
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/health", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})
}

// =============================================================================
// TestListModelsHandler
// Tests for models listing endpoint
// =============================================================================

func TestListModelsHandler(t *testing.T) {
	t.Run("returns models list", func(t *testing.T) {
		_, router := newTestServer("test-key")

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/models", nil)
		req.Header.Set("Authorization", "Bearer test-key")
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "object")
		assert.Contains(t, w.Body.String(), "list")
	})

	t.Run("requires authentication", func(t *testing.T) {
		_, router := newTestServer("test-key")

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/models", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

// =============================================================================
// TestErrorFormat
// Tests for error response format
// =============================================================================

func TestErrorFormat(t *testing.T) {
	t.Run("auth error has correct format", func(t *testing.T) {
		// Original: test_auth_error_format_is_anthropic_style
		_, router := newTestServer("test-key")

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/models", nil)
		router.ServeHTTP(w, req)

		// Error response should have error.message structure
		assert.Contains(t, w.Body.String(), "error")
		assert.Contains(t, w.Body.String(), "message")
		assert.Contains(t, w.Body.String(), "type")
	})
}

// =============================================================================
// TestChatCompletionsValidation
// Original: /code/github/kiro-gateway/tests/unit/test_routes_openai.py::TestChatCompletionsValidation
// =============================================================================

func TestChatCompletionsValidation(t *testing.T) {
	t.Run("rejects invalid JSON", func(t *testing.T) {
		// Original: test_validates_invalid_json
		_, router := newTestServer("test-key")

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/chat/completions", strings.NewReader("not valid json"))
		req.Header.Set("Authorization", "Bearer test-key")
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("rejects empty body", func(t *testing.T) {
		// Original: test_validates_invalid_json
		_, router := newTestServer("test-key")

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/chat/completions", nil)
		req.Header.Set("Authorization", "Bearer test-key")
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("requires authentication", func(t *testing.T) {
		// Original: test_chat_completions_requires_authentication
		_, router := newTestServer("test-key")

		body := `{"model": "claude-haiku-4.5", "messages": [{"role": "user", "content": "Hello"}]}`
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("rejects invalid API key", func(t *testing.T) {
		// Original: test_chat_completions_rejects_invalid_key
		_, router := newTestServer("correct-key")

		body := `{"model": "claude-haiku-4.5", "messages": [{"role": "user", "content": "Hello"}]}`
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer wrong-key")
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

// =============================================================================
// TestRootEndpoint
// Original: /code/github/kiro-gateway/tests/unit/test_routes_openai.py::TestRootEndpoint
// =============================================================================

func TestRootEndpoint(t *testing.T) {
	t.Run("returns status ok", func(t *testing.T) {
		// Original: test_root_returns_status_ok
		_, router := newTestServer("test-key")

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "ok")
	})

	t.Run("returns gateway message", func(t *testing.T) {
		// Original: test_root_returns_gateway_message
		_, router := newTestServer("test-key")

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		// Response should contain status
		assert.Contains(t, w.Body.String(), "status")
	})

	t.Run("does not require auth", func(t *testing.T) {
		// Original: test_root_does_not_require_auth
		_, router := newTestServer("test-key")

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})
}

// =============================================================================
// TestModelsEndpointDetailed
// Original: /code/github/kiro-gateway/tests/unit/test_routes_openai.py::TestModelsEndpoint
// =============================================================================

func TestModelsEndpointDetailed(t *testing.T) {
	t.Run("returns list object", func(t *testing.T) {
		// Original: test_models_returns_list_object
		_, router := newTestServer("test-key")

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/models", nil)
		req.Header.Set("Authorization", "Bearer test-key")
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &resp)
		assert.Equal(t, "list", resp["object"])
	})

	t.Run("returns data array", func(t *testing.T) {
		// Original: test_models_returns_data_array
		_, router := newTestServer("test-key")

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/models", nil)
		req.Header.Set("Authorization", "Bearer test-key")
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &resp)
		assert.Contains(t, resp, "data")
		// Data can be nil if no models available, but key should exist
	})

	t.Run("rejects invalid key", func(t *testing.T) {
		// Original: test_models_rejects_invalid_key
		_, router := newTestServer("correct-key")

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/models", nil)
		req.Header.Set("Authorization", "Bearer wrong-key")
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

// =============================================================================
// TestChatCompletionsWithTools
// Original: /code/github/kiro-gateway/tests/unit/test_routes_openai.py::TestChatCompletionsWithTools
// =============================================================================

func TestChatCompletionsWithTools(t *testing.T) {
	t.Run("accepts valid tool definition", func(t *testing.T) {
		// Original: test_accepts_valid_tool_definition
		_, router := newTestServer("test-key")

		body := `{
			"model": "claude-haiku-4.5",
			"messages": [{"role": "user", "content": "Hello"}],
			"tools": [{"type": "function", "function": {"name": "get_weather", "description": "Get weather", "parameters": {"type": "object"}}}]
		}`
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer test-key")
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		// Should not return 400 for valid tool definition
		assert.NotEqual(t, http.StatusBadRequest, w.Code)
	})

	t.Run("accepts multiple tools", func(t *testing.T) {
		// Original: test_accepts_multiple_tools
		_, router := newTestServer("test-key")

		body := `{
			"model": "claude-haiku-4.5",
			"messages": [{"role": "user", "content": "Hello"}],
			"tools": [
				{"type": "function", "function": {"name": "get_weather", "description": "Get weather"}},
				{"type": "function", "function": {"name": "search", "description": "Search web"}}
			]
		}`
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer test-key")
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		// Should not return 400 for valid tools
		assert.NotEqual(t, http.StatusBadRequest, w.Code)
	})
}

// =============================================================================
// TestChatCompletionsOptionalParams
// Original: /code/github/kiro-gateway/tests/unit/test_routes_openai.py::TestChatCompletionsOptionalParams
// =============================================================================

func TestChatCompletionsOptionalParams(t *testing.T) {
	t.Run("accepts temperature parameter", func(t *testing.T) {
		// Original: test_accepts_temperature_parameter
		_, router := newTestServer("test-key")

		body := `{
			"model": "claude-haiku-4.5",
			"messages": [{"role": "user", "content": "Hello"}],
			"temperature": 0.7
		}`
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer test-key")
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		// Should not return 400 for valid temperature
		assert.NotEqual(t, http.StatusBadRequest, w.Code)
	})

	t.Run("accepts max_tokens parameter", func(t *testing.T) {
		// Original: test_accepts_max_tokens_parameter
		_, router := newTestServer("test-key")

		body := `{
			"model": "claude-haiku-4.5",
			"messages": [{"role": "user", "content": "Hello"}],
			"max_tokens": 1024
		}`
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer test-key")
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		// Should not return 400 for valid max_tokens
		assert.NotEqual(t, http.StatusBadRequest, w.Code)
	})

	t.Run("accepts stream true", func(t *testing.T) {
		// Original: test_accepts_stream_true
		_, router := newTestServer("test-key")

		body := `{
			"model": "claude-haiku-4.5",
			"messages": [{"role": "user", "content": "Hello"}],
			"stream": true
		}`
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer test-key")
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		// Should not return 400 for stream parameter
		assert.NotEqual(t, http.StatusBadRequest, w.Code)
	})

	t.Run("accepts top_p parameter", func(t *testing.T) {
		// Original: test_accepts_top_p_parameter
		_, router := newTestServer("test-key")

		body := `{
			"model": "claude-haiku-4.5",
			"messages": [{"role": "user", "content": "Hello"}],
			"top_p": 0.9
		}`
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer test-key")
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		// Should not return 400 for valid top_p
		assert.NotEqual(t, http.StatusBadRequest, w.Code)
	})
}

// =============================================================================
// TestChatCompletionsMessageTypes
// Original: /code/github/kiro-gateway/tests/unit/test_routes_openai.py::TestChatCompletionsMessageTypes
// =============================================================================

func TestChatCompletionsMessageTypes(t *testing.T) {
	t.Run("accepts system message", func(t *testing.T) {
		// Original: test_accepts_system_message
		_, router := newTestServer("test-key")

		body := `{
			"model": "claude-haiku-4.5",
			"messages": [
				{"role": "system", "content": "You are helpful"},
				{"role": "user", "content": "Hello"}
			]
		}`
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer test-key")
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		// Should not return 400 for valid system message
		assert.NotEqual(t, http.StatusBadRequest, w.Code)
	})

	t.Run("accepts assistant message", func(t *testing.T) {
		// Original: test_accepts_assistant_message
		_, router := newTestServer("test-key")

		body := `{
			"model": "claude-haiku-4.5",
			"messages": [
				{"role": "user", "content": "Hello"},
				{"role": "assistant", "content": "Hi there!"},
				{"role": "user", "content": "How are you?"}
			]
		}`
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer test-key")
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		// Should not return 400 for valid assistant message
		assert.NotEqual(t, http.StatusBadRequest, w.Code)
	})

	t.Run("accepts multipart content", func(t *testing.T) {
		// Original: test_accepts_multipart_content
		_, router := newTestServer("test-key")

		body := `{
			"model": "claude-haiku-4.5",
			"messages": [
				{
					"role": "user",
					"content": [
						{"type": "text", "text": "Hello"},
						{"type": "text", "text": "World"}
					]
				}
			]
		}`
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer test-key")
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		// Should not return 400 for valid multipart content
		assert.NotEqual(t, http.StatusBadRequest, w.Code)
	})
}

// =============================================================================
// TestRouterIntegration
// Original: /code/github/kiro-gateway/tests/unit/test_routes_openai.py::TestRouterIntegration
// =============================================================================

func TestRouterIntegration(t *testing.T) {
	t.Run("router has root endpoint", func(t *testing.T) {
		// Original: test_router_has_root_endpoint
		_, router := newTestServer("test-key")

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("router has health endpoint", func(t *testing.T) {
		// Original: test_router_has_health_endpoint
		_, router := newTestServer("test-key")

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/health", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("router has models endpoint", func(t *testing.T) {
		// Original: test_router_has_models_endpoint
		_, router := newTestServer("test-key")

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/models", nil)
		req.Header.Set("Authorization", "Bearer test-key")
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("router has chat completions endpoint", func(t *testing.T) {
		// Original: test_router_has_chat_completions_endpoint
		_, router := newTestServer("test-key")

		body := `{"model": "claude-haiku-4.5", "messages": [{"role": "user", "content": "Hello"}]}`
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer test-key")
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		// Endpoint should exist (may return error due to no auth backend, but not 404)
		assert.NotEqual(t, http.StatusNotFound, w.Code)
	})

	t.Run("root endpoint uses GET method", func(t *testing.T) {
		// Original: test_root_endpoint_uses_get_method
		_, router := newTestServer("test-key")

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("health endpoint uses GET method", func(t *testing.T) {
		// Original: test_health_endpoint_uses_get_method
		_, router := newTestServer("test-key")

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/health", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("models endpoint uses GET method", func(t *testing.T) {
		// Original: test_models_endpoint_uses_get_method
		_, router := newTestServer("test-key")

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/models", nil)
		req.Header.Set("Authorization", "Bearer test-key")
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("chat completions endpoint uses POST method", func(t *testing.T) {
		// Original: test_chat_completions_endpoint_uses_post_method
		_, router := newTestServer("test-key")

		body := `{"model": "claude-haiku-4.5", "messages": [{"role": "user", "content": "Hello"}]}`
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer test-key")
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		// Should not be 404 (endpoint exists) or 405 (method allowed)
		assert.NotEqual(t, http.StatusNotFound, w.Code)
		assert.NotEqual(t, http.StatusMethodNotAllowed, w.Code)
	})
}
