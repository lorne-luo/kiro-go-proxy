# Kiro Gateway (Go)

A Go implementation of the Kiro Gateway proxy server that provides OpenAI-compatible and Anthropic-compatible API interfaces for the Kiro API (Amazon Q Developer / AWS CodeWhisperer backend).

## Features

- **Dual API Support**: OpenAI-compatible (`/v1/chat/completions`) and Anthropic-compatible (`/v1/messages`) endpoints
- **Smart Model Resolution**: Normalizes model names, resolves aliases, handles hidden models
- **Extended Thinking**: Fake reasoning via tag injection for extended thinking mode
- **Vision Support**: Image processing through multimodal content
- **Tool Calling**: Full function calling support with OpenAI and Anthropic formats
- **Streaming**: SSE streaming with proper chunk formatting
- **Automatic Retry**: Handles 403 (token refresh), 429 (rate limit), 5xx errors with exponential backoff
- **VPN/Proxy Support**: HTTP/SOCKS5 proxy for restricted networks
- **Token Management**: Automatic token refresh before expiration

---

## How to Run

### Method 1: Run Directly with Go

```bash
# 1. Navigate to the project directory
cd /code/lorne/kiro-go-proxy

# 2. Download dependencies (only needed once)
go mod download

# 3. Create your .env file from the example
cp .env.example .env

# 4. Edit .env and add your Kiro credentials
# At minimum, set REFRESH_TOKEN or one of the other auth methods
nano .env  # or use your preferred editor

# 5. Run the server
go run .
```

### Method 2: Build and Run the Executable

```bash
# 1. Build the executable
go build -o kiro-gateway .

# 2. Run it
./kiro-gateway

# Optional: Specify host and port via command line
./kiro-gateway --host 127.0.0.1 --port 8080

# Optional: Show version
./kiro-gateway --version
```

### Method 3: Run with Environment Variables

```bash
# Set environment variables directly
export REFRESH_TOKEN="your_kiro_refresh_token"
export PROXY_API_KEY="your-secure-password"
export SERVER_PORT=8000

# Run
go run .
```

---

## Configuration

### Required Configuration

Create a `.env` file in the project root (copy from `.env.example`):

```bash
cp .env.example .env
```

**Minimum required settings:**

```env
# Your Kiro refresh token (get from Kiro IDE)
REFRESH_TOKEN=your_refresh_token_here

# Password that clients must use to access this proxy
PROXY_API_KEY=my-secret-password
```

### Authentication Methods (choose one)

#### Method 1: Refresh Token (Simplest)
```env
REFRESH_TOKEN=your_kiro_refresh_token_here
```

#### Method 2: Credentials File
```env
KIRO_CREDS_FILE=/path/to/credentials.json
```

Create `credentials.json`:
```json
{
  "refreshToken": "your_refresh_token",
  "accessToken": "your_access_token",
  "profileArn": "arn:aws:codewhisperer:us-east-1:xxx:profile/xxx",
  "region": "us-east-1"
}
```

#### Method 3: kiro-cli SQLite Database
```env
KIRO_CLI_DB_FILE=~/.kiro-cli/auth.db
```

### All Configuration Options

| Variable | Description | Default |
|----------|-------------|---------|
| `SERVER_HOST` | Server host address | `0.0.0.0` |
| `SERVER_PORT` | Server port | `8000` |
| `PROXY_API_KEY` | Password for proxy access | `my-super-secret-password-123` |
| `REFRESH_TOKEN` | Kiro refresh token | (optional) |
| `KIRO_CREDS_FILE` | Path to credentials JSON file | (optional) |
| `KIRO_CLI_DB_FILE` | Path to kiro-cli SQLite database | (optional) |
| `PROFILE_ARN` | AWS CodeWhisperer profile ARN | (optional) |
| `KIRO_REGION` | AWS region | `us-east-1` |
| `VPN_PROXY_URL` | Proxy URL for restricted networks | (optional) |
| `TOKEN_REFRESH_THRESHOLD` | Seconds before expiry to refresh token | `600` |
| `MAX_RETRIES` | Max retry attempts | `3` |
| `BASE_RETRY_DELAY` | Base delay between retries (seconds) | `1.0` |
| `FIRST_TOKEN_TIMEOUT` | Timeout for first token (seconds) | `15` |
| `FIRST_TOKEN_MAX_RETRIES` | Max retries for first token timeout | `3` |
| `STREAMING_READ_TIMEOUT` | Streaming timeout (seconds) | `300` |
| `MODEL_CACHE_TTL` | Model cache TTL (seconds) | `3600` |
| `FAKE_REASONING` | Enable extended thinking | `true` |
| `FAKE_REASONING_MAX_TOKENS` | Max thinking tokens | `4000` |
| `FAKE_REASONING_HANDLING` | How to handle thinking content | `as_reasoning_content` |
| `LOG_LEVEL` | Logging level (DEBUG/INFO/WARNING/ERROR) | `INFO` |
| `DEBUG_MODE` | Debug mode (off/errors/all) | `off` |
| `TOOL_DESCRIPTION_MAX_LENGTH` | Max tool description length | `10000` |
| `TRUNCATION_RECOVERY` | Enable truncation recovery | `true` |

---

## API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/` | GET | Health check |
| `/health` | GET | Detailed health check with timestamp |
| `/v1/models` | GET | List available models (OpenAI format) |
| `/v1/chat/completions` | POST | Chat completions (OpenAI format) |
| `/v1/messages` | POST | Messages API (Anthropic format) |

---

## Usage Examples

### Test with curl

