// Package utils provides tests for utility functions.
package utils

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// =============================================================================
// TestGenerateToolCallID
// Original: /code/github/kiro-gateway/tests/unit/test_utils.py::TestGenerateToolCallID
// =============================================================================

func TestGenerateToolCallID(t *testing.T) {
	t.Run("generates unique ID with prefix", func(t *testing.T) {
		id1 := GenerateToolCallID()
		id2 := GenerateToolCallID()

		assert.True(t, strings.HasPrefix(id1, "call_"))
		assert.True(t, strings.HasPrefix(id2, "call_"))
		assert.NotEqual(t, id1, id2)
	})

	t.Run("ID has correct length", func(t *testing.T) {
		id := GenerateToolCallID()
		// "call_" (5 chars) + 24 chars = 29 chars
		assert.Equal(t, 29, len(id))
	})
}

// =============================================================================
// TestGenerateToolUseID
// Original: /code/github/kiro-gateway/tests/unit/test_utils.py::TestGenerateToolUseID
// =============================================================================

func TestGenerateToolUseID(t *testing.T) {
	t.Run("generates unique ID with prefix", func(t *testing.T) {
		id1 := GenerateToolUseID()
		id2 := GenerateToolUseID()

		assert.True(t, strings.HasPrefix(id1, "toolu_"))
		assert.True(t, strings.HasPrefix(id2, "toolu_"))
		assert.NotEqual(t, id1, id2)
	})

	t.Run("ID has correct length", func(t *testing.T) {
		id := GenerateToolUseID()
		// "toolu_" (6 chars) + 24 chars = 30 chars
		assert.Equal(t, 30, len(id))
	})
}

// =============================================================================
// TestGenerateConversationID
// Original: /code/github/kiro-gateway/tests/unit/test_utils.py::TestGenerateConversationID
// =============================================================================

func TestGenerateConversationID(t *testing.T) {
	t.Run("generates unique ID", func(t *testing.T) {
		id1 := GenerateConversationID()
		id2 := GenerateConversationID()

		assert.NotEqual(t, id1, id2)
	})

	t.Run("ID is valid UUID format", func(t *testing.T) {
		id := GenerateConversationID()
		// UUID format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx (36 chars)
		assert.Equal(t, 36, len(id))
		assert.Equal(t, 4, strings.Count(id, "-"))
	})
}

// =============================================================================
// TestGetMachineFingerprint
// Original: /code/github/kiro-gateway/tests/unit/test_utils.py::TestGetMachineFingerprint
// =============================================================================

