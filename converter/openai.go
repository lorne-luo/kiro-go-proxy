// Package converter handles conversion between API formats and Kiro format.
package converter

import (
	"encoding/json"

	"kiro-go-proxy/utils"

	log "github.com/sirupsen/logrus"
)

// OpenAI Models

// OpenAIRequest represents an OpenAI API request
type OpenAIRequest struct {
	Model            string             `json:"model"`
	Messages         []OpenAIMessage    `json:"messages"`
	Stream           bool               `json:"stream"`
	Tools            []OpenAITool       `json:"tools,omitempty"`
	Temperature      *float64           `json:"temperature,omitempty"`
	MaxTokens        *int               `json:"max_tokens,omitempty"`
	TopP             *float64           `json:"top_p,omitempty"`
	FrequencyPenalty *float64           `json:"frequency_penalty,omitempty"`
	PresencePenalty  *float64           `json:"presence_penalty,omitempty"`
	Stop             interface{}        `json:"stop,omitempty"`
	N                *int               `json:"n,omitempty"`
}

// OpenAIMessage represents an OpenAI message
type OpenAIMessage struct {
	Role      string          `json:"role"`
	Content   interface{}     `json:"content"`
	Name      string          `json:"name,omitempty"`
	ToolCalls []OpenAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
}

// OpenAIToolCall represents a tool call in OpenAI format
type OpenAIToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function OpenAIFunction  `json:"function"`
}

// OpenAIFunction represents function details
type OpenAIFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// OpenAITool represents a tool definition
type OpenAITool struct {
	Type     string              `json:"type"`
	Function OpenAIFunctionDef  `json:"function"`
}

// OpenAIFunctionDef represents a function definition
type OpenAIFunctionDef struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// OpenAIResponse represents an OpenAI API response
type OpenAIResponse struct {
	ID      string           `json:"id"`
	Object  string           `json:"object"`
	Created int64            `json:"created"`
	Model   string           `json:"model"`
	Choices []OpenAIChoice   `json:"choices"`
	Usage   *OpenAIUsage     `json:"usage,omitempty"`
}

// OpenAIChoice represents a choice in the response
type OpenAIChoice struct {
	Index        int                `json:"index"`
	Message      *OpenAIMessage     `json:"message,omitempty"`
	Delta        *OpenAIDelta       `json:"delta,omitempty"`
	FinishReason string             `json:"finish_reason"`
	LogProbs     interface{}        `json:"logprobs,omitempty"`
}

// OpenAIDelta represents a streaming delta
type OpenAIDelta struct {
	Role              string           `json:"role,omitempty"`
	Content           string           `json:"content,omitempty"`
	ReasoningContent  string           `json:"reasoning_content,omitempty"`
	ToolCalls         []OpenAIToolCall `json:"tool_calls,omitempty"`
}

// OpenAIUsage represents usage statistics
type OpenAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// OpenAIModelsResponse represents the models list response
type OpenAIModelsResponse struct {
	Object string            `json:"object"`
	Data   []OpenAIModelData `json:"data"`
}

// OpenAIModelData represents a model in the list
type OpenAIModelData struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

// ConvertOpenAIToUnified converts OpenAI messages to unified format
func ConvertOpenAIToUnified(messages []OpenAIMessage) ([]UnifiedMessage, string) {
	var unified []UnifiedMessage
	var systemPrompt string

	for _, msg := range messages {
		switch msg.Role {
		case "system":
			systemPrompt = utils.ExtractTextContent(msg.Content)
		case "user":
			unifiedMsg := UnifiedMessage{
				Role:    "user",
				Content: msg.Content,
			}
			// Check for tool result
			if msg.ToolCallID != "" {
				unifiedMsg.ToolResults = []ToolResult{{
					ToolUseID: msg.ToolCallID,
					Content:   msg.Content,
				}}
			}
			// Extract images from content
			unifiedMsg.Images = ExtractImagesFromOpenAIContent(msg.Content)
			unified = append(unified, unifiedMsg)
		case "assistant":
			unifiedMsg := UnifiedMessage{
				Role:    "assistant",
				Content: msg.Content,
			}
			// Convert tool calls
			if len(msg.ToolCalls) > 0 {
				for _, tc := range msg.ToolCalls {
					unifiedMsg.ToolCalls = append(unifiedMsg.ToolCalls, ToolCall{
						ID:   tc.ID,
						Type: tc.Type,
						Function: struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						}{
							Name:      tc.Function.Name,
							Arguments: tc.Function.Arguments,
						},
					})
				}
			}
			unified = append(unified, unifiedMsg)
		case "tool":
			// Tool result - add to previous user message or create new one
			if len(unified) > 0 && unified[len(unified)-1].Role == "user" {
				unified[len(unified)-1].ToolResults = append(unified[len(unified)-1].ToolResults, ToolResult{
					ToolUseID: msg.ToolCallID,
					Content:   msg.Content,
				})
			} else {
				unified = append(unified, UnifiedMessage{
					Role: "user",
					ToolResults: []ToolResult{{
						ToolUseID: msg.ToolCallID,
						Content:   msg.Content,
					}},
				})
			}
		default:
			log.Warnf("Unknown role '%s', treating as user", msg.Role)
			unified = append(unified, UnifiedMessage{
				Role:    "user",
				Content: msg.Content,
			})
		}
	}

	return unified, systemPrompt
}

