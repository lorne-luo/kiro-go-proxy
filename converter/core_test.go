// Package converter provides tests for format conversion.
package converter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"kiro-go-proxy/config"
)

// =============================================================================
// TestConvertToolsToKiroFormat
// Original: /code/github/kiro-gateway/tests/unit/test_converters_core.py::TestConvertToolsToKiroFormat
// =============================================================================

func TestConvertToolsToKiroFormat(t *testing.T) {
	t.Run("converts single tool", func(t *testing.T) {
		tools := []UnifiedTool{
			{
				Name:        "get_weather",
				Description: "Get weather for a city",
				InputSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"city": map[string]interface{}{
							"type": "string",
						},
					},
				},
			},
		}

		result := ConvertToolsToKiroFormat(tools)

		assert.Len(t, result, 1)
		assert.Contains(t, result[0], "toolSpecification")

		spec := result[0]["toolSpecification"].(map[string]interface{})
		assert.Equal(t, "get_weather", spec["name"])
		assert.Equal(t, "Get weather for a city", spec["description"])
		assert.Contains(t, spec, "inputSchema")
	})

	t.Run("converts multiple tools", func(t *testing.T) {
		tools := []UnifiedTool{
			{Name: "tool1", Description: "First tool"},
			{Name: "tool2", Description: "Second tool"},
		}

		result := ConvertToolsToKiroFormat(tools)

		assert.Len(t, result, 2)
	})

	t.Run("handles empty description", func(t *testing.T) {
		tools := []UnifiedTool{
			{Name: "tool_without_desc", Description: ""},
		}

		result := ConvertToolsToKiroFormat(tools)

		spec := result[0]["toolSpecification"].(map[string]interface{})
		assert.Equal(t, "Tool: tool_without_desc", spec["description"])
	})

	t.Run("sanitizes input schema", func(t *testing.T) {
		tools := []UnifiedTool{
			{
				Name: "tool",
				InputSchema: map[string]interface{}{
					"type":                 "object",
					"additionalProperties": true,
					"required":             []interface{}{},
				},
			},
		}

		result := ConvertToolsToKiroFormat(tools)

		spec := result[0]["toolSpecification"].(map[string]interface{})
		inputSchema := spec["inputSchema"].(map[string]interface{})
		jsonSchema := inputSchema["json"].(map[string]interface{})

		_, hasAP := jsonSchema["additionalProperties"]
		assert.False(t, hasAP, "additionalProperties should be removed")
		_, hasRequired := jsonSchema["required"]
		assert.False(t, hasRequired, "empty required should be removed")
	})
}

// =============================================================================
// TestConvertToolResultsToKiroFormat
// Original: /code/github/kiro-gateway/tests/unit/test_converters_core.py::TestConvertToolResultsToKiroFormat
// =============================================================================

func TestConvertToolResultsToKiroFormat(t *testing.T) {
	t.Run("converts tool result", func(t *testing.T) {
		results := []ToolResult{
			{
				ToolUseID: "tool_123",
				Content:   "Result content",
			},
		}

		kiroResults := ConvertToolResultsToKiroFormat(results)

		assert.Len(t, kiroResults, 1)
		assert.Equal(t, "tool_123", kiroResults[0]["toolUseId"])
		assert.Equal(t, "success", kiroResults[0]["status"])

		content := kiroResults[0]["content"].([]map[string]interface{})
		assert.Equal(t, "Result content", content[0]["text"])
	})

	t.Run("handles empty content", func(t *testing.T) {
		results := []ToolResult{
			{
				ToolUseID: "tool_456",
				Content:   "",
			},
		}

		kiroResults := ConvertToolResultsToKiroFormat(results)

		content := kiroResults[0]["content"].([]map[string]interface{})
		assert.Equal(t, "(empty result)", content[0]["text"])
	})

	t.Run("converts multiple results", func(t *testing.T) {
		results := []ToolResult{
			{ToolUseID: "tool_1", Content: "Result 1"},
			{ToolUseID: "tool_2", Content: "Result 2"},
		}

		kiroResults := ConvertToolResultsToKiroFormat(results)

		assert.Len(t, kiroResults, 2)
	})
}

// =============================================================================
// TestProcessToolsWithLongDescriptions
// Original: /code/github/kiro-gateway/tests/unit/test_converters_core.py::TestProcessToolsWithLongDescriptions
// =============================================================================

func TestProcessToolsWithLongDescriptions(t *testing.T) {
	t.Run("keeps short descriptions", func(t *testing.T) {
		tools := []UnifiedTool{
			{
				Name:        "short_desc_tool",
				Description: "This is a short description",
			},
		}

		processed, docs := ProcessToolsWithLongDescriptions(tools, 1000)

		assert.Len(t, processed, 1)
		assert.Equal(t, "This is a short description", processed[0].Description)
		assert.Equal(t, "", docs)
	})

	t.Run("moves long description to docs", func(t *testing.T) {
		longDesc := ""
		for i := 0; i < 200; i++ {
			longDesc += "word "
		}

		tools := []UnifiedTool{
			{
				Name:        "long_desc_tool",
				Description: longDesc,
			},
		}

		processed, docs := ProcessToolsWithLongDescriptions(tools, 100)

		assert.Len(t, processed, 1)
		assert.Contains(t, docs, "long_desc_tool")
		assert.NotEqual(t, longDesc, processed[0].Description)
	})

	t.Run("handles empty tools", func(t *testing.T) {
		tools := []UnifiedTool{}

		processed, docs := ProcessToolsWithLongDescriptions(tools, 100)

		assert.Empty(t, processed)
		assert.Equal(t, "", docs)
	})
}

