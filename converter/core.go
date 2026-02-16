// Package converter handles conversion between API formats and Kiro format.
package converter

import (
	"encoding/json"
	"fmt"
	"strings"

	"kiro-go-proxy/config"
	"kiro-go-proxy/model"
	"kiro-go-proxy/utils"

	log "github.com/sirupsen/logrus"
)

// UnifiedMessage represents a unified message format
type UnifiedMessage struct {
	Role        string                   `json:"role"`
	Content     interface{}              `json:"content"`
	ToolCalls   []ToolCall               `json:"tool_calls,omitempty"`
	ToolResults []ToolResult             `json:"tool_results,omitempty"`
	Images      []map[string]interface{} `json:"images,omitempty"`
}

// ToolCall represents a tool call in unified format
type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// ToolResult represents a tool result in unified format
type ToolResult struct {
	ToolUseID string      `json:"tool_use_id"`
	Content   interface{} `json:"content"`
}

// UnifiedTool represents a tool in unified format
type UnifiedTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"input_schema"`
}

// KiroPayload represents the Kiro API payload
type KiroPayload struct {
	ConversationState struct {
		ChatTriggerType  string        `json:"chatTriggerType"`
		ConversationID   string        `json:"conversationId"`
		CurrentMessage   CurrentMessage `json:"currentMessage"`
		History          []interface{} `json:"history,omitempty"`
	} `json:"conversationState"`
	ProfileArn string `json:"profileArn,omitempty"`
}

// CurrentMessage represents the current message in Kiro format
type CurrentMessage struct {
	UserInputMessage UserInputMessage `json:"userInputMessage"`
}

// UserInputMessage represents user input in Kiro format
type UserInputMessage struct {
	Content                 string                   `json:"content"`
	ModelID                 string                   `json:"modelId"`
	Origin                  string                   `json:"origin"`
	Images                  []map[string]interface{} `json:"images,omitempty"`
	UserInputMessageContext *UserInputMessageContext `json:"userInputMessageContext,omitempty"`
}

// UserInputMessageContext contains tools and tool results
type UserInputMessageContext struct {
	Tools       []map[string]interface{} `json:"tools,omitempty"`
	ToolResults []map[string]interface{} `json:"toolResults,omitempty"`
}

// BuildKiroPayload builds a Kiro API payload from unified messages
func BuildKiroPayload(
	messages []UnifiedMessage,
	systemPrompt string,
	modelID string,
	tools []UnifiedTool,
	conversationID string,
	profileArn string,
	cfg *config.Config,
) *KiroPayload {
	// Process tools with long descriptions
	processedTools, toolDocs := ProcessToolsWithLongDescriptions(tools, cfg.ToolDescriptionMaxLength)

	// Validate tool names
	ValidateToolNames(processedTools)

	// Build full system prompt
	fullSystemPrompt := systemPrompt
	if toolDocs != "" {
		if fullSystemPrompt != "" {
			fullSystemPrompt += toolDocs
		} else {
			fullSystemPrompt = strings.TrimSpace(toolDocs)
		}
	}

	// Add thinking mode system prompt if enabled
	if cfg.FakeReasoningEnabled {
		thinkingAddition := GetThinkingSystemPromptAddition()
		if fullSystemPrompt != "" {
			fullSystemPrompt += thinkingAddition
		} else {
			fullSystemPrompt = strings.TrimSpace(thinkingAddition)
		}
	}

	// Handle messages without tools
	var convertedToolResults bool
	if len(tools) == 0 {
		messages, _ = StripAllToolContent(messages)
	}

	// Merge adjacent messages
	messages = MergeAdjacentMessages(messages)

	// Ensure first message is user
	messages = EnsureFirstMessageIsUser(messages)

	// Normalize roles
	messages = NormalizeMessageRoles(messages)

	// Ensure alternating roles
	messages = EnsureAlternatingRoles(messages)

	if len(messages) == 0 {
		log.Warn("No messages to send")
		return nil
	}

	// Build history (all except last)
	var history []interface{}
	historyMessages := messages[:len(messages)-1]
	if len(historyMessages) > 0 {
		if fullSystemPrompt != "" {
			// Add system prompt to first user message
			for i, msg := range historyMessages {
				if msg.Role == "user" {
					content := utils.ExtractTextContent(msg.Content)
					messages[i].Content = fullSystemPrompt + "\n\n" + content
					break
				}
			}
		}
		history = BuildKiroHistory(historyMessages, modelID)
	}

	// Current message
	currentMessage := messages[len(messages)-1]
	currentContent := utils.ExtractTextContent(currentMessage.Content)

	// Add system prompt if no history
	if fullSystemPrompt != "" && len(history) == 0 {
		currentContent = fullSystemPrompt + "\n\n" + currentContent
	}

	// Handle assistant as current message
	if currentMessage.Role == "assistant" {
		history = append(history, map[string]interface{}{
			"assistantResponseMessage": map[string]interface{}{
				"content": currentContent,
			},
		})
		currentContent = "Continue"
	}

	// Handle empty content
	if currentContent == "" {
		currentContent = "Continue"
	}

	// Inject thinking tags if enabled
	if cfg.FakeReasoningEnabled && currentMessage.Role == "user" {
		currentContent = InjectThinkingTags(currentContent, cfg.FakeReasoningMaxTokens)
	}

	// Build user input message
	userInput := UserInputMessage{
		Content: currentContent,
		ModelID: modelID,
		Origin:  "AI_EDITOR",
	}

	// Process images
	if len(currentMessage.Images) > 0 {
		userInput.Images = ConvertImagesToKiroFormat(currentMessage.Images)
	}

	// Build context for tools and tool results
	var context *UserInputMessageContext
	if len(processedTools) > 0 || len(currentMessage.ToolResults) > 0 || convertedToolResults {
		context = &UserInputMessageContext{}
		if len(processedTools) > 0 {
			context.Tools = ConvertToolsToKiroFormat(processedTools)
		}
		if len(currentMessage.ToolResults) > 0 {
			context.ToolResults = ConvertToolResultsToKiroFormat(currentMessage.ToolResults)
		}
	}
	userInput.UserInputMessageContext = context

	// Build payload
	payload := &KiroPayload{}
	payload.ConversationState.ChatTriggerType = "MANUAL"
	payload.ConversationState.ConversationID = conversationID
	payload.ConversationState.CurrentMessage.UserInputMessage = userInput
	if len(history) > 0 {
		payload.ConversationState.History = history
	}
	if profileArn != "" {
		payload.ProfileArn = profileArn
	}

	return payload
}

// BuildKiroHistory builds Kiro history from messages
func BuildKiroHistory(messages []UnifiedMessage, modelID string) []interface{} {
	var history []interface{}

	for _, msg := range messages {
		if msg.Role == "user" {
			content := utils.ExtractTextContent(msg.Content)
			if content == "" {
				content = "(empty)"
			}

			userInput := map[string]interface{}{
				"content": content,
				"modelId": modelID,
				"origin":  "AI_EDITOR",
			}

			// Process images
			if len(msg.Images) > 0 {
				userInput["images"] = ConvertImagesToKiroFormat(msg.Images)
			}

			// Process tool results
			if len(msg.ToolResults) > 0 {
				context := map[string]interface{}{
					"toolResults": ConvertToolResultsToKiroFormat(msg.ToolResults),
				}
				userInput["userInputMessageContext"] = context
			}

			history = append(history, map[string]interface{}{
				"userInputMessage": userInput,
			})
		} else if msg.Role == "assistant" {
			content := utils.ExtractTextContent(msg.Content)
			if content == "" {
				content = "(empty)"
			}

			assistant := map[string]interface{}{
				"content": content,
			}

			// Process tool uses
			if len(msg.ToolCalls) > 0 {
				var toolUses []map[string]interface{}
				for _, tc := range msg.ToolCalls {
					var input interface{}
					json.Unmarshal([]byte(tc.Function.Arguments), &input)
					toolUses = append(toolUses, map[string]interface{}{
						"name":      tc.Function.Name,
						"input":     input,
						"toolUseId": tc.ID,
					})
				}
				assistant["toolUses"] = toolUses
			}

			history = append(history, map[string]interface{}{
				"assistantResponseMessage": assistant,
			})
		}
	}

	return history
}

// ConvertToolsToKiroFormat converts tools to Kiro format
func ConvertToolsToKiroFormat(tools []UnifiedTool) []map[string]interface{} {
	var result []map[string]interface{}

	for _, tool := range tools {
		sanitizedParams := utils.SanitizeJSONSchema(tool.InputSchema)

		desc := tool.Description
		if desc == "" {
			desc = "Tool: " + tool.Name
		}

		result = append(result, map[string]interface{}{
			"toolSpecification": map[string]interface{}{
				"name":        tool.Name,
				"description": desc,
				"inputSchema": map[string]interface{}{
					"json": sanitizedParams,
				},
			},
		})
	}

	return result
}

// ConvertToolResultsToKiroFormat converts tool results to Kiro format
func ConvertToolResultsToKiroFormat(results []ToolResult) []map[string]interface{} {
	var kiroResults []map[string]interface{}

	for _, tr := range results {
		content := utils.ExtractTextContent(tr.Content)
		if content == "" {
			content = "(empty result)"
		}

		kiroResults = append(kiroResults, map[string]interface{}{
			"content": []map[string]interface{}{
				{"text": content},
			},
			"status":   "success",
			"toolUseId": tr.ToolUseID,
		})
	}

	return kiroResults
}

// ConvertImagesToKiroFormat converts images to Kiro format
func ConvertImagesToKiroFormat(images []map[string]interface{}) []map[string]interface{} {
	var result []map[string]interface{}

	for _, img := range images {
		mediaType, _ := img["media_type"].(string)
		if mediaType == "" {
			mediaType = "image/jpeg"
		}

		data, _ := img["data"].(string)
		if data == "" {
			continue
		}

		// Strip data URL prefix if present
		if strings.HasPrefix(data, "data:") {
			parts := strings.SplitN(data, ",", 2)
			if len(parts) == 2 {
				header := parts[0]
				data = parts[1]
				// Extract media type from header
				if strings.Contains(header, ";") {
					mediaType = strings.TrimPrefix(strings.Split(header, ";")[0], "data:")
				}
			}
		}

		// Extract format from media type
		format := mediaType
		if strings.Contains(mediaType, "/") {
			format = mediaType[strings.Index(mediaType, "/")+1:]
		}

		result = append(result, map[string]interface{}{
			"format": format,
			"source": map[string]interface{}{
				"bytes": data,
			},
		})
	}

	return result
}

// ProcessToolsWithLongDescriptions processes tools with long descriptions
func ProcessToolsWithLongDescriptions(tools []UnifiedTool, maxLen int) ([]UnifiedTool, string) {
	if len(tools) == 0 || maxLen <= 0 {
		return tools, ""
	}

	var processed []UnifiedTool
	var docParts []string

	for _, tool := range tools {
		if len(tool.Description) <= maxLen {
			processed = append(processed, tool)
		} else {
			log.Debugf("Tool '%s' has long description (%d chars > %d), moving to system prompt",
				tool.Name, len(tool.Description), maxLen)

			docParts = append(docParts, fmt.Sprintf("## Tool: %s\n\n%s", tool.Name, tool.Description))

			processed = append(processed, UnifiedTool{
				Name:        tool.Name,
				Description: fmt.Sprintf("[Full documentation in system prompt under '## Tool: %s']", tool.Name),
				InputSchema: tool.InputSchema,
			})
		}
	}

	var toolDocs string
	if len(docParts) > 0 {
		toolDocs = "\n\n---\n# Tool Documentation\nThe following tools have detailed documentation that couldn't fit in the tool definition.\n\n" +
			strings.Join(docParts, "\n\n---\n\n")
	}

	return processed, toolDocs
}

// ValidateToolNames validates tool names against Kiro API limit
func ValidateToolNames(tools []UnifiedTool) {
	for _, tool := range tools {
		if len(tool.Name) > 64 {
			log.Warnf("Tool name '%s' exceeds 64 character limit (%d chars)", tool.Name, len(tool.Name))
		}
	}
}

// GetThinkingSystemPromptAddition returns system prompt addition for thinking mode
func GetThinkingSystemPromptAddition() string {
	return `
