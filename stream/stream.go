// Package stream provides streaming support for Kiro Gateway.
package stream

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"kiro-go-proxy/config"
	"kiro-go-proxy/converter"
	"kiro-go-proxy/model"
	"kiro-go-proxy/parser"

	log "github.com/sirupsen/logrus"
)

// KiroEvent represents a unified event from Kiro API stream
type KiroEvent struct {
	Type                   string
	Content                string
	ThinkingContent        string
	ToolUse                map[string]interface{}
	Usage                  map[string]interface{}
	ContextUsagePercentage *float64
	IsFirstThinkingChunk   bool
	IsLastThinkingChunk    bool
}

// StreamResult represents the collected stream result
type StreamResult struct {
	Content               string
	ThinkingContent       string
	ToolCalls             []parser.ToolCall
	Usage                 map[string]interface{}
	ContextUsagePercentage *float64
}

// FirstTokenTimeoutError is raised when first token timeout occurs
type FirstTokenTimeoutError struct {
	Timeout float64
}

func (e *FirstTokenTimeoutError) Error() string {
	return fmt.Sprintf("no response within %.0f seconds", e.Timeout)
}

// ParseKiroStream parses Kiro SSE stream and yields events
func ParseKiroStream(
	response *http.Response,
	firstTokenTimeout float64,
	enableThinkingParser bool,
	cfg *config.Config,
) (<-chan KiroEvent, <-chan error) {
	events := make(chan KiroEvent, 100)
	errs := make(chan error, 1)

	go func() {
		defer close(events)
		defer close(errs)

		awsParser := parser.NewAwsEventStreamParser()

		var thinkingParser *parser.ThinkingParser
		if cfg.FakeReasoningEnabled && enableThinkingParser {
			thinkingParser = parser.NewThinkingParser(
				parser.ThinkingHandlingMode(cfg.FakeReasoningHandling),
				cfg.FakeReasoningOpenTags,
				cfg.FakeReasoningBufferSize,
			)
			log.Debugf("Thinking parser initialized with mode: %s", cfg.FakeReasoningHandling)
		}

		reader := bufio.NewReader(response.Body)

		// Wait for first chunk with timeout
		firstChunk := make([]byte, 4096)
		n, err := reader.Read(firstChunk)
		if err != nil {
			if err == io.EOF {
				log.Debug("Empty response from Kiro API")
				return
			}
			errs <- fmt.Errorf("error reading first chunk: %w", err)
			return
		}

		log.Debug("First token received")

		// Process chunks
		buffer := firstChunk[:n]

		for {
			// Process current buffer
			parsedEvents := awsParser.Feed(buffer)
			for _, event := range parsedEvents {
				kiroEvent := processAwsEvent(event, thinkingParser)
				if kiroEvent != nil {
					events <- *kiroEvent
				}
			}

			// Read next chunk
			buffer = make([]byte, 4096)
			n, err := reader.Read(buffer)
			if err != nil {
				if err == io.EOF {
					break
				}
				errs <- fmt.Errorf("error reading stream: %w", err)
				return
			}
			buffer = buffer[:n]
		}

		// Finalize thinking parser
		if thinkingParser != nil {
			finalResult := thinkingParser.Finalize()
			if finalResult.ThinkingContent != "" {
				events <- KiroEvent{
					Type:                 "thinking",
					ThinkingContent:      finalResult.ThinkingContent,
					IsFirstThinkingChunk: finalResult.IsFirstThinkingChunk,
					IsLastThinkingChunk:  finalResult.IsLastThinkingChunk,
				}
			}
			if finalResult.RegularContent != "" {
				events <- KiroEvent{
					Type:    "content",
					Content: finalResult.RegularContent,
				}
			}
		}

		// Yield tool calls
		for _, tc := range awsParser.GetToolCalls() {
			events <- KiroEvent{
				Type: "tool_use",
				ToolUse: map[string]interface{}{
					"id":   tc.ID,
					"type": tc.Type,
					"function": map[string]interface{}{
						"name":      tc.Function.Name,
						"arguments": tc.Function.Arguments,
					},
				},
			}
		}
	}()

	return events, errs
}

