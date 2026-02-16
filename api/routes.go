// Package api provides HTTP routes for Kiro Gateway.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"kiro-go-proxy/auth"
	"kiro-go-proxy/client"
	"kiro-go-proxy/config"
	"kiro-go-proxy/converter"
	"kiro-go-proxy/model"
	"kiro-go-proxy/parser"
	"kiro-go-proxy/stream"
	"kiro-go-proxy/utils"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

// Server holds the API server dependencies
type Server struct {
	Cfg           *config.Config
	AuthManager   *auth.Manager
	HttpClient    *client.Client
	ModelCache    *model.Cache
	ModelResolver *model.Resolver
}

// NewServer creates a new API server
func NewServer(cfg *config.Config, authManager *auth.Manager) *Server {
	httpClient := client.NewClient(cfg, authManager)
	modelCache := model.NewCache(cfg)
	modelResolver := model.NewResolver(modelCache, cfg)

	return &Server{
		Cfg:           cfg,
		AuthManager:   authManager,
		HttpClient:    httpClient,
		ModelCache:    modelCache,
		ModelResolver: modelResolver,
	}
}

// SetupRoutes sets up all API routes
func (s *Server) SetupRoutes(r *gin.Engine) {
	// Health check
	r.GET("/", s.HealthHandler)
	r.GET("/health", s.HealthHandler)

	// OpenAI-compatible routes
	v1 := r.Group("/v1")
	v1.Use(s.AuthMiddleware())
	{
		v1.GET("/models", s.ListModelsHandler)
		v1.POST("/chat/completions", s.ChatCompletionsHandler)
	}

	// Anthropic-compatible routes
	v1.POST("/messages", s.MessagesHandler)
}

// AuthMiddleware validates API key
func (s *Server) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip auth for health endpoints
		if c.Request.URL.Path == "/" || c.Request.URL.Path == "/health" {
			c.Next()
			return
		}

		// Get authorization header
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{
					"message": "Missing Authorization header",
					"type":    "invalid_request_error",
				},
			})
			c.Abort()
			return
		}

		// Extract API key
		var apiKey string
		if strings.HasPrefix(authHeader, "Bearer ") {
			apiKey = strings.TrimPrefix(authHeader, "Bearer ")
		} else {
			apiKey = authHeader
		}

		// Validate API key
		if apiKey != s.Cfg.ProxyAPIKey {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{
					"message": "Invalid API key",
					"type":    "invalid_request_error",
				},
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// HealthHandler handles health check requests
func (s *Server) HealthHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "ok",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"version":   config.AppVersion,
	})
}

// ListModelsHandler handles GET /v1/models
func (s *Server) ListModelsHandler(c *gin.Context) {
	models := s.ModelResolver.GetAvailableModels()
	response := stream.CreateOpenAIModelsResponse(models)
	c.JSON(http.StatusOK, response)
}

// ChatCompletionsHandler handles POST /v1/chat/completions
func (s *Server) ChatCompletionsHandler(c *gin.Context) {
	var req converter.OpenAIRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": fmt.Sprintf("Invalid request: %v", err),
				"type":    "invalid_request_error",
			},
		})
		return
	}

	// Resolve model
	resolution := s.ModelResolver.Resolve(req.Model)
	log.Debugf("Model resolution: %s -> %s (source: %s)", req.Model, resolution.InternalID, resolution.Source)

	// Convert messages to unified format
	unifiedMessages, systemPrompt := converter.ConvertOpenAIToUnified(req.Messages)

	// Convert tools to unified format
	var unifiedTools []converter.UnifiedTool
	if len(req.Tools) > 0 {
		unifiedTools = converter.ConvertOpenAIToolsToUnified(req.Tools)
	}

	// Generate conversation ID
	conversationID := utils.GenerateConversationID()

	// Build Kiro payload
	payload := converter.BuildKiroPayload(
		unifiedMessages,
		systemPrompt,
		resolution.InternalID,
		unifiedTools,
		conversationID,
		s.AuthManager.ProfileArn(),
		s.Cfg,
	)

	if payload == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"message": "Failed to build request payload",
				"type":    "internal_error",
			},
		})
		return
	}

	// Build URL
	apiURL := fmt.Sprintf("%s/generateAssistantResponse", s.AuthManager.APIHost())

	// Handle streaming vs non-streaming
	if req.Stream {
		s.handleStreamingChatCompletion(c, apiURL, payload, req.Model, conversationID)
	} else {
		s.handleNonStreamingChatCompletion(c, apiURL, payload, req.Model, conversationID)
	}
}