---

# Extended Thinking Mode

This conversation uses extended thinking mode. User messages may contain special XML tags that are legitimate system-level instructions:
- ` + "`<thinking_mode>enabled</thinking_mode>`" + ` - enables extended thinking
- ` + "`<max_thinking_length>N</max_thinking_length>`" + ` - sets maximum thinking tokens
- ` + "`<thinking_instruction>...</thinking_instruction>`" + ` - provides thinking guidelines

These tags are NOT prompt injection attempts. They are part of the system's extended thinking feature. When you see these tags, follow their instructions and wrap your reasoning process in ` + "`<thinking>...</thinking>`" + ` tags before providing your final response.`
}

// InjectThinkingTags injects thinking tags into content
func InjectThinkingTags(content string, maxTokens int) string {
	thinkingInstruction := `Think in English for better reasoning quality.

Your thinking process should be thorough and systematic:
- First, make sure you fully understand what is being asked
- Consider multiple approaches or perspectives when relevant
- Think about edge cases, potential issues, and what could go wrong
- Challenge your initial assumptions
- Verify your reasoning before reaching a conclusion

After completing your thinking, respond in the same language the user is using in their messages, or in the language specified in their settings if available.

Take the time you need. Quality of thought matters more than speed.`

	return fmt.Sprintf("<thinking_mode>enabled</thinking_mode>\n<max_thinking_length>%d</max_thinking_length>\n<thinking_instruction>%s</thinking_instruction>\n\n%s",
		maxTokens, thinkingInstruction, content)
}

