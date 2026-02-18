// Package parser provides tests for AWS Event Stream format and thinking blocks.
package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// =============================================================================
// TestFindMatchingBrace
// Original: /code/github/kiro-gateway/tests/unit/test_parsers.py::TestFindMatchingBrace
// =============================================================================

func TestFindMatchingBrace(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		startPos int
		want     int
	}{
		// Original: test_simple_json_object
		{
			name:     "simple JSON object",
			input:    `{"key": "value"}`,
			startPos: 0,
			want:     15,
		},
		// Original: test_nested_json_object
		{
			name:     "nested JSON object",
			input:    `{"outer": {"inner": "value"}}`,
			startPos: 0,
			want:     28,
		},
		// Original: test_json_with_braces_in_string
		{
			name:     "JSON with braces in string",
			input:    `{"text": "Hello {world}"}`,
			startPos: 0,
			want:     24,
		},
		// Original: test_json_with_escaped_quotes
		{
			name:     "JSON with escaped quotes",
			input:    `{"text": "Say \"hello\""}`,
			startPos: 0,
			want:     24,
		},
		// Original: test_incomplete_json
		{
			name:     "incomplete JSON",
			input:    `{"key": "value"`,
			startPos: 0,
			want:     -1,
		},
		// Original: test_invalid_start_position
		{
			name:     "invalid start position",
			input:    `hello {"key": "value"}`,
			startPos: 0,
			want:     -1,
		},
		// Original: test_start_position_out_of_bounds
		{
			name:     "start position out of bounds",
			input:    `{"a":1}`,
			startPos: 100,
			want:     -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FindMatchingBrace(tt.input, tt.startPos)
			assert.Equal(t, tt.want, got)
		})
	}
}

// =============================================================================
// TestParseBracketToolCalls
// Original: /code/github/kiro-gateway/tests/unit/test_parsers.py::TestParseBracketToolCalls
// =============================================================================

func TestParseBracketToolCalls(t *testing.T) {
	t.Run("parses single tool call", func(t *testing.T) {
		// Original: test_parses_single_tool_call
		text := `[Called get_weather with args: {"location": "Moscow"}]`
		result := ParseBracketToolCalls(text)

		assert.Len(t, result, 1)
		assert.Equal(t, "get_weather", result[0].Function.Name)
		assert.Contains(t, result[0].Function.Arguments, "location")
	})

	t.Run("parses multiple tool calls", func(t *testing.T) {
		// Original: test_parses_multiple_tool_calls
		text := `
		[Called get_weather with args: {"location": "Moscow"}]
		Some text in between
		[Called get_time with args: {"timezone": "UTC"}]
		`
		result := ParseBracketToolCalls(text)

		assert.Len(t, result, 2)
		assert.Equal(t, "get_weather", result[0].Function.Name)
		assert.Equal(t, "get_time", result[1].Function.Name)
	})

	t.Run("returns empty for no tool calls", func(t *testing.T) {
		// Original: test_returns_empty_for_no_tool_calls
		text := "This is just regular text without any tool calls."
		result := ParseBracketToolCalls(text)
		assert.Nil(t, result)
	})

	t.Run("returns empty for empty string", func(t *testing.T) {
		// Original: test_returns_empty_for_empty_string
		result := ParseBracketToolCalls("")
		assert.Nil(t, result)
	})

	t.Run("handles nested JSON in args", func(t *testing.T) {
		// Original: test_handles_nested_json_in_args
		text := `[Called complex_func with args: {"data": {"nested": {"deep": "value"}}}]`
		result := ParseBracketToolCalls(text)

		assert.Len(t, result, 1)
		assert.Equal(t, "complex_func", result[0].Function.Name)
		assert.Contains(t, result[0].Function.Arguments, "nested")
	})

	t.Run("generates unique IDs", func(t *testing.T) {
		// Original: test_generates_unique_ids
		text := `
		[Called func with args: {"a": 1}]
		[Called func with args: {"a": 1}]
		`
		result := ParseBracketToolCalls(text)

		assert.Len(t, result, 2)
		assert.NotEqual(t, result[0].ID, result[1].ID)
	})
}

// =============================================================================
// TestDeduplicateToolCalls
// Original: /code/github/kiro-gateway/tests/unit/test_parsers.py::TestDeduplicateToolCalls
// =============================================================================