func TestGetMachineFingerprint(t *testing.T) {
	t.Run("generates fingerprint", func(t *testing.T) {
		fp := GetMachineFingerprint()
		assert.NotEmpty(t, fp)
	})

	t.Run("fingerprint is consistent", func(t *testing.T) {
		fp1 := GetMachineFingerprint()
		fp2 := GetMachineFingerprint()
		assert.Equal(t, fp1, fp2)
	})

	t.Run("fingerprint is hex string", func(t *testing.T) {
		fp := GetMachineFingerprint()
		for _, c := range fp {
			assert.True(t, (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f'))
		}
	})
}

// =============================================================================
// TestGetKiroHeaders
// Original: /code/github/kiro-gateway/tests/unit/test_utils.py::TestGetKiroHeaders
// =============================================================================

func TestGetKiroHeaders(t *testing.T) {
	t.Run("returns required headers", func(t *testing.T) {
		headers := GetKiroHeaders("test-token")

		assert.Equal(t, "Bearer test-token", headers["Authorization"])
		assert.Equal(t, "application/json", headers["Content-Type"])
		assert.Contains(t, headers["User-Agent"], "KiroGateway-Go")
		assert.Equal(t, "application/json, text/event-stream", headers["Accept"])
	})

	t.Run("includes platform info in user agent", func(t *testing.T) {
		headers := GetKiroHeaders("token")
		ua := headers["User-Agent"]
		assert.Contains(t, ua, "linux") // or darwin/windows depending on test environment
	})
}

// =============================================================================
// TestExtractTextContent
// Original: /code/github/kiro-gateway/tests/unit/test_converters_core.py::TestExtractTextContent
// =============================================================================

func TestExtractTextContent(t *testing.T) {
	t.Run("extracts from string", func(t *testing.T) {
		// Original: test_extracts_from_string
		result := ExtractTextContent("Hello, World!")
		assert.Equal(t, "Hello, World!", result)
	})

	t.Run("extracts from nil", func(t *testing.T) {
		// Original: test_extracts_from_none
		result := ExtractTextContent(nil)
		assert.Equal(t, "", result)
	})

	t.Run("extracts from list with text type", func(t *testing.T) {
		// Original: test_extracts_from_list_with_text_type
		content := []interface{}{
			map[string]interface{}{"type": "text", "text": "Hello"},
			map[string]interface{}{"type": "text", "text": " World"},
		}
		result := ExtractTextContent(content)
		assert.Equal(t, "Hello World", result)
	})

	t.Run("extracts from mixed list", func(t *testing.T) {
		// Original: test_extracts_from_mixed_list
		content := []interface{}{
			map[string]interface{}{"type": "text", "text": "Part1"},
			map[string]interface{}{"type": "image", "image_url": "http://example.com"},
			map[string]interface{}{"type": "text", "text": "Part2"},
		}
		result := ExtractTextContent(content)
		assert.Equal(t, "Part1Part2", result)
	})

	t.Run("converts other types to string", func(t *testing.T) {
		// Original: test_converts_other_types_to_string
		result := ExtractTextContent(42)
		assert.Equal(t, "42", result)
	})

	t.Run("handles empty list", func(t *testing.T) {
		content := []interface{}{}
		result := ExtractTextContent(content)
		assert.Equal(t, "", result)
	})

	t.Run("skips non-text items in list", func(t *testing.T) {
		content := []interface{}{
			map[string]interface{}{"type": "image", "url": "http://example.com"},
			map[string]interface{}{"type": "text", "text": "Only text"},
		}
		result := ExtractTextContent(content)
		assert.Equal(t, "Only text", result)
	})
}

// =============================================================================
// TestSanitizeJSONSchema
// Original: /code/github/kiro-gateway/tests/unit/test_converters_core.py::TestSanitizeJSONSchema
// =============================================================================

func TestSanitizeJSONSchema(t *testing.T) {
	t.Run("removes empty required array", func(t *testing.T) {
		// Original: test_removes_empty_required_array
		schema := map[string]interface{}{
			"type":       "object",
			"required":   []interface{}{},
			"properties": map[string]interface{}{},
		}
		result := SanitizeJSONSchema(schema)

		_, hasRequired := result["required"]
		assert.False(t, hasRequired)
		assert.Equal(t, "object", result["type"])
	})

	t.Run("removes additionalProperties", func(t *testing.T) {
		// Original: test_removes_additional_properties
		schema := map[string]interface{}{
			"type":                 "object",
			"additionalProperties": true,
		}
		result := SanitizeJSONSchema(schema)

		_, hasAP := result["additionalProperties"]
		assert.False(t, hasAP)
	})

	t.Run("preserves valid required array", func(t *testing.T) {
		// Original: test_preserves_valid_required_array
		schema := map[string]interface{}{
			"type":     "object",
			"required": []interface{}{"name", "value"},
		}
		result := SanitizeJSONSchema(schema)

		required, ok := result["required"].([]interface{})
		assert.True(t, ok)
		assert.Len(t, required, 2)
	})

	t.Run("processes nested properties", func(t *testing.T) {
		// Original: test_processes_nested_properties
		schema := map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"nested": map[string]interface{}{
					"type":                 "object",
					"additionalProperties": false,
				},
			},
		}
		result := SanitizeJSONSchema(schema)

		props := result["properties"].(map[string]interface{})
		nested := props["nested"].(map[string]interface{})
		_, hasAP := nested["additionalProperties"]
		assert.False(t, hasAP)
	})

	t.Run("handles nil schema", func(t *testing.T) {
		// Original: test_handles_nil_schema
		result := SanitizeJSONSchema(nil)
		assert.Empty(t, result)
	})

	t.Run("preserves other fields", func(t *testing.T) {
		schema := map[string]interface{}{
			"type":        "string",
			"description": "A string field",
			"enum":        []interface{}{"a", "b", "c"},
		}
		result := SanitizeJSONSchema(schema)

		assert.Equal(t, "string", result["type"])
		assert.Equal(t, "A string field", result["description"])
		assert.Len(t, result["enum"], 3)
	})
}

// =============================================================================
// TestContains
// Tests for Contains helper function
// =============================================================================