func (s *Server) handleStreamingChatCompletion(c *gin.Context, apiURL string, payload *converter.KiroPayload, model, conversationID string) {
	// Make request
	ctx := context.Background()
	resp, err := s.HttpClient.PostStream(ctx, apiURL, payload)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"message": fmt.Sprintf("Request failed: %v", err),
				"type":    "internal_error",
			},
		})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		c.JSON(resp.StatusCode, gin.H{
			"error": gin.H{
				"message": string(body),
				"type":    "api_error",
			},
		})
		return
	}

	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Transfer-Encoding", "chunked")

	// Stream response
	events := stream.StreamToOpenAI(resp, model, conversationID, s.Cfg.FirstTokenTimeout, true, s.Cfg)

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"message": "Streaming not supported",
				"type":    "internal_error",
			},
		})
		return
	}

	for event := range events {
		c.Writer.WriteString(event)
		flusher.Flush()
	}

	// Send [DONE] marker
	c.Writer.WriteString("data: [DONE]\n\n")
	flusher.Flush()
}

func (s *Server) handleNonStreamingChatCompletion(c *gin.Context, apiURL string, payload *converter.KiroPayload, model, conversationID string) {
	ctx := context.Background()
	resp, err := s.HttpClient.PostStream(ctx, apiURL, payload)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"message": fmt.Sprintf("Request failed: %v", err),
				"type":    "internal_error",
			},
		})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		c.JSON(resp.StatusCode, gin.H{
			"error": gin.H{
				"message": string(body),
				"type":    "api_error",
			},
		})
		return
	}

	// Collect stream result
	result, err := stream.CollectStreamResult(resp, s.Cfg.FirstTokenTimeout, true, s.Cfg)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"message": fmt.Sprintf("Stream processing failed: %v", err),
				"type":    "internal_error",
			},
		})
		return
	}

	// Calculate token usage
	completionTokens := len(result.Content) / 4 // Rough estimate
	promptTokens, totalTokens, _, _ := stream.CalculateTokensFromContextUsage(
		result.ContextUsagePercentage,
		completionTokens,
		s.ModelCache,
		model,
	)

	// Build response
	response := converter.CreateOpenAIResponse(
		conversationID,
		model,
		result.Content,
		convertParserToolCalls(result.ToolCalls),
		"stop",
		&converter.OpenAIUsage{
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			TotalTokens:      totalTokens,
		},
	)

	c.JSON(http.StatusOK, response)
}

// MessagesHandler handles POST /v1/messages (Anthropic-compatible)
func (s *Server) MessagesHandler(c *gin.Context) {
	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": fmt.Sprintf("Invalid request: %v", err),
				"type":    "invalid_request_error",
			},
		})
		return
	}

	// Extract model
	modelName, _ := req["model"].(string)
	resolution := s.ModelResolver.Resolve(modelName)
	log.Debugf("Model resolution: %s -> %s (source: %s)", modelName, resolution.InternalID, resolution.Source)

	// Convert Anthropic request to unified format
	unifiedMessages, systemPrompt := convertAnthropicRequest(req)

	// Extract tools
	var unifiedTools []converter.UnifiedTool
	if tools, ok := req["tools"].([]interface{}); ok {
		for _, t := range tools {
			if toolMap, ok := t.(map[string]interface{}); ok {
				if toolMap["type"] == "function" || toolMap["name"] != nil {
					// Anthropic format
					name, _ := toolMap["name"].(string)
					desc, _ := toolMap["description"].(string)
					inputSchema, _ := toolMap["input_schema"].(map[string]interface{})

					if name != "" {
						unifiedTools = append(unifiedTools, converter.UnifiedTool{
							Name:        name,
							Description: desc,
							InputSchema: inputSchema,
						})
					}
				}
			}
		}
	}

	// Generate conversation ID
	conversationID := utils.GenerateConversationID()

	// Build Kiro payload
	payload := converter.BuildKiroPayload(
		unifiedMessages,
		systemPrompt,
		resolution.InternalID,
		unifiedTools,
		conversationID,
		s.AuthManager.ProfileArn(),
		s.Cfg,
	)

	if payload == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"message": "Failed to build request payload",
				"type":    "internal_error",
			},
		})
		return
	}

	// Build URL
	apiURL := fmt.Sprintf("%s/generateAssistantResponse", s.AuthManager.APIHost())

	// Check if streaming
	streaming, _ := req["stream"].(bool)

	if streaming {
		s.handleStreamingMessages(c, apiURL, payload, modelName, conversationID)
	} else {
		s.handleNonStreamingMessages(c, apiURL, payload, modelName, conversationID)
	}
}