func TestDeduplicateToolCalls(t *testing.T) {
	t.Run("removes duplicates", func(t *testing.T) {
		// Original: test_removes_duplicates
		toolCalls := []ToolCall{
			{ID: "1", Function: ToolCallFunction{Name: "func", Arguments: `{"a": 1}`}},
			{ID: "2", Function: ToolCallFunction{Name: "func", Arguments: `{"a": 1}`}},
			{ID: "3", Function: ToolCallFunction{Name: "other", Arguments: `{"b": 2}`}},
		}
		result := DeduplicateToolCalls(toolCalls)
		assert.Len(t, result, 2)
	})

	t.Run("preserves first occurrence", func(t *testing.T) {
		// Original: test_preserves_first_occurrence
		toolCalls := []ToolCall{
			{ID: "first", Function: ToolCallFunction{Name: "func", Arguments: `{"a": 1}`}},
			{ID: "second", Function: ToolCallFunction{Name: "func", Arguments: `{"a": 1}`}},
		}
		result := DeduplicateToolCalls(toolCalls)
		assert.Len(t, result, 1)
		assert.Equal(t, "first", result[0].ID)
	})

	t.Run("handles empty list", func(t *testing.T) {
		// Original: test_handles_empty_list
		result := DeduplicateToolCalls(nil)
		assert.Nil(t, result)
	})

	t.Run("deduplicates by id keeps one with arguments", func(t *testing.T) {
		// Original: test_deduplicates_by_id_keeps_one_with_arguments
		toolCalls := []ToolCall{
			{ID: "call_123", Function: ToolCallFunction{Name: "func", Arguments: "{}"}},
			{ID: "call_123", Function: ToolCallFunction{Name: "func", Arguments: `{"location": "Moscow"}`}},
		}
		result := DeduplicateToolCalls(toolCalls)

		assert.Len(t, result, 1)
		assert.Contains(t, result[0].Function.Arguments, "Moscow")
	})

	t.Run("deduplicates by id prefers longer arguments", func(t *testing.T) {
		// Original: test_deduplicates_by_id_prefers_longer_arguments
		toolCalls := []ToolCall{
			{ID: "call_abc", Function: ToolCallFunction{Name: "search", Arguments: `{"q": "test"}`}},
			{ID: "call_abc", Function: ToolCallFunction{Name: "search", Arguments: `{"q": "test", "limit": 10, "offset": 0}`}},
		}
		result := DeduplicateToolCalls(toolCalls)

		assert.Len(t, result, 1)
		assert.Contains(t, result[0].Function.Arguments, "limit")
	})

	t.Run("deduplicates empty arguments replaced by non-empty", func(t *testing.T) {
		// Original: test_deduplicates_empty_arguments_replaced_by_non_empty
		toolCalls := []ToolCall{
			{ID: "call_xyz", Function: ToolCallFunction{Name: "get_weather", Arguments: "{}"}},
			{ID: "call_xyz", Function: ToolCallFunction{Name: "get_weather", Arguments: `{"city": "London"}`}},
		}
		result := DeduplicateToolCalls(toolCalls)

		assert.Len(t, result, 1)
		assert.Equal(t, `{"city": "London"}`, result[0].Function.Arguments)
	})

	t.Run("handles tool calls without id", func(t *testing.T) {
		// Original: test_handles_tool_calls_without_id
		toolCalls := []ToolCall{
			{ID: "", Function: ToolCallFunction{Name: "func", Arguments: `{"a": 1}`}},
			{ID: "", Function: ToolCallFunction{Name: "func", Arguments: `{"a": 1}`}},
			{ID: "", Function: ToolCallFunction{Name: "func", Arguments: `{"b": 2}`}},
		}
		result := DeduplicateToolCalls(toolCalls)
		// Two unique by name+arguments
		assert.Len(t, result, 2)
	})

	t.Run("mixed with and without id", func(t *testing.T) {
		// Original: test_mixed_with_and_without_id
		toolCalls := []ToolCall{
			{ID: "call_1", Function: ToolCallFunction{Name: "func1", Arguments: `{"x": 1}`}},
			{ID: "call_1", Function: ToolCallFunction{Name: "func1", Arguments: "{}"}}, // Duplicate by id
			{ID: "", Function: ToolCallFunction{Name: "func2", Arguments: `{"y": 2}`}},
			{ID: "", Function: ToolCallFunction{Name: "func2", Arguments: `{"y": 2}`}}, // Duplicate by name+args
		}
		result := DeduplicateToolCalls(toolCalls)

		// call_1 with arguments + func2 once
		assert.Len(t, result, 2)

		// Verify that call_1 kept its arguments
		var call1 *ToolCall
		for i := range result {
			if result[i].ID == "call_1" {
				call1 = &result[i]
				break
			}
		}
		assert.NotNil(t, call1)
		assert.Equal(t, `{"x": 1}`, call1.Function.Arguments)
	})
}

// =============================================================================
// TestAwsEventStreamParserInitialization
// Original: /code/github/kiro-gateway/tests/unit/test_parsers.py::TestAwsEventStreamParserInitialization
// =============================================================================