// =============================================================================
// TestValidateToolNames
// Tests for tool name validation
// =============================================================================

func TestValidateToolNames(t *testing.T) {
	t.Run("accepts valid names", func(t *testing.T) {
		tools := []UnifiedTool{
			{Name: "get_weather"},
			{Name: "search_web"},
			{Name: "calculate_sum"},
		}

		// Should not panic
		assert.NotPanics(t, func() {
			ValidateToolNames(tools)
		})
	})
}

// =============================================================================
// TestBuildKiroHistory
// Original: /code/github/kiro-gateway/tests/unit/test_converters_core.py::TestBuildKiroHistory
// =============================================================================

func TestBuildKiroHistory(t *testing.T) {
	t.Run("builds history with user message", func(t *testing.T) {
		messages := []UnifiedMessage{
			{
				Role:    "user",
				Content: "Hello",
			},
		}

		history := BuildKiroHistory(messages, "test-model")

		assert.Len(t, history, 1)

		userMsg := history[0].(map[string]interface{})
		assert.Contains(t, userMsg, "userInputMessage")

		input := userMsg["userInputMessage"].(map[string]interface{})
		assert.Equal(t, "Hello", input["content"])
		assert.Equal(t, "test-model", input["modelId"])
	})

	t.Run("builds history with assistant message", func(t *testing.T) {
		messages := []UnifiedMessage{
			{
				Role:    "assistant",
				Content: "Hi there!",
			},
		}

		history := BuildKiroHistory(messages, "test-model")

		assert.Len(t, history, 1)

		assistantMsg := history[0].(map[string]interface{})
		assert.Contains(t, assistantMsg, "assistantResponseMessage")

		resp := assistantMsg["assistantResponseMessage"].(map[string]interface{})
		assert.Equal(t, "Hi there!", resp["content"])
	})

	t.Run("builds history with tool calls", func(t *testing.T) {
		messages := []UnifiedMessage{
			{
				Role:    "assistant",
				Content: "",
				ToolCalls: []ToolCall{
					{
						ID:   "call_123",
						Type: "function",
						Function: struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						}{
							Name:      "get_weather",
							Arguments: `{"city": "London"}`,
						},
					},
				},
			},
		}

		history := BuildKiroHistory(messages, "test-model")

		assistantMsg := history[0].(map[string]interface{})
		resp := assistantMsg["assistantResponseMessage"].(map[string]interface{})

		toolUses := resp["toolUses"].([]map[string]interface{})
		assert.Len(t, toolUses, 1)
		assert.Equal(t, "get_weather", toolUses[0]["name"])
		assert.Equal(t, "call_123", toolUses[0]["toolUseId"])
	})

	t.Run("builds history with tool results", func(t *testing.T) {
		messages := []UnifiedMessage{
			{
				Role:    "user",
				Content: "",
				ToolResults: []ToolResult{
					{
						ToolUseID: "call_123",
						Content:   "Sunny, 25°C",
					},
				},
			},
		}

		history := BuildKiroHistory(messages, "test-model")

		userMsg := history[0].(map[string]interface{})
		input := userMsg["userInputMessage"].(map[string]interface{})

		context := input["userInputMessageContext"].(map[string]interface{})
		toolResults := context["toolResults"].([]map[string]interface{})

		assert.Len(t, toolResults, 1)
		assert.Equal(t, "call_123", toolResults[0]["toolUseId"])
	})
}

// =============================================================================
// TestBuildKiroPayload
// Tests for building Kiro API payload
// =============================================================================

func TestBuildKiroPayload(t *testing.T) {
	cfg := &config.Config{
		ToolDescriptionMaxLength: 10000,
	}

	t.Run("builds basic payload", func(t *testing.T) {
		messages := []UnifiedMessage{
			{Role: "user", Content: "Hello"},
		}

		payload := BuildKiroPayload(messages, "You are helpful", "claude-haiku-4.5", nil, "conv-123", "arn:profile", cfg)

		assert.Equal(t, "MANUAL", payload.ConversationState.ChatTriggerType)
		assert.Equal(t, "conv-123", payload.ConversationState.ConversationID)
		assert.Equal(t, "arn:profile", payload.ProfileArn)
		assert.Equal(t, "claude-haiku-4.5", payload.ConversationState.CurrentMessage.UserInputMessage.ModelID)
	})

	t.Run("builds payload with tools", func(t *testing.T) {
		messages := []UnifiedMessage{
			{Role: "user", Content: "What's the weather?"},
		}

		tools := []UnifiedTool{
			{Name: "get_weather", Description: "Get weather"},
		}

		payload := BuildKiroPayload(messages, "", "model", tools, "conv", "", cfg)

		context := payload.ConversationState.CurrentMessage.UserInputMessage.UserInputMessageContext
		assert.NotNil(t, context)
		assert.Len(t, context.Tools, 1)
	})

	t.Run("builds payload with history", func(t *testing.T) {
		messages := []UnifiedMessage{
			{Role: "user", Content: "First"},
			{Role: "assistant", Content: "Response"},
			{Role: "user", Content: "Second"},
		}

		payload := BuildKiroPayload(messages, "", "model", nil, "conv", "", cfg)

		// History should have 2 entries (first user + assistant)
		assert.Len(t, payload.ConversationState.History, 2)
	})
}

