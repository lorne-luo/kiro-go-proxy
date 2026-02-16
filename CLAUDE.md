# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.
This Golang project is migrated from python project https://github.com/jwadow/kiro-gateway.git

## Build and Run Commands

```bash
# Run directly (development)
go run .

# Build executable
go build -o kiro-go-proxy .

# Run with custom host/port
./kiro-go-proxy --host 127.0.0.1 --port 8080

# Show version
./kiro-go-proxy --version

# Download dependencies
go mod download
```

## Testing

```bash
# Run all tests
go test ./...

# Run tests with verbose output
go test -v ./...

# Run tests for a specific package
go test ./model/...
go test ./parser/...
go test ./converter/...

# Run a specific test
go test -v -run TestNormalizeModelName ./model/...
go test -v -run TestAwsEventStreamParserFeed ./parser/...

# Run tests with coverage
go test -cover ./...
```

Test framework: `github.com/stretchr/testify/assert` - use `assert.Equal`, `assert.Len`, `assert.True`, etc.

## Configuration

Configuration is loaded from environment variables and `.env` file. Copy `.env.example` to `.env` and configure:

- **Required**: One of `REFRESH_TOKEN`, `KIRO_CREDS_FILE`, or `KIRO_CLI_DB_FILE`
- **Required**: `PROXY_API_KEY` - password clients use to access the proxy
- Key settings: `SERVER_HOST`, `SERVER_PORT`, `KIRO_REGION`, `LOG_LEVEL`, `DEBUG_MODE`

## Architecture

This is a Go proxy gateway that provides OpenAI-compatible (`/v1/chat/completions`) and Anthropic-compatible (`/v1/messages`) API interfaces for the Kiro API (Amazon Q Developer / AWS CodeWhisperer backend).

### Request Flow

```
Client Request (OpenAI/Anthropic format)
    ↓
api/routes.go - Parse request, resolve model
    ↓
converter/core.go - Convert to UnifiedMessage, then to KiroPayload
    ↓
client/http.go - POST to Kiro API with auth token
    ↓
parser/parser.go - Parse AWS Event Stream binary response
    ↓
stream/stream.go - Convert to OpenAI/Anthropic SSE format
    ↓
Client Response
```

### Key Packages

| Package | Purpose |
|---------|---------|
| `api/routes.go` | HTTP routes, handlers, streaming orchestration |
| `auth/auth.go` | Token lifecycle (Kiro Desktop, AWS SSO OIDC), supports 3 credential methods |
| `config/config.go` | Configuration from environment, URL templates |
| `converter/core.go` | Unified message format, Kiro payload builder, message processing |
| `converter/openai.go` | OpenAI-specific types and conversions |
| `parser/parser.go` | AWS Event Stream binary parser, tool call extraction |
| `parser/thinking.go` | FSM parser for `<thinking>` blocks |
| `stream/stream.go` | SSE streaming for both OpenAI and Anthropic formats |
| `model/resolver.go` | 4-layer model name resolution: alias → normalize → cache → hidden → passthrough |
| `client/http.go` | HTTP client with retry logic for 403/429/5xx errors |

### Key Types

- `converter.UnifiedMessage` - Internal message format with Role, Content, ToolCalls, ToolResults, Images
- `converter.KiroPayload` - Request payload sent to Kiro API
- `parser.Event` - Parsed event from AWS Event Stream (content, tool_start, tool_input, etc.)
- `model.Resolution` - Model resolution result with InternalID, Source, Normalized fields
- `stream.KiroEvent` - Unified streaming event (content, thinking, tool_use, usage)

### Model Resolution Pipeline

The `model/resolver.go` implements a 4-layer resolution pipeline:
1. **Alias resolution** - e.g., `auto-kiro` → `auto`
2. **Normalization** - Standardizes model names (e.g., `claude-sonnet-4-5` → `claude-sonnet-4.5`)
3. **Dynamic cache** - Models fetched from Kiro API
4. **Hidden models** - Pre-configured internal model mappings
5. **Passthrough** - Unknown models passed to Kiro API

**Critical Safety Principle**: Model family (haiku/sonnet/opus) must NEVER change during resolution. Tests in `model/resolver_test.go` enforce this.

### Authentication Types

`auth/auth.go` supports two auth types:
- **Kiro Desktop**: Simple refresh token flow via `prod.{region}.auth.desktop.kiro.dev`
- **AWS SSO OIDC**: Device registration + OIDC token flow (for kiro-cli)

Thread-safe token management with `sync.RWMutex` for concurrent access.

### Message Processing

`converter/core.go` processes messages through:
1. Tool description length handling (moves long descriptions to system prompt)
2. Thinking mode tag injection (for extended thinking)
3. Tool content stripping (when no tools in request)
4. Adjacent message merging
5. Role normalization and alternating enforcement

### AWS Event Stream Parsing

`parser/parser.go` handles binary SSE responses:
- JSON objects arrive sequentially without delimiters
- `FindMatchingBrace()` locates complete JSON objects
- Handles incomplete JSON across chunks (buffer accumulation)
- Deduplicates repeated content events
- Extracts tool calls from both structured events and `[Called func with args: {...}]` format

## Debugging

Enable verbose logging:
```env
LOG_LEVEL=DEBUG
DEBUG_MODE=all
DEBUG_DIR=debug_logs
```

## Code Patterns

### Error Handling
- Return errors with context: `fmt.Errorf("failed to X: %w", err)`
- Log errors with `log.Errorf()` or `log.Warnf()`
- HTTP errors return JSON with `gin.H{"error": gin.H{"message": ..., "type": ...}}`

### JSON Handling
- Use `json.Marshal`/`json.Unmarshal` for serialization
- `json.RawMessage` for raw JSON preservation
- Handle both `string` and `map[string]interface{}` for flexible input

### Concurrency
- Use `sync.RWMutex` for shared state (see `auth/auth.go`, `model/resolver.go`)
- Channels for streaming: `<-chan KiroEvent`, `<-chan string`
- `context.Context` for cancellation

### Testing
- Table-driven tests with `t.Run()` for subtests
- Helper functions return test fixtures (e.g., `newTestConfig()`, `newTestResolver()`)
- Use `assert` package: `assert.Equal(t, expected, actual)`

## Manual Testing

The `test.sh` file contains curl examples:
```bash
# Basic test
curl http://localhost:8000/v1/chat/completions \
  -H "Authorization: Bearer my-secret-password" \
  -H "Content-Type: application/json" \
  -d '{"model": "claude-sonnet-4.5", "messages": [{"role": "user", "content": "Hello"}]}'
```