func TestAwsEventStreamParser_Initialization(t *testing.T) {
	// Original: test_initialization_creates_empty_state
	parser := NewAwsEventStreamParser()

	assert.Equal(t, "", parser.buffer)
	assert.Nil(t, parser.lastContent)
	assert.Nil(t, parser.currentToolCall)
	assert.Empty(t, parser.toolCalls)
}

// =============================================================================
// TestAwsEventStreamParserFeed
// Original: /code/github/kiro-gateway/tests/unit/test_parsers.py::TestAwsEventStreamParserFeed
// =============================================================================

func TestAwsEventStreamParser_Feed(t *testing.T) {
	t.Run("parses content event", func(t *testing.T) {
		// Original: test_parses_content_event
		parser := NewAwsEventStreamParser()
		chunk := []byte(`{"content":"Hello World"}`)

		events := parser.Feed(chunk)

		assert.Len(t, events, 1)
		assert.Equal(t, EventTypeContent, events[0].Type)
		assert.Equal(t, "Hello World", events[0].Data.(ContentData).Content)
	})

	t.Run("parses multiple content events", func(t *testing.T) {
		// Original: test_parses_multiple_content_events
		parser := NewAwsEventStreamParser()
		chunk := []byte(`{"content":"First"}{"content":"Second"}`)

		events := parser.Feed(chunk)

		assert.Len(t, events, 2)
		assert.Equal(t, "First", events[0].Data.(ContentData).Content)
		assert.Equal(t, "Second", events[1].Data.(ContentData).Content)
	})

	t.Run("deduplicates repeated content", func(t *testing.T) {
		// Original: test_deduplicates_repeated_content
		parser := NewAwsEventStreamParser()

		events1 := parser.Feed([]byte(`{"content":"Same"}`))
		events2 := parser.Feed([]byte(`{"content":"Same"}`))

		assert.Len(t, events1, 1)
		assert.Len(t, events2, 0) // Duplicate filtered out
	})

	t.Run("parses usage event", func(t *testing.T) {
		// Original: test_parses_usage_event
		// Note: Go version uses int for Credits, Python uses float
		parser := NewAwsEventStreamParser()
		chunk := []byte(`{"usage":42}`)

		events := parser.Feed(chunk)

		assert.Len(t, events, 1)
		assert.Equal(t, EventTypeUsage, events[0].Type)
		assert.Equal(t, 42, events[0].Data.(UsageData).Credits)
	})

	t.Run("parses context usage event", func(t *testing.T) {
		// Original: test_parses_context_usage_event
		parser := NewAwsEventStreamParser()
		chunk := []byte(`{"contextUsagePercentage":25.5}`)

		events := parser.Feed(chunk)

		assert.Len(t, events, 1)
		assert.Equal(t, EventTypeContextUsage, events[0].Type)
		assert.Equal(t, 25.5, events[0].Data.(ContextUsageData).Percentage)
	})

	t.Run("handles incomplete JSON", func(t *testing.T) {
		// Original: test_handles_incomplete_json
		parser := NewAwsEventStreamParser()
		chunk := []byte(`{"content":"Hel`)

		events := parser.Feed(chunk)

		assert.Len(t, events, 0) // Nothing parsed
		assert.Contains(t, parser.buffer, "content")
	})

	t.Run("completes JSON across chunks", func(t *testing.T) {
		// Original: test_completes_json_across_chunks
		parser := NewAwsEventStreamParser()

		events1 := parser.Feed([]byte(`{"content":"Hel`))
		events2 := parser.Feed([]byte(`lo World"}`))

		assert.Len(t, events1, 0)
		assert.Len(t, events2, 1)
		assert.Equal(t, "Hello World", events2[0].Data.(ContentData).Content)
	})

	t.Run("decodes escape sequences", func(t *testing.T) {
		// Original: test_decodes_escape_sequences
		parser := NewAwsEventStreamParser()
		chunk := []byte(`{"content":"Line1\nLine2"}`)

		events := parser.Feed(chunk)

		assert.Len(t, events, 1)
		assert.Contains(t, events[0].Data.(ContentData).Content, "\n")
	})

	t.Run("handles invalid bytes", func(t *testing.T) {
		// Original: test_handles_invalid_bytes
		parser := NewAwsEventStreamParser()
		chunk := []byte{0xff, 0xfe, '{', '"', 'c', 'o', 'n', 't', 'e', 'n', 't', '"', ':', '"', 't', 'e', 's', 't', '"', '}'}

		events := parser.Feed(chunk)

		// Parser should continue working
		assert.Len(t, events, 1)
	})
}

// =============================================================================
// TestAwsEventStreamParserToolCalls
// Original: /code/github/kiro-gateway/tests/unit/test_parsers.py::TestAwsEventStreamParserToolCalls
// =============================================================================