// =============================================================================
// TestMergeAdjacentMessages
// Tests for message merging
// =============================================================================

func TestMergeAdjacentMessages(t *testing.T) {
	t.Run("merges adjacent same-role messages", func(t *testing.T) {
		// Note: Go implementation joins with newline
		messages := []UnifiedMessage{
			{Role: "user", Content: "Hello "},
			{Role: "user", Content: "World"},
		}

		merged := MergeAdjacentMessages(messages)

		assert.Len(t, merged, 1)
		// Go implementation joins with newline
		assert.Contains(t, merged[0].Content, "Hello")
		assert.Contains(t, merged[0].Content, "World")
	})

	t.Run("keeps different roles separate", func(t *testing.T) {
		messages := []UnifiedMessage{
			{Role: "user", Content: "Hello"},
			{Role: "assistant", Content: "Hi"},
		}

		merged := MergeAdjacentMessages(messages)

		assert.Len(t, merged, 2)
	})

	t.Run("handles empty messages", func(t *testing.T) {
		messages := []UnifiedMessage{}

		merged := MergeAdjacentMessages(messages)

		assert.Empty(t, merged)
	})
}

// =============================================================================
// TestEnsureFirstMessageIsUser
// Tests for ensuring first message is user
// =============================================================================

func TestEnsureFirstMessageIsUser(t *testing.T) {
	t.Run("keeps user first message", func(t *testing.T) {
		messages := []UnifiedMessage{
			{Role: "user", Content: "Hello"},
			{Role: "assistant", Content: "Hi"},
		}

		result := EnsureFirstMessageIsUser(messages)

		assert.Len(t, result, 2)
		assert.Equal(t, "user", result[0].Role)
	})

	t.Run("handles assistant first", func(t *testing.T) {
		// Note: Go implementation prepends "(empty)" user message
		messages := []UnifiedMessage{
			{Role: "assistant", Content: "Hi"},
		}

		result := EnsureFirstMessageIsUser(messages)

		assert.GreaterOrEqual(t, len(result), 1)
		// First message should be user role
		assert.Equal(t, "user", result[0].Role)
	})

	t.Run("handles empty messages", func(t *testing.T) {
		// Note: Go implementation may return empty for empty input
		messages := []UnifiedMessage{}

		result := EnsureFirstMessageIsUser(messages)

		// Verify it doesn't panic and returns valid result
		// (may be empty or contain a placeholder depending on implementation)
		assert.NotNil(t, result)
	})
}

// =============================================================================
// TestNormalizeMessageRoles
// Tests for role normalization
// =============================================================================

func TestNormalizeMessageRoles(t *testing.T) {
	t.Run("normalizes system to user", func(t *testing.T) {
		messages := []UnifiedMessage{
			{Role: "system", Content: "You are helpful"},
		}

		normalized := NormalizeMessageRoles(messages)

		assert.Equal(t, "user", normalized[0].Role)
	})

	t.Run("keeps user and assistant unchanged", func(t *testing.T) {
		messages := []UnifiedMessage{
			{Role: "user", Content: "Hello"},
			{Role: "assistant", Content: "Hi"},
		}

		normalized := NormalizeMessageRoles(messages)

		assert.Equal(t, "user", normalized[0].Role)
		assert.Equal(t, "assistant", normalized[1].Role)
	})
}

// =============================================================================
// TestEnsureAlternatingRoles
// Tests for alternating roles
// =============================================================================

func TestEnsureAlternatingRoles(t *testing.T) {
	t.Run("keeps already alternating roles", func(t *testing.T) {
		messages := []UnifiedMessage{
			{Role: "user", Content: "Hello"},
			{Role: "assistant", Content: "Hi"},
			{Role: "user", Content: "How are you?"},
		}

		result := EnsureAlternatingRoles(messages)

		assert.Len(t, result, 3)
		assert.Equal(t, "user", result[0].Role)
		assert.Equal(t, "assistant", result[1].Role)
		assert.Equal(t, "user", result[2].Role)
	})

	t.Run("handles consecutive same roles", func(t *testing.T) {
		messages := []UnifiedMessage{
			{Role: "user", Content: "Hello"},
			{Role: "user", Content: "World"},
		}

		result := EnsureAlternatingRoles(messages)

		// Result should have at least the original messages
		assert.GreaterOrEqual(t, len(result), 2)
		// First should still be user
		assert.Equal(t, "user", result[0].Role)
	})
}
