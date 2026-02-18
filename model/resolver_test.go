// Package model provides tests for model resolution and caching.
package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"kiro-go-proxy/config"
)

// =============================================================================
// TestNormalizeModelName
// Original: /code/github/kiro-gateway/tests/unit/test_model_resolver.py::TestNormalizeModelName
// =============================================================================

func TestNormalizeModelName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// Standard format tests
		{
			name:     "standard format with minor",
			input:    "claude-haiku-4-5",
			expected: "claude-haiku-4.5",
		},
		{
			name:     "standard format with date suffix",
			input:    "claude-sonnet-4-5-20251001",
			expected: "claude-sonnet-4.5",
		},
		{
			name:     "standard format with latest suffix",
			input:    "claude-opus-4-5-latest",
			expected: "claude-opus-4.5",
		},
		{
			name:     "standard format without minor",
			input:    "claude-sonnet-4",
			expected: "claude-sonnet-4",
		},
		{
			name:     "standard format without minor with date",
			input:    "claude-sonnet-4-20250514",
			expected: "claude-sonnet-4",
		},

		// Legacy format tests
		{
			name:     "legacy format",
			input:    "claude-3-7-sonnet",
			expected: "claude-3.7-sonnet",
		},
		{
			name:     "legacy format with date",
			input:    "claude-3-7-sonnet-20250219",
			expected: "claude-3.7-sonnet",
		},
		{
			name:     "legacy format haiku",
			input:    "claude-3-5-haiku",
			expected: "claude-3.5-haiku",
		},

		// Already normalized tests
		{
			name:     "already normalized",
			input:    "claude-haiku-4.5",
			expected: "claude-haiku-4.5",
		},
		{
			name:     "already normalized with date",
			input:    "claude-haiku-4.5-20251001",
			expected: "claude-haiku-4.5",
		},

		// Inverted format tests
		{
			name:     "inverted format with suffix",
			input:    "claude-4.5-opus-high",
			expected: "claude-opus-4.5",
		},

		// Case handling
		{
			name:     "uppercase input",
			input:    "CLAUDE-HAIKU-4-5",
			expected: "claude-haiku-4.5",
		},
		{
			name:     "mixed case input",
			input:    "Claude-Sonnet-4-5",
			expected: "claude-sonnet-4.5",
		},

		// Edge cases
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "unknown model passed through",
			input:    "gpt-4",
			expected: "gpt-4",
		},
		{
			name:     "custom model name",
			input:    "custom-model",
			expected: "custom-model",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeModelName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// =============================================================================
// TestExtractModelFamily
// Original: /code/github/kiro-gateway/tests/unit/test_model_resolver.py::TestExtractModelFamily
// =============================================================================

func TestExtractModelFamily(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "extracts haiku",
			input:    "claude-haiku-4.5",
			expected: "haiku",
		},
		{
			name:     "extracts sonnet",
			input:    "claude-sonnet-4",
			expected: "sonnet",
		},
		{
			name:     "extracts opus",
			input:    "claude-3.7-opus",
			expected: "opus",
		},
		{
			name:     "case insensitive",
			input:    "CLAUDE-HAIKU-4",
			expected: "haiku",
		},
		{
			name:     "no family returns empty",
			input:    "gpt-4",
			expected: "",
		},
		{
			name:     "empty string returns empty",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractModelFamily(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// =============================================================================
// TestModelCache
// Original: /code/github/kiro-gateway/tests/unit/test_model_resolver.py::TestModelCache
// =============================================================================

func newTestConfig() *config.Config {
	return &config.Config{
		ModelCacheTTL:     300,
		MaxInputTokens:    200000,
		FallbackModels:    []config.ModelInfo{},
		HiddenModels:      make(map[string]string),
		ModelAliases:      make(map[string]string),
		HiddenFromList:    []string{},
	}
}

func TestModelCache(t *testing.T) {
	t.Run("creates empty cache", func(t *testing.T) {
		// Original: test_creates_empty_cache
		cfg := newTestConfig()
		cache := NewCache(cfg)

		assert.True(t, cache.IsEmpty())
		assert.Equal(t, 0, cache.Size())
	})

	t.Run("updates cache with models", func(t *testing.T) {
		// Original: test_updates_cache_with_models
		cfg := newTestConfig()
		cache := NewCache(cfg)

		models := []Info{
			{ModelID: "claude-haiku-4.5"},
			{ModelID: "claude-sonnet-4.5"},
		}
		cache.Update(models)

		assert.False(t, cache.IsEmpty())
		assert.Equal(t, 2, cache.Size())
		assert.True(t, cache.IsValidModel("claude-haiku-4.5"))
		assert.True(t, cache.IsValidModel("claude-sonnet-4.5"))
		assert.False(t, cache.IsValidModel("unknown-model"))
	})

	t.Run("replaces cache on update", func(t *testing.T) {
		// Original: test_replaces_cache_on_update
		cfg := newTestConfig()
		cache := NewCache(cfg)

		// First update
		cache.Update([]Info{{ModelID: "model-a"}})
		assert.Equal(t, 1, cache.Size())

		// Second update should replace
		cache.Update([]Info{{ModelID: "model-b"}, {ModelID: "model-c"}})
		assert.Equal(t, 2, cache.Size())
		assert.False(t, cache.IsValidModel("model-a"))
		assert.True(t, cache.IsValidModel("model-b"))
	})

	t.Run("returns all model IDs", func(t *testing.T) {
		// Original: test_returns_all_model_ids
		cfg := newTestConfig()
		cache := NewCache(cfg)

		cache.Update([]Info{
			{ModelID: "model-c"},
			{ModelID: "model-a"},
			{ModelID: "model-b"},
		})

		ids := cache.GetAllModelIDs()
		assert.Len(t, ids, 3)
		// Should be sorted
		assert.Equal(t, "model-a", ids[0])
		assert.Equal(t, "model-b", ids[1])
		assert.Equal(t, "model-c", ids[2])
	})

	t.Run("manages max input tokens", func(t *testing.T) {
		// Original: test_manages_max_input_tokens
		cfg := newTestConfig()
		cache := NewCache(cfg)

		cache.SetMaxInputTokens("claude-haiku-4.5", 150000)
		assert.Equal(t, 150000, cache.GetMaxInputTokens("claude-haiku-4.5"))

		// Unknown model returns default
		assert.Equal(t, 200000, cache.GetMaxInputTokens("unknown-model"))
	})

	t.Run("detects stale cache", func(t *testing.T) {
		// Original: test_detects_stale_cache
		cfg := newTestConfig()
		cfg.ModelCacheTTL = 1 // 1 second
		cache := NewCache(cfg)

		// New cache is stale (never updated)
		assert.True(t, cache.IsStale())

		// After update, not stale
		cache.Update([]Info{{ModelID: "model-a"}})
		assert.False(t, cache.IsStale())

		// Wait for TTL to expire
		time.Sleep(2 * time.Second)
		assert.True(t, cache.IsStale())
	})

	t.Run("adds hidden model", func(t *testing.T) {
		// Original: test_adds_hidden_model
		cfg := newTestConfig()
		cache := NewCache(cfg)

		cache.AddHiddenModel("display-name", "internal-id")

		assert.True(t, cache.IsValidModel("display-name"))
		assert.Equal(t, "internal-id", cache.hiddenModels["display-name"])
	})
}

// =============================================================================
// TestModelResolver
// Original: /code/github/kiro-gateway/tests/unit/test_model_resolver.py::TestModelResolver
// =============================================================================

func TestModelResolver(t *testing.T) {
	t.Run("resolves from cache", func(t *testing.T) {
		// Original: test_resolves_from_cache
		cfg := newTestConfig()
		cache := NewCache(cfg)
		cache.Update([]Info{{ModelID: "claude-haiku-4.5"}})

		resolver := NewResolver(cache, cfg)
		resolution := resolver.Resolve("claude-haiku-4.5")

		assert.Equal(t, "claude-haiku-4.5", resolution.InternalID)
		assert.Equal(t, "cache", resolution.Source)
		assert.True(t, resolution.IsVerified)
	})

	t.Run("resolves from hidden models", func(t *testing.T) {
		// Original: test_resolves_from_hidden_models
		cfg := newTestConfig()
		cfg.HiddenModels = map[string]string{"display-name": "internal-id"}

		cache := NewCache(cfg)
		resolver := NewResolver(cache, cfg)
		resolution := resolver.Resolve("display-name")

		assert.Equal(t, "internal-id", resolution.InternalID)
		assert.Equal(t, "hidden", resolution.Source)
		assert.True(t, resolution.IsVerified)
	})

	t.Run("resolves alias first", func(t *testing.T) {
		// Original: test_resolves_alias_first
		cfg := newTestConfig()
		cfg.ModelAliases = map[string]string{"fast": "claude-haiku-4.5"}

		cache := NewCache(cfg)
		cache.Update([]Info{{ModelID: "claude-haiku-4.5"}})

		resolver := NewResolver(cache, cfg)
		resolution := resolver.Resolve("fast")

		assert.Equal(t, "claude-haiku-4.5", resolution.InternalID)
		assert.Equal(t, "cache", resolution.Source)
		assert.Equal(t, "fast", resolution.OriginalRequest)
	})

	t.Run("passes through unknown model", func(t *testing.T) {
		// Original: test_passes_through_unknown_model
		cfg := newTestConfig()
		cache := NewCache(cfg)

		resolver := NewResolver(cache, cfg)
		resolution := resolver.Resolve("unknown-model")

		assert.Equal(t, "unknown-model", resolution.InternalID)
		assert.Equal(t, "passthrough", resolution.Source)
		assert.False(t, resolution.IsVerified)
	})

	t.Run("normalizes before resolution", func(t *testing.T) {
		// Original: test_normalizes_before_resolution
		cfg := newTestConfig()
		cache := NewCache(cfg)
		cache.Update([]Info{{ModelID: "claude-haiku-4.5"}})

		resolver := NewResolver(cache, cfg)

		// Input is normalized from claude-haiku-4-5 to claude-haiku-4.5
		resolution := resolver.Resolve("claude-haiku-4-5")

		assert.Equal(t, "claude-haiku-4.5", resolution.InternalID)
		assert.Equal(t, "cache", resolution.Source)
		assert.Equal(t, "claude-haiku-4-5", resolution.OriginalRequest)
		assert.Equal(t, "claude-haiku-4.5", resolution.Normalized)
	})

	t.Run("returns available models", func(t *testing.T) {
		// Original: test_returns_available_models
		cfg := newTestConfig()
		cfg.ModelAliases = map[string]string{"fast": "claude-haiku-4.5"}

		cache := NewCache(cfg)
		cache.Update([]Info{
			{ModelID: "claude-haiku-4.5"},
			{ModelID: "claude-sonnet-4.5"},
		})

		resolver := NewResolver(cache, cfg)
		models := resolver.GetAvailableModels()

		assert.Contains(t, models, "claude-haiku-4.5")
		assert.Contains(t, models, "claude-sonnet-4.5")
		assert.Contains(t, models, "fast") // alias
	})

	t.Run("filters hidden from list", func(t *testing.T) {
		// Original: test_filters_hidden_from_list
		cfg := newTestConfig()
		cfg.HiddenFromList = []string{"hidden-model"}

		cache := NewCache(cfg)
		cache.Update([]Info{
			{ModelID: "visible-model"},
			{ModelID: "hidden-model"},
		})

		resolver := NewResolver(cache, cfg)
		models := resolver.GetAvailableModels()

		assert.Contains(t, models, "visible-model")
		assert.NotContains(t, models, "hidden-model")
	})

	t.Run("gets models by family", func(t *testing.T) {
		// Original: test_gets_models_by_family
		cfg := newTestConfig()
		cache := NewCache(cfg)
		cache.Update([]Info{
			{ModelID: "claude-haiku-4.5"},
			{ModelID: "claude-sonnet-4.5"},
			{ModelID: "claude-opus-4"},
		})

		resolver := NewResolver(cache, cfg)

		haikuModels := resolver.GetModelsByFamily("haiku")
		assert.Len(t, haikuModels, 1)
		assert.Contains(t, haikuModels, "claude-haiku-4.5")

		sonnetModels := resolver.GetModelsByFamily("sonnet")
		assert.Len(t, sonnetModels, 1)
	})

	t.Run("gets suggestions for model", func(t *testing.T) {
		// Original: test_gets_suggestions_for_model
		cfg := newTestConfig()
		cache := NewCache(cfg)
		cache.Update([]Info{
			{ModelID: "claude-haiku-4.5"},
			{ModelID: "claude-haiku-4"},
		})

		resolver := NewResolver(cache, cfg)
		suggestions := resolver.GetSuggestionsForModel("claude-haiku-unknown")

		// Should return haiku family models
		assert.Contains(t, suggestions, "claude-haiku-4.5")
		assert.Contains(t, suggestions, "claude-haiku-4")
	})
}

// =============================================================================
// TestGetModelIDForKiro
// Tests for GetModelIDForKiro helper function
// =============================================================================

func TestGetModelIDForKiro(t *testing.T) {
	t.Run("returns hidden model internal id", func(t *testing.T) {
		hiddenModels := map[string]string{"display-name": "internal-id"}
		result := GetModelIDForKiro("display-name", hiddenModels)
		assert.Equal(t, "internal-id", result)
	})

	t.Run("returns normalized name if not hidden", func(t *testing.T) {
		hiddenModels := map[string]string{}
		result := GetModelIDForKiro("claude-haiku-4-5", hiddenModels)
		assert.Equal(t, "claude-haiku-4.5", result)
	})
}
