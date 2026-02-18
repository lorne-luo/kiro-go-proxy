// Package converter provides tests for OpenAI format conversion.
package converter

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// =============================================================================
// TestConvertOpenAIToUnified
// Original: /code/github/kiro-gateway/tests/unit/test_converters_openai.py::TestConvertOpenAIToUnified
// =============================================================================

func TestConvertOpenAIToUnified(t *testing.T) {
	t.Run("converts user message", func(t *testing.T) {
		messages := []OpenAIMessage{
			{Role: "user", Content: "Hello"},
		}

		unified, systemPrompt := ConvertOpenAIToUnified(messages)

		assert.Len(t, unified, 1)
		assert.Equal(t, "user", unified[0].Role)
		assert.Equal(t, "Hello", unified[0].Content)
		assert.Equal(t, "", systemPrompt)
	})

	t.Run("extracts system prompt", func(t *testing.T) {
		messages := []OpenAIMessage{
			{Role: "system", Content: "You are helpful"},
			{Role: "user", Content: "Hello"},
		}

		unified, systemPrompt := ConvertOpenAIToUnified(messages)

		assert.Equal(t, "You are helpful", systemPrompt)
		assert.Len(t, unified, 1)
	})

	t.Run("converts assistant message", func(t *testing.T) {
		messages := []OpenAIMessage{
			{Role: "user", Content: "Hello"},
			{Role: "assistant", Content: "Hi there!"},
		}

		unified, _ := ConvertOpenAIToUnified(messages)

		assert.Len(t, unified, 2)
		assert.Equal(t, "assistant", unified[1].Role)
		assert.Equal(t, "Hi there!", unified[1].Content)
	})

	t.Run("converts tool calls", func(t *testing.T) {
		messages := []OpenAIMessage{
			{
				Role:    "assistant",
				Content: "",
				ToolCalls: []OpenAIToolCall{
					{
						ID:   "call_123",
						Type: "function",
						Function: OpenAIFunction{
							Name:      "get_weather",
							Arguments: `{"city": "London"}`,
						},
					},
				},
			},
		}

		unified, _ := ConvertOpenAIToUnified(messages)

		assert.Len(t, unified, 1)
		assert.Len(t, unified[0].ToolCalls, 1)
		assert.Equal(t, "call_123", unified[0].ToolCalls[0].ID)
		assert.Equal(t, "get_weather", unified[0].ToolCalls[0].Function.Name)
	})

	t.Run("converts tool result", func(t *testing.T) {
		messages := []OpenAIMessage{
			{Role: "user", Content: "What's the weather?"},
			{
				Role:       "tool",
				Content:    "Sunny, 25°C",
				ToolCallID: "call_123",
			},
		}

		unified, _ := ConvertOpenAIToUnified(messages)

		assert.Len(t, unified, 1)
		assert.Len(t, unified[0].ToolResults, 1)
		assert.Equal(t, "call_123", unified[0].ToolResults[0].ToolUseID)
	})

	t.Run("handles multiple messages", func(t *testing.T) {
		messages := []OpenAIMessage{
			{Role: "system", Content: "Be helpful"},
			{Role: "user", Content: "Q1"},
			{Role: "assistant", Content: "A1"},
			{Role: "user", Content: "Q2"},
			{Role: "assistant", Content: "A2"},
		}

		unified, systemPrompt := ConvertOpenAIToUnified(messages)

		assert.Equal(t, "Be helpful", systemPrompt)
		assert.Len(t, unified, 4) // system not in unified
	})
}

// =============================================================================
// TestConvertOpenAIToolsToUnified
// Original: /code/github/kiro-gateway/tests/unit/test_converters_openai.py::TestConvertOpenAIToolsToUnified
// =============================================================================

func TestConvertOpenAIToolsToUnified(t *testing.T) {
	t.Run("converts function tools", func(t *testing.T) {
		tools := []OpenAITool{
			{
				Type: "function",
				Function: OpenAIFunctionDef{
					Name:        "get_weather",
					Description: "Get weather info",
					Parameters: map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"city": map[string]interface{}{"type": "string"},
						},
					},
				},
			},
		}

		unified := ConvertOpenAIToolsToUnified(tools)

		assert.Len(t, unified, 1)
		assert.Equal(t, "get_weather", unified[0].Name)
		assert.Equal(t, "Get weather info", unified[0].Description)
		assert.Contains(t, unified[0].InputSchema, "type")
	})

	t.Run("converts multiple tools", func(t *testing.T) {
		tools := []OpenAITool{
			{Type: "function", Function: OpenAIFunctionDef{Name: "tool1"}},
			{Type: "function", Function: OpenAIFunctionDef{Name: "tool2"}},
		}

		unified := ConvertOpenAIToolsToUnified(tools)

		assert.Len(t, unified, 2)
	})

	t.Run("skips non-function tools", func(t *testing.T) {
		tools := []OpenAITool{
			{Type: "code_interpreter"},
			{Type: "function", Function: OpenAIFunctionDef{Name: "func"}},
		}

		unified := ConvertOpenAIToolsToUnified(tools)

		assert.Len(t, unified, 1)
		assert.Equal(t, "func", unified[0].Name)
	})

	t.Run("handles empty tools", func(t *testing.T) {
		tools := []OpenAITool{}

		unified := ConvertOpenAIToolsToUnified(tools)

		assert.Empty(t, unified)
	})
}