func convertAnthropicRequest(req map[string]interface{}) ([]converter.UnifiedMessage, string) {
	var messages []converter.UnifiedMessage
	var systemPrompt string

	// Extract system prompt
	if sys, ok := req["system"]; ok {
		switch v := sys.(type) {
		case string:
			systemPrompt = v
		case []interface{}:
			// Handle list of content blocks
			var parts []string
			for _, block := range v {
				if blockMap, ok := block.(map[string]interface{}); ok {
					if blockMap["type"] == "text" {
						if text, ok := blockMap["text"].(string); ok {
							parts = append(parts, text)
						}
					}
				}
			}
			systemPrompt = strings.Join(parts, "\n")
		}
	}

	// Convert messages
	if msgList, ok := req["messages"].([]interface{}); ok {
		for _, msg := range msgList {
			if msgMap, ok := msg.(map[string]interface{}); ok {
				role, _ := msgMap["role"].(string)
				content := msgMap["content"]

				unifiedMsg := converter.UnifiedMessage{
					Role:    role,
					Content: content,
				}

				// Handle tool use in content
				if contentList, ok := content.([]interface{}); ok {
					for _, block := range contentList {
						if blockMap, ok := block.(map[string]interface{}); ok {
							blockType, _ := blockMap["type"].(string)

							if blockType == "tool_use" {
								name, _ := blockMap["name"].(string)
								id, _ := blockMap["id"].(string)
								input := blockMap["input"]

								var args string
								if input != nil {
									b, _ := json.Marshal(input)
									args = string(b)
								}

								unifiedMsg.ToolCalls = append(unifiedMsg.ToolCalls, converter.ToolCall{
									ID:   id,
									Type: "function",
									Function: struct {
										Name      string `json:"name"`
										Arguments string `json:"arguments"`
									}{
										Name:      name,
										Arguments: args,
									},
								})
							}

							if blockType == "tool_result" {
								toolUseID, _ := blockMap["tool_use_id"].(string)
								unifiedMsg.ToolResults = append(unifiedMsg.ToolResults, converter.ToolResult{
									ToolUseID: toolUseID,
									Content:   blockMap["content"],
								})
							}

							if blockType == "image" {
								// Extract image
								if source, ok := blockMap["source"].(map[string]interface{}); ok {
									if source["type"] == "base64" {
										mediaType, _ := source["media_type"].(string)
										data, _ := source["data"].(string)
										unifiedMsg.Images = append(unifiedMsg.Images, map[string]interface{}{
											"media_type": mediaType,
											"data":       data,
										})
									}
								}
							}
						}
					}
				}

				messages = append(messages, unifiedMsg)
			}
		}
	}

	return messages, systemPrompt
}