func processAwsEvent(event parser.Event, thinkingParser *parser.ThinkingParser) *KiroEvent {
	switch event.Type {
	case parser.EventTypeContent:
		contentData, ok := event.Data.(parser.ContentData)
		if !ok {
			return nil
		}

		if thinkingParser != nil {
			result := thinkingParser.Feed(contentData.Content)
			if result.ThinkingContent != "" {
				return &KiroEvent{
					Type:                 "thinking",
					ThinkingContent:      result.ThinkingContent,
					IsFirstThinkingChunk: result.IsFirstThinkingChunk,
					IsLastThinkingChunk:  result.IsLastThinkingChunk,
				}
			}
			if result.RegularContent != "" {
				return &KiroEvent{
					Type:    "content",
					Content: result.RegularContent,
				}
			}
			return nil
		}

		return &KiroEvent{
			Type:    "content",
			Content: contentData.Content,
		}

	case parser.EventTypeUsage:
		usageData, ok := event.Data.(parser.UsageData)
		if !ok {
			return nil
		}
		return &KiroEvent{
			Type: "usage",
			Usage: map[string]interface{}{
				"credits": usageData.Credits,
			},
		}

	case parser.EventTypeContextUsage:
		contextData, ok := event.Data.(parser.ContextUsageData)
		if !ok {
			return nil
		}
		return &KiroEvent{
			Type:                   "context_usage",
			ContextUsagePercentage: &contextData.Percentage,
		}
	}

	return nil
}

// CollectStreamResult collects full response from stream
func CollectStreamResult(
	response *http.Response,
	firstTokenTimeout float64,
	enableThinkingParser bool,
	cfg *config.Config,
) (*StreamResult, error) {
	events, errs := ParseKiroStream(response, firstTokenTimeout, enableThinkingParser, cfg)

	result := &StreamResult{}
	var fullContentForBracketTools strings.Builder

	for {
		select {
		case event, ok := <-events:
			if !ok {
				// Check for bracket-style tool calls
				bracketToolCalls := parser.ParseBracketToolCalls(fullContentForBracketTools.String())
				if len(bracketToolCalls) > 0 {
					result.ToolCalls = parser.DeduplicateToolCalls(append(result.ToolCalls, bracketToolCalls...))
				}
				return result, nil
			}

			switch event.Type {
			case "content":
				result.Content += event.Content
				fullContentForBracketTools.WriteString(event.Content)
			case "thinking":
				result.ThinkingContent += event.ThinkingContent
				fullContentForBracketTools.WriteString(event.ThinkingContent)
			case "tool_use":
				tc := parser.ToolCall{
					ID:   event.ToolUse["id"].(string),
					Type: event.ToolUse["type"].(string),
				}
				if fn, ok := event.ToolUse["function"].(map[string]interface{}); ok {
					tc.Function.Name = fn["name"].(string)
					tc.Function.Arguments = fn["arguments"].(string)
				}
				result.ToolCalls = append(result.ToolCalls, tc)
			case "usage":
				result.Usage = event.Usage
			case "context_usage":
				result.ContextUsagePercentage = event.ContextUsagePercentage
			}

		case err := <-errs:
			if err != nil {
				return nil, err
			}
		}
	}
}

// CalculateTokensFromContextUsage calculates token counts from context usage percentage
func CalculateTokensFromContextUsage(
	contextUsagePercentage *float64,
	completionTokens int,
	modelCache *model.Cache,
	model string,
) (promptTokens, totalTokens int, promptSource, totalSource string) {
	if contextUsagePercentage != nil && *contextUsagePercentage > 0 {
		maxInputTokens := modelCache.GetMaxInputTokens(model)
		totalTokens = int((*contextUsagePercentage / 100) * float64(maxInputTokens))
		promptTokens = totalTokens - completionTokens
		if promptTokens < 0 {
			promptTokens = 0
		}
		return promptTokens, totalTokens, "subtraction", "API Kiro"
	}

	return 0, completionTokens, "unknown", "tiktoken"
}

// OpenAI Streaming