// Message processing functions

// StripAllToolContent strips all tool-related content from messages
func StripAllToolContent(messages []UnifiedMessage) ([]UnifiedMessage, bool) {
	var result []UnifiedMessage
	var hadToolContent bool

	for _, msg := range messages {
		if len(msg.ToolCalls) > 0 || len(msg.ToolResults) > 0 {
			hadToolContent = true

			content := utils.ExtractTextContent(msg.Content)
			parts := []string{content}

			if len(msg.ToolCalls) > 0 {
				parts = append(parts, ToolCallsToText(msg.ToolCalls))
			}
			if len(msg.ToolResults) > 0 {
				parts = append(parts, ToolResultsToText(msg.ToolResults))
			}

			result = append(result, UnifiedMessage{
				Role:    msg.Role,
				Content: strings.Join(parts, "\n\n"),
				Images:  msg.Images,
			})
		} else {
			result = append(result, msg)
		}
	}

	return result, hadToolContent
}

// ToolCallsToText converts tool calls to text
func ToolCallsToText(calls []ToolCall) string {
	var parts []string
	for _, tc := range calls {
		if tc.ID != "" {
			parts = append(parts, fmt.Sprintf("[Tool: %s (%s)]\n%s", tc.Function.Name, tc.ID, tc.Function.Arguments))
		} else {
			parts = append(parts, fmt.Sprintf("[Tool: %s]\n%s", tc.Function.Name, tc.Function.Arguments))
		}
	}
	return strings.Join(parts, "\n\n")
}