// ConvertOpenAIToolsToUnified converts OpenAI tools to unified format
func ConvertOpenAIToolsToUnified(tools []OpenAITool) []UnifiedTool {
	var unified []UnifiedTool

	for _, tool := range tools {
		if tool.Type == "function" {
			unified = append(unified, UnifiedTool{
				Name:        tool.Function.Name,
				Description: tool.Function.Description,
				InputSchema: tool.Function.Parameters,
			})
		}
	}

	return unified
}

// ExtractImagesFromOpenAIContent extracts images from OpenAI content
func ExtractImagesFromOpenAIContent(content interface{}) []map[string]interface{} {
	var images []map[string]interface{}

	contentList, ok := content.([]interface{})
	if !ok {
		return images
	}

	for _, item := range contentList {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		itemType, _ := itemMap["type"].(string)
		if itemType != "image_url" {
			continue
		}

		imageURL, _ := itemMap["image_url"].(map[string]interface{})
		if imageURL == nil {
			continue
		}

		url, _ := imageURL["url"].(string)
		if url == "" {
			continue
		}

		// Parse data URL
		if len(url) > 5 && url[:5] == "data:" {
			parts := splitDataURI(url)
			if len(parts) == 2 {
				mediaType := extractMediaType(parts[0])
				images = append(images, map[string]interface{}{
					"media_type": mediaType,
					"data":       parts[1],
				})
			}
		}
	}

	return images
}

func splitDataURI(url string) []string {
	idx := -1
	for i := 5; i < len(url); i++ {
		if url[i] == ',' {
			idx = i
			break
		}
	}
	if idx == -1 {
		return nil
	}
	return []string{url[:idx], url[idx+1:]}
}

func extractMediaType(header string) string {
	// header is like "data:image/jpeg;base64"
	if len(header) < 5 {
		return "image/jpeg"
	}
	header = header[5:] // Remove "data:"
	if idx := findChar(header, ';'); idx != -1 {
		header = header[:idx]
	}
	return header
}

func findChar(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}

// CreateOpenAIResponse creates an OpenAI response
func CreateOpenAIResponse(id, model string, content string, toolCalls []ToolCall, finishReason string, usage *OpenAIUsage) *OpenAIResponse {
	return &OpenAIResponse{
		ID:      id,
		Object:  "chat.completion",
		Created: 0, // Will be set by caller
		Model:   model,
		Choices: []OpenAIChoice{{
			Index: 0,
			Message: &OpenAIMessage{
				Role:      "assistant",
				Content:   content,
				ToolCalls: convertToolCallsToOpenAI(toolCalls),
			},
			FinishReason: finishReason,
		}},
		Usage: usage,
	}
}

func convertToolCallsToOpenAI(calls []ToolCall) []OpenAIToolCall {
	if len(calls) == 0 {
		return nil
	}

	var result []OpenAIToolCall
	for _, tc := range calls {
		result = append(result, OpenAIToolCall{
			ID:   tc.ID,
			Type: tc.Type,
			Function: OpenAIFunction{
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			},
		})
	}
	return result
}

// ToJSON converts response to JSON
func (r *OpenAIResponse) ToJSON() string {
	b, _ := json.Marshal(r)
	return string(b)
}