// StreamToOpenAI converts Kiro stream to OpenAI SSE format
func StreamToOpenAI(
	response *http.Response,
	model string,
	conversationID string,
	firstTokenTimeout float64,
	enableThinkingParser bool,
	cfg *config.Config,
) <-chan string {
	output := make(chan string, 100)

	go func() {
		defer close(output)

		events, errs := ParseKiroStream(response, firstTokenTimeout, enableThinkingParser, cfg)

		chunkIndex := 0
		toolCallIndex := 0

		for {
			select {
			case event, ok := <-events:
				if !ok {
					// Send finish chunk
					finishChunk := createOpenAIFinishChunk(conversationID, model, chunkIndex)
					output <- formatSSE(finishChunk)
					return
				}

				chunkIndex++
				var chunk string

				switch event.Type {
				case "content":
					if event.Content != "" {
						chunk = createOpenAIContentChunk(conversationID, model, event.Content, chunkIndex)
					}
				case "thinking":
					if event.ThinkingContent != "" && cfg.FakeReasoningHandling == "as_reasoning_content" {
						chunk = createOpenAIReasoningChunk(conversationID, model, event.ThinkingContent, chunkIndex)
					}
				case "tool_use":
					chunk = createOpenAIToolCallChunk(conversationID, model, event.ToolUse, chunkIndex, toolCallIndex)
					toolCallIndex++
				}

				if chunk != "" {
					output <- formatSSE(chunk)
				}

			case err := <-errs:
				if err != nil {
					errorChunk := createOpenAIErrorChunk(err.Error())
					output <- formatSSE(errorChunk)
					return
				}
			}
		}
	}()

	return output
}

func createOpenAIContentChunk(id, model, content string, index int) string {
	delta := map[string]interface{}{
		"content": content,
	}
	return createOpenAIDeltaChunk(id, model, delta, index, "")
}

func createOpenAIReasoningChunk(id, model, content string, index int) string {
	delta := map[string]interface{}{
		"reasoning_content": content,
	}
	return createOpenAIDeltaChunk(id, model, delta, index, "")
}

func createOpenAIToolCallChunk(id string, model string, toolUse map[string]interface{}, chunkIndex, toolCallIndex int) string {
	delta := map[string]interface{}{
		"tool_calls": []map[string]interface{}{
			{
				"index": toolCallIndex,
				"id":    toolUse["id"],
				"type":  toolUse["type"],
				"function": map[string]interface{}{
					"name":      toolUse["function"].(map[string]interface{})["name"],
					"arguments": toolUse["function"].(map[string]interface{})["arguments"],
				},
			},
		},
	}
	return createOpenAIDeltaChunk(id, model, delta, chunkIndex, "")
}

func createOpenAIFinishChunk(id, model string, index int) string {
	return createOpenAIDeltaChunk(id, model, map[string]interface{}{}, index, "stop")
}

func createOpenAIErrorChunk(message string) string {
	errorResp := map[string]interface{}{
		"error": map[string]interface{}{
			"message": message,
			"type":    "internal_error",
		},
	}
	b, _ := json.Marshal(errorResp)
	return string(b)
}

func createOpenAIDeltaChunk(id, model string, delta map[string]interface{}, index int, finishReason string) string {
	chunk := map[string]interface{}{
		"id":      id,
		"object":  "chat.completion.chunk",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []map[string]interface{}{
			{
				"index": index,
				"delta": delta,
			},
		},
	}

	if finishReason != "" {
		chunk["choices"].([]map[string]interface{})[0]["finish_reason"] = finishReason
	}

	b, _ := json.Marshal(chunk)
	return string(b)
}

func formatSSE(data string) string {
	return fmt.Sprintf("data: %s\n\n", data)
}

// ParseSSE parses SSE data from reader
func ParseSSE(reader io.Reader) <-chan string {
	output := make(chan string, 100)

	go func() {
		defer close(output)

		scanner := bufio.NewScanner(reader)
		var buffer bytes.Buffer

		for scanner.Scan() {
			line := scanner.Text()

			if line == "" {
				// Empty line signals end of event
				if buffer.Len() > 0 {
					data := buffer.String()
					if strings.HasPrefix(data, "data: ") {
						output <- strings.TrimPrefix(data, "data: ")
					}
					buffer.Reset()
				}
				continue
			}

			if strings.HasPrefix(line, "data: ") {
				if buffer.Len() > 0 {
					buffer.WriteByte('\n')
				}
				buffer.WriteString(line)
			}
		}
	}()

	return output
}

// CreateOpenAIModelsResponse creates a models list response
func CreateOpenAIModelsResponse(models []string) *converter.OpenAIModelsResponse {
	var data []converter.OpenAIModelData
	now := time.Now().Unix()

	for _, id := range models {
		data = append(data, converter.OpenAIModelData{
			ID:      id,
			Object:  "model",
			Created: now,
			OwnedBy: "kiro",
		})
	}

	return &converter.OpenAIModelsResponse{
		Object: "list",
		Data:   data,
	}
}