func (s *Server) handleStreamingMessages(c *gin.Context, apiURL string, payload *converter.KiroPayload, model, conversationID string) {
	ctx := context.Background()
	resp, err := s.HttpClient.PostStream(ctx, apiURL, payload)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"message": fmt.Sprintf("Request failed: %v", err),
				"type":    "internal_error",
			},
		})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		c.JSON(resp.StatusCode, gin.H{
			"error": gin.H{
				"message": string(body),
				"type":    "api_error",
			},
		})
		return
	}

	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"message": "Streaming not supported",
				"type":    "internal_error",
			},
		})
		return
	}

	// Stream in Anthropic format
	events, errs := stream.ParseKiroStream(resp, s.Cfg.FirstTokenTimeout, true, s.Cfg)

	// Send message_start event
	messageStart := map[string]interface{}{
		"type": "message_start",
		"message": map[string]interface{}{
			"id":         conversationID,
			"type":       "message",
			"role":       "assistant",
			"content":    []interface{}{},
			"model":      model,
			"stop_reason": nil,
			"usage": map[string]interface{}{
				"input_tokens":  0,
				"output_tokens": 0,
			},
		},
	}
	b, _ := json.Marshal(messageStart)
	c.Writer.WriteString("event: message_start\ndata: " + string(b) + "\n\n")
	flusher.Flush()

	// Track content blocks
	contentIndex := 0
	textBlockStarted := false
	thinkingBlockStarted := false
	var textBlockIndex int
	var thinkingBlockIndex int
	var outputTokens int

	for {
		select {
		case event, ok := <-events:
			if !ok {
				// Close any open blocks
				if thinkingBlockStarted {
					c.Writer.WriteString(fmt.Sprintf("event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":%d}\n\n", thinkingBlockIndex))
					flusher.Flush()
				}
				if textBlockStarted {
					c.Writer.WriteString(fmt.Sprintf("event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":%d}\n\n", textBlockIndex))
					flusher.Flush()
				}

				// Send message_delta with final usage
				messageDelta := map[string]interface{}{
					"type": "message_delta",
					"delta": map[string]interface{}{
						"stop_reason": "end_turn",
					},
					"usage": map[string]interface{}{
						"output_tokens": outputTokens,
					},
				}
				b, _ := json.Marshal(messageDelta)
				c.Writer.WriteString("event: message_delta\ndata: " + string(b) + "\n\n")
				flusher.Flush()

				// Send message_stop
				c.Writer.WriteString("event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
				flusher.Flush()
				return
			}

			switch event.Type {
			case "content":
				if event.Content != "" {
					// Start text block if not started
					if !textBlockStarted {
						textBlockIndex = contentIndex
						contentIndex++
						textBlockStarted = true
						startBlock := map[string]interface{}{
							"type":  "content_block_start",
							"index": textBlockIndex,
							"content_block": map[string]interface{}{
								"type": "text",
								"text": "",
							},
						}
						b, _ := json.Marshal(startBlock)
						c.Writer.WriteString("event: content_block_start\ndata: " + string(b) + "\n\n")
						flusher.Flush()
					}

					// Send text delta
					contentBlock := map[string]interface{}{
						"type":  "content_block_delta",
						"index": textBlockIndex,
						"delta": map[string]interface{}{
							"type": "text_delta",
							"text": event.Content,
						},
					}
					b, _ := json.Marshal(contentBlock)
					c.Writer.WriteString("event: content_block_delta\ndata: " + string(b) + "\n\n")
					flusher.Flush()
					outputTokens += len(event.Content) / 4
				}

			case "thinking":
				if event.ThinkingContent != "" {
					// Start thinking block if not started
					if !thinkingBlockStarted {
						thinkingBlockIndex = contentIndex
						contentIndex++
						thinkingBlockStarted = true
						startBlock := map[string]interface{}{
							"type":  "content_block_start",
							"index": thinkingBlockIndex,
							"content_block": map[string]interface{}{
								"type":     "thinking",
								"thinking": "",
							},
						}
						b, _ := json.Marshal(startBlock)
						c.Writer.WriteString("event: content_block_start\ndata: " + string(b) + "\n\n")
						flusher.Flush()
					}

					// Send thinking delta
					contentBlock := map[string]interface{}{
						"type":  "content_block_delta",
						"index": thinkingBlockIndex,
						"delta": map[string]interface{}{
							"type":     "thinking_delta",
							"thinking": event.ThinkingContent,
						},
					}
					b, _ := json.Marshal(contentBlock)
					c.Writer.WriteString("event: content_block_delta\ndata: " + string(b) + "\n\n")
					flusher.Flush()
					outputTokens += len(event.ThinkingContent) / 4
				}

			case "tool_use":
				if event.ToolUse != nil {
					// Close thinking block if open
					if thinkingBlockStarted {
						c.Writer.WriteString(fmt.Sprintf("event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":%d}\n\n", thinkingBlockIndex))
						flusher.Flush()
						thinkingBlockStarted = false
					}

					// Close text block if open
					if textBlockStarted {
						c.Writer.WriteString(fmt.Sprintf("event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":%d}\n\n", textBlockIndex))
						flusher.Flush()
						textBlockStarted = false
					}

					tool := event.ToolUse
					toolID, _ := tool["id"].(string)
					if toolID == "" {
						toolID = utils.GenerateToolUseID()
					}

					// Get tool name and input
					var toolName string
					var toolInput interface{}

					if fn, ok := tool["function"].(map[string]interface{}); ok {
						toolName, _ = fn["name"].(string)
						toolInput = fn["arguments"]
					} else if name, ok := tool["name"].(string); ok {
						toolName = name
						toolInput = tool["input"]
					}

					// Parse input if string
					if inputStr, ok := toolInput.(string); ok && inputStr != "" {
						var parsed interface{}
						if err := json.Unmarshal([]byte(inputStr), &parsed); err == nil {
							toolInput = parsed
						}
					}
					if toolInput == nil {
						toolInput = map[string]interface{}{}
					}

					// Send content_block_start for tool_use
					toolBlockIndex := contentIndex
					contentIndex++
					startBlock := map[string]interface{}{
						"type":  "content_block_start",
						"index": toolBlockIndex,
						"content_block": map[string]interface{}{
							"type":  "tool_use",
							"id":    toolID,
							"name":  toolName,
							"input": map[string]interface{}{},
						},
					}
					b, _ := json.Marshal(startBlock)
					c.Writer.WriteString("event: content_block_start\ndata: " + string(b) + "\n\n")
					flusher.Flush()

					// Send content_block_delta with input_json_delta
					inputJSON, _ := json.Marshal(toolInput)
					deltaBlock := map[string]interface{}{
						"type":  "content_block_delta",
						"index": toolBlockIndex,
						"delta": map[string]interface{}{
							"type":         "input_json_delta",
							"partial_json": string(inputJSON),
						},
					}
					b, _ = json.Marshal(deltaBlock)
					c.Writer.WriteString("event: content_block_delta\ndata: " + string(b) + "\n\n")
					flusher.Flush()

					// Send content_block_stop
					c.Writer.WriteString(fmt.Sprintf("event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":%d}\n\n", toolBlockIndex))
					flusher.Flush()

					outputTokens += len(toolName) / 2
				}

			case "context_usage":
				// Context usage info - used for token calculation
				// Will be used in message_delta if available
			}

		case err := <-errs:
			if err != nil {
				errorBlock := map[string]interface{}{
					"type": "error",
					"error": map[string]interface{}{
						"type":    "internal_error",
						"message": err.Error(),
					},
				}
				b, _ := json.Marshal(errorBlock)
				c.Writer.WriteString("event: error\ndata: " + string(b) + "\n\n")
				flusher.Flush()
				return
			}
		}
	}
}

func (s *Server) handleNonStreamingMessages(c *gin.Context, apiURL string, payload *converter.KiroPayload, model, conversationID string) {
	ctx := context.Background()
	resp, err := s.HttpClient.PostStream(ctx, apiURL, payload)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"message": fmt.Sprintf("Request failed: %v", err),
				"type":    "internal_error",
			},
		})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		c.JSON(resp.StatusCode, gin.H{
			"error": gin.H{
				"message": string(body),
				"type":    "api_error",
			},
		})
		return
	}

	// Collect stream result
	result, err := stream.CollectStreamResult(resp, s.Cfg.FirstTokenTimeout, true, s.Cfg)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"message": fmt.Sprintf("Stream processing failed: %v", err),
				"type":    "internal_error",
			},
		})
		return
	}

	// Build Anthropic-style response
	var content []map[string]interface{}

	if result.Content != "" {
		content = append(content, map[string]interface{}{
			"type": "text",
			"text": result.Content,
		})
	}

	for _, tc := range result.ToolCalls {
		content = append(content, map[string]interface{}{
			"type":  "tool_use",
			"id":    tc.ID,
			"name":  tc.Function.Name,
			"input": json.RawMessage(tc.Function.Arguments),
		})
	}

	response := map[string]interface{}{
		"id":    conversationID,
		"type":  "message",
		"role":  "assistant",
		"model": model,
		"content": content,
		"stop_reason": "end_turn",
		"usage": map[string]interface{}{
			"input_tokens":  0,
			"output_tokens": len(result.Content) / 4,
		},
	}

	c.JSON(http.StatusOK, response)
}

// convertParserToolCalls converts parser.ToolCall to converter.ToolCall
func convertParserToolCalls(calls []parser.ToolCall) []converter.ToolCall {
	if len(calls) == 0 {
		return nil
	}

	result := make([]converter.ToolCall, len(calls))
	for i, tc := range calls {
		result[i] = converter.ToolCall{
			ID:   tc.ID,
			Type: tc.Type,
			Function: struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			}{
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			},
		}
	}
	return result
}