func TestAwsEventStreamParser_ToolCalls(t *testing.T) {
	t.Run("parses tool start event", func(t *testing.T) {
		// Original: test_parses_tool_start_event
		parser := NewAwsEventStreamParser()
		chunk := []byte(`{"name":"get_weather","toolUseId":"call_123"}`)

		events := parser.Feed(chunk)

		// tool_start doesn't return event, but creates currentToolCall
		assert.Len(t, events, 0)
		assert.NotNil(t, parser.currentToolCall)
		assert.Equal(t, "get_weather", parser.currentToolCall.Function.Name)
	})

	t.Run("parses tool input event", func(t *testing.T) {
		// Original: test_parses_tool_input_event
		parser := NewAwsEventStreamParser()
		parser.Feed([]byte(`{"name":"func","toolUseId":"call_1"}`))
		parser.Feed([]byte(`{"input":"{\"key\": \"value\"}"}`))

		assert.Contains(t, parser.currentToolCall.Function.Arguments, "key")
	})

	t.Run("parses tool stop event", func(t *testing.T) {
		// Original: test_parses_tool_stop_event
		parser := NewAwsEventStreamParser()
		parser.Feed([]byte(`{"name":"func","toolUseId":"call_1"}`))
		parser.Feed([]byte(`{"input":"{}"}`))
		parser.Feed([]byte(`{"stop":true}`))

		assert.Len(t, parser.toolCalls, 1)
		assert.Nil(t, parser.currentToolCall)
	})

	t.Run("get tool calls returns all", func(t *testing.T) {
		// Original: test_get_tool_calls_returns_all
		parser := NewAwsEventStreamParser()
		parser.Feed([]byte(`{"name":"func1","toolUseId":"call_1"}`))
		parser.Feed([]byte(`{"stop":true}`))
		parser.Feed([]byte(`{"name":"func2","toolUseId":"call_2"}`))
		parser.Feed([]byte(`{"stop":true}`))

		toolCalls := parser.GetToolCalls()

		assert.Len(t, toolCalls, 2)
	})

	t.Run("get tool calls finalizes current", func(t *testing.T) {
		// Original: test_get_tool_calls_finalizes_current
		parser := NewAwsEventStreamParser()
		parser.Feed([]byte(`{"name":"func","toolUseId":"call_1"}`))

		toolCalls := parser.GetToolCalls()

		assert.Len(t, toolCalls, 1)
		assert.Nil(t, parser.currentToolCall)
	})
}

// =============================================================================
// TestAwsEventStreamParserReset
// Original: /code/github/kiro-gateway/tests/unit/test_parsers.py::TestAwsEventStreamParserReset
// =============================================================================

func TestAwsEventStreamParser_Reset(t *testing.T) {
	// Original: test_reset_clears_state
	parser := NewAwsEventStreamParser()
	parser.Feed([]byte(`{"content":"test"}`))
	parser.Feed([]byte(`{"name":"func","toolUseId":"call_1"}`))

	parser.Reset()

	assert.Equal(t, "", parser.buffer)
	assert.Nil(t, parser.lastContent)
	assert.Nil(t, parser.currentToolCall)
	assert.Empty(t, parser.toolCalls)
}

// =============================================================================
// TestAwsEventStreamParserEdgeCases
// Original: /code/github/kiro-gateway/tests/unit/test_parsers.py::TestAwsEventStreamParserEdgeCases
// =============================================================================

func TestAwsEventStreamParser_EdgeCases(t *testing.T) {
	t.Run("handles followup prompt", func(t *testing.T) {
		// Original: test_handles_followup_prompt
		parser := NewAwsEventStreamParser()
		chunk := []byte(`{"content":"text","followupPrompt":"suggestion"}`)

		events := parser.Feed(chunk)

		assert.Len(t, events, 0) // followupPrompt is ignored
	})

	t.Run("handles mixed events", func(t *testing.T) {
		// Original: test_handles_mixed_events
		parser := NewAwsEventStreamParser()
		// Note: Go version uses int for usage, Python uses float
		chunk := []byte(`{"content":"Hello"}{"usage":42}{"contextUsagePercentage":50}`)

		events := parser.Feed(chunk)

		assert.Len(t, events, 3)
		assert.Equal(t, EventTypeContent, events[0].Type)
		assert.Equal(t, EventTypeUsage, events[1].Type)
		assert.Equal(t, EventTypeContextUsage, events[2].Type)
	})

	t.Run("handles garbage between events", func(t *testing.T) {
		// Original: test_handles_garbage_between_events
		parser := NewAwsEventStreamParser()
		chunk := []byte(`garbage{"content":"valid"}more garbage{"usage":42}`)

		events := parser.Feed(chunk)

		assert.Len(t, events, 2)
	})

	t.Run("handles empty chunk", func(t *testing.T) {
		// Original: test_handles_empty_chunk
		parser := NewAwsEventStreamParser()

		events := parser.Feed([]byte{})

		assert.Empty(t, events)
	})
}