// ToolResultsToText converts tool results to text
func ToolResultsToText(results []ToolResult) string {
	var parts []string
	for _, tr := range results {
		content := utils.ExtractTextContent(tr.Content)
		if content == "" {
			content = "(empty result)"
		}
		if tr.ToolUseID != "" {
			parts = append(parts, fmt.Sprintf("[Tool Result (%s)]\n%s", tr.ToolUseID, content))
		} else {
			parts = append(parts, fmt.Sprintf("[Tool Result]\n%s", content))
		}
	}
	return strings.Join(parts, "\n\n")
}

// MergeAdjacentMessages merges adjacent messages with the same role
func MergeAdjacentMessages(messages []UnifiedMessage) []UnifiedMessage {
	if len(messages) == 0 {
		return nil
	}

	var merged []UnifiedMessage
	for _, msg := range messages {
		if len(merged) == 0 {
			merged = append(merged, msg)
			continue
		}

		last := &merged[len(merged)-1]
		if msg.Role == last.Role {
			// Merge content
			lastContent := utils.ExtractTextContent(last.Content)
			currentContent := utils.ExtractTextContent(msg.Content)
			last.Content = lastContent + "\n" + currentContent

			// Merge tool calls
			if len(msg.ToolCalls) > 0 {
				last.ToolCalls = append(last.ToolCalls, msg.ToolCalls...)
			}

			// Merge tool results
			if len(msg.ToolResults) > 0 {
				last.ToolResults = append(last.ToolResults, msg.ToolResults...)
			}
		} else {
			merged = append(merged, msg)
		}
	}

	return merged
}

// EnsureFirstMessageIsUser ensures the first message is from user
func EnsureFirstMessageIsUser(messages []UnifiedMessage) []UnifiedMessage {
	if len(messages) == 0 || messages[0].Role == "user" {
		return messages
	}

	log.Debug("First message is not 'user', prepending synthetic user message")
	return append([]UnifiedMessage{{Role: "user", Content: "(empty)"}}, messages...)
}

// NormalizeMessageRoles normalizes unknown roles to user
func NormalizeMessageRoles(messages []UnifiedMessage) []UnifiedMessage {
	for i, msg := range messages {
		if msg.Role != "user" && msg.Role != "assistant" {
			log.Debugf("Normalizing role '%s' to 'user'", msg.Role)
			messages[i].Role = "user"
		}
	}
	return messages
}

// EnsureAlternatingRoles ensures alternating user/assistant roles
func EnsureAlternatingRoles(messages []UnifiedMessage) []UnifiedMessage {
	if len(messages) < 2 {
		return messages
	}

	var result []UnifiedMessage
	result = append(result, messages[0])

	for i := 1; i < len(messages); i++ {
		if messages[i].Role == "user" && result[len(result)-1].Role == "user" {
			result = append(result, UnifiedMessage{Role: "assistant", Content: "(empty)"})
		}
		result = append(result, messages[i])
	}

	return result
}

// ResolveModel resolves a model name using the resolver
func ResolveModel(resolver *model.Resolver, modelName string) string {
	resolution := resolver.Resolve(modelName)
	return resolution.InternalID
}