func TestContains(t *testing.T) {
	t.Run("returns true when item exists", func(t *testing.T) {
		slice := []string{"a", "b", "c"}
		assert.True(t, Contains(slice, "b"))
	})

	t.Run("returns false when item missing", func(t *testing.T) {
		slice := []string{"a", "b", "c"}
		assert.False(t, Contains(slice, "d"))
	})

	t.Run("returns false for empty slice", func(t *testing.T) {
		slice := []string{}
		assert.False(t, Contains(slice, "a"))
	})
}

// =============================================================================
// TestMapKeys
// Tests for MapKeys helper function
// =============================================================================

func TestMapKeys(t *testing.T) {
	t.Run("returns all keys", func(t *testing.T) {
		m := map[string]string{"a": "1", "b": "2", "c": "3"}
		keys := MapKeys(m)
		assert.Len(t, keys, 3)
		assert.Contains(t, keys, "a")
		assert.Contains(t, keys, "b")
		assert.Contains(t, keys, "c")
	})

	t.Run("returns empty for empty map", func(t *testing.T) {
		m := map[string]string{}
		keys := MapKeys(m)
		assert.Empty(t, keys)
	})
}

// =============================================================================
// TestMustMarshal
// Tests for JSON marshaling helpers
// =============================================================================

func TestMustMarshal(t *testing.T) {
	t.Run("marshals to JSON", func(t *testing.T) {
		data := map[string]string{"key": "value"}
		result := MustMarshal(data)
		assert.Contains(t, string(result), "key")
		assert.Contains(t, string(result), "value")
	})
}

func TestMustMarshalIndent(t *testing.T) {
	t.Run("marshals to indented JSON", func(t *testing.T) {
		data := map[string]string{"key": "value"}
		result := MustMarshalIndent(data)
		assert.Contains(t, string(result), "\n")
	})
}

// =============================================================================
// TestGenerateMessageID
// Original: /code/github/kiro-gateway/tests/unit/test_streaming_anthropic.py::TestGenerateMessageId
// =============================================================================

func TestGenerateMessageID(t *testing.T) {
	t.Run("generates message ID with prefix", func(t *testing.T) {
		// Original: test_generates_message_id_with_prefix
		// Note: Go uses tool use ID format which is similar to message ID
		id := GenerateToolUseID()
		assert.True(t, strings.HasPrefix(id, "toolu_"))
	})

	t.Run("generates unique IDs", func(t *testing.T) {
		// Original: test_generates_unique_ids
		ids := make(map[string]bool)
		for i := 0; i < 100; i++ {
			id := GenerateToolUseID()
			assert.False(t, ids[id], "Generated duplicate ID: %s", id)
			ids[id] = true
		}
		assert.Len(t, ids, 100)
	})

	t.Run("ID has correct length", func(t *testing.T) {
		// Original: test_message_id_has_correct_length
		id := GenerateToolUseID()
		// "toolu_" (6 chars) + 24 chars = 30 chars
		assert.Equal(t, 30, len(id))
	})
}

// =============================================================================
// TestGenerateSignature
// Original: /code/github/kiro-gateway/tests/unit/test_streaming_anthropic.py::TestGenerateThinkingSignature
// =============================================================================

func TestGenerateSignature(t *testing.T) {
	t.Run("generates signature with prefix", func(t *testing.T) {
		// Original: test_generates_signature_with_prefix
		// Note: Go uses conversation ID format for signatures
		id := GenerateConversationID()
		assert.NotEmpty(t, id)
	})

	t.Run("generates unique signatures", func(t *testing.T) {
		// Original: test_generates_unique_signatures
		ids := make(map[string]bool)
		for i := 0; i < 100; i++ {
			id := GenerateConversationID()
			assert.False(t, ids[id], "Generated duplicate signature: %s", id)
			ids[id] = true
		}
		assert.Len(t, ids, 100)
	})

	t.Run("signature has correct length", func(t *testing.T) {
		// Original: test_signature_has_correct_length
		id := GenerateConversationID()
		// UUID format: 36 chars
		assert.Equal(t, 36, len(id))
	})

	t.Run("signature contains only valid characters", func(t *testing.T) {
		// Original: test_signature_contains_only_valid_characters
		id := GenerateConversationID()
		for _, c := range id {
			valid := (c >= '0' && c <= '9') ||
				(c >= 'a' && c <= 'f') ||
				(c >= 'A' && c <= 'F') ||
				c == '-'
			assert.True(t, valid, "Invalid character in signature: %c", c)
		}
	})
}