// =============================================================================
// TestExtractImagesFromOpenAIContent
// Original: /code/github/kiro-gateway/tests/unit/test_converters_openai.py::TestExtractImages
// =============================================================================

func TestExtractImagesFromOpenAIContent(t *testing.T) {
	t.Run("extracts data URL images", func(t *testing.T) {
		content := []interface{}{
			map[string]interface{}{"type": "text", "text": "Check this image:"},
			map[string]interface{}{
				"type": "image_url",
				"image_url": map[string]interface{}{
					"url": "data:image/png;base64,iVBORw0KGgo=",
				},
			},
		}

		images := ExtractImagesFromOpenAIContent(content)

		assert.Len(t, images, 1)
		assert.Equal(t, "image/png", images[0]["media_type"])
		assert.Equal(t, "iVBORw0KGgo=", images[0]["data"])
	})

	t.Run("handles string content", func(t *testing.T) {
		content := "Just text"

		images := ExtractImagesFromOpenAIContent(content)

		assert.Empty(t, images)
	})

	t.Run("skips non-image items", func(t *testing.T) {
		content := []interface{}{
			map[string]interface{}{"type": "text", "text": "Hello"},
			map[string]interface{}{"type": "other"},
		}

		images := ExtractImagesFromOpenAIContent(content)

		assert.Empty(t, images)
	})

	t.Run("extracts jpeg images", func(t *testing.T) {
		content := []interface{}{
			map[string]interface{}{
				"type": "image_url",
				"image_url": map[string]interface{}{
					"url": "data:image/jpeg;base64,/9j/4AAQSkZJ",
				},
			},
		}

		images := ExtractImagesFromOpenAIContent(content)

		assert.Len(t, images, 1)
		assert.Equal(t, "image/jpeg", images[0]["media_type"])
	})
}

// =============================================================================
// TestCreateOpenAIResponse
// Tests for creating OpenAI responses
// =============================================================================

func TestCreateOpenAIResponse(t *testing.T) {
	t.Run("creates response with content", func(t *testing.T) {
		response := CreateOpenAIResponse("msg_123", "claude-haiku-4.5", "Hello!", nil, "stop", nil)

		assert.Equal(t, "msg_123", response.ID)
		assert.Equal(t, "chat.completion", response.Object)
		assert.Equal(t, "claude-haiku-4.5", response.Model)
		assert.Len(t, response.Choices, 1)
		assert.Equal(t, "Hello!", response.Choices[0].Message.Content)
		assert.Equal(t, "stop", response.Choices[0].FinishReason)
	})

	t.Run("creates response with tool calls", func(t *testing.T) {
		toolCalls := []ToolCall{
			{
				ID:   "call_123",
				Type: "function",
				Function: struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				}{
					Name:      "get_weather",
					Arguments: `{"city": "Paris"}`,
				},
			},
		}

		response := CreateOpenAIResponse("msg_123", "model", "", toolCalls, "tool_calls", nil)

		assert.Len(t, response.Choices[0].Message.ToolCalls, 1)
		assert.Equal(t, "tool_calls", response.Choices[0].FinishReason)
	})

	t.Run("creates response with usage", func(t *testing.T) {
		usage := &OpenAIUsage{
			PromptTokens:     100,
			CompletionTokens: 50,
			TotalTokens:      150,
		}

		response := CreateOpenAIResponse("msg_123", "model", "Hi", nil, "stop", usage)

		assert.NotNil(t, response.Usage)
		assert.Equal(t, 100, response.Usage.PromptTokens)
		assert.Equal(t, 50, response.Usage.CompletionTokens)
		assert.Equal(t, 150, response.Usage.TotalTokens)
	})
}

// =============================================================================
// TestOpenAIResponseToJSON
// Tests for JSON serialization
// =============================================================================

func TestOpenAIResponseToJSON(t *testing.T) {
	t.Run("serializes to JSON", func(t *testing.T) {
		response := CreateOpenAIResponse("msg_123", "model", "Hello", nil, "stop", nil)
		json := response.ToJSON()

		assert.Contains(t, json, "msg_123")
		assert.Contains(t, json, "chat.completion")
		assert.Contains(t, json, "Hello")
	})
}

// =============================================================================
// TestOpenAITypes
// Tests for OpenAI type structures
// =============================================================================

func TestOpenAITypes(t *testing.T) {
	t.Run("OpenAIRequest defaults", func(t *testing.T) {
		req := OpenAIRequest{}

		assert.Equal(t, "", req.Model)
		assert.False(t, req.Stream)
		assert.Nil(t, req.Temperature)
		assert.Nil(t, req.MaxTokens)
	})

	t.Run("OpenAIMessage defaults", func(t *testing.T) {
		msg := OpenAIMessage{}

		assert.Equal(t, "", msg.Role)
		assert.Nil(t, msg.Content)
		assert.Empty(t, msg.ToolCalls)
	})

	t.Run("OpenAIUsage defaults", func(t *testing.T) {
		usage := OpenAIUsage{}

		assert.Equal(t, 0, usage.PromptTokens)
		assert.Equal(t, 0, usage.CompletionTokens)
		assert.Equal(t, 0, usage.TotalTokens)
	})
}