```bash
# Health check
curl http://localhost:8000/health

# List models
curl http://localhost:8000/v1/models \
  -H "Authorization: Bearer my-secret-password"

# Chat completion (OpenAI format)
curl http://localhost:8000/v1/chat/completions \
  -H "Authorization: Bearer my-secret-password" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-sonnet-4.5",
    "messages": [{"role": "user", "content": "Hello, how are you?"}]
  }'

# Chat completion (Anthropic format)
curl http://localhost:8000/v1/messages \
  -H "x-api-key: my-secret-password" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-sonnet-4.5",
    "max_tokens": 1024,
    "messages": [{"role": "user", "content": "Hello, how are you?"}]
  }'
```

### Python - OpenAI SDK

```python
from openai import OpenAI

client = OpenAI(
    base_url="http://localhost:8000/v1",
    api_key="my-secret-password"  # Same as PROXY_API_KEY
)

# Non-streaming
response = client.chat.completions.create(
    model="claude-sonnet-4.5",
    messages=[
        {"role": "user", "content": "Write a hello world program in Go"}
    ]
)
print(response.choices[0].message.content)

# Streaming
for chunk in client.chat.completions.create(
    model="claude-sonnet-4.5",
    messages=[{"role": "user", "content": "Count from 1 to 10"}],
    stream=True
):
    if chunk.choices[0].delta.content:
        print(chunk.choices[0].delta.content, end="", flush=True)
```

### Python - Anthropic SDK

```python
from anthropic import Anthropic

client = Anthropic(
    base_url="http://localhost:8000",
    api_key="my-secret-password"
)

# Non-streaming
message = client.messages.create(
    model="claude-sonnet-4.5",
    max_tokens=1024,
    messages=[
        {"role": "user", "content": "Write a hello world program in Go"}
    ]
)
print(message.content[0].text)

# Streaming
with client.messages.stream(
    model="claude-sonnet-4.5",
    max_tokens=1024,
    messages=[{"role": "user", "content": "Count from 1 to 10"}]
) as stream:
    for text in stream.text_stream:
        print(text, end="", flush=True)
```

### Tool Calling Example

```python
from openai import OpenAI

client = OpenAI(
    base_url="http://localhost:8000/v1",
    api_key="my-secret-password"
)

response = client.chat.completions.create(
    model="claude-sonnet-4.5",
    messages=[{"role": "user", "content": "What's the weather in Tokyo?"}],
    tools=[{
        "type": "function",
        "function": {
            "name": "get_weather",
            "description": "Get the current weather for a city",
            "parameters": {
                "type": "object",
                "properties": {
                    "city": {"type": "string", "description": "City name"}
                },
                "required": ["city"]
            }
        }
    }]
)

# Check if model wants to call a tool
if response.choices[0].message.tool_calls:
    for tool_call in response.choices[0].message.tool_calls:
        print(f"Tool: {tool_call.function.name}")
        print(f"Args: {tool_call.function.arguments}")
```

---

## Supported Models

- `claude-sonnet-4.5` - Latest Sonnet model
- `claude-haiku-4.5` - Fast, lightweight model
- `claude-opus-4.5` - Most capable model
- `claude-sonnet-4` - Previous generation
- `auto` - Smart model selection
- And more...

---

## Docker

Create a `Dockerfile`:

```dockerfile
FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o kiro-gateway .

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/kiro-gateway .
COPY --from=builder /app/.env.example .env
EXPOSE 8000
CMD ["./kiro-gateway"]
```

Build and run:

```bash
docker build -t kiro-gateway .
docker run -p 8000:8000 \
  -e REFRESH_TOKEN="your_token" \
  -e PROXY_API_KEY="your_password" \
  kiro-gateway
```

---

## Project Structure

```
kiro-go-proxy/
├── main.go              # Application entry point
├── go.mod               # Go module definition
├── go.sum               # Dependencies checksum
├── .env.example         # Environment configuration template
├── README.md            # This file
│
├── api/
│   └── routes.go        # HTTP routes and handlers
│
├── auth/
│   └── auth.go          # Authentication management (Kiro Desktop, AWS SSO OIDC)
│
├── client/
│   └── http.go          # HTTP client with retry logic
│
├── config/
│   └── config.go        # Configuration management
│
├── converter/
│   ├── core.go          # Core conversion logic (unified message format)
│   └── openai.go        # OpenAI format models and conversion
│
├── model/
│   └── resolver.go      # Model resolution, normalization, and caching
│
├── parser/
│   ├── parser.go        # AWS Event Stream binary parser
│   └── thinking.go      # Thinking/reasoning block FSM parser
│
├── stream/
│   └── stream.go        # SSE streaming for OpenAI and Anthropic formats
│
└── utils/
    └── utils.go         # Utility functions (IDs, JSON schema, etc.)
```

---

## Troubleshooting

### Server won't start

1. Check that you have Go 1.23+ installed:
   ```bash
   go version
   ```

2. Verify your `.env` file exists and has valid credentials:
   ```bash
   cat .env
   ```

### Authentication errors

1. Verify your refresh token is valid
2. Try refreshing your token in the Kiro IDE
3. Check that `KIRO_REGION` matches your token's region

### Connection refused

1. Check that the server is running on the expected port
2. Verify `SERVER_HOST` and `SERVER_PORT` in your `.env`
3. Check firewall settings

### Enable debug logging

```env
LOG_LEVEL=DEBUG
DEBUG_MODE=all
```

---

## License

GNU Affero General Public License v3.0 (AGPL-3.0)

## Credits

This is a Go port of the original [kiro-gateway](https://github.com/jwadow/kiro-gateway) Python project.
