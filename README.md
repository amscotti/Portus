<p align="center">
  <img src="images/Portus.png" alt="Portus Logo" width="300">
</p>

# Portus

A lightweight Go-based configuration proxy for the **Portkey Gateway**. Portus simplifies AI model routing by allowing clients to use abstract model aliases (e.g., `claude-sonnet`) instead of managing complex provider configurations directly.

## Quick Start

### Prerequisites

- **Go 1.25+** (for local development)
- **Docker & Docker Compose** (for containerized deployment)
- API keys for providers you want to use (Anthropic, OpenAI, AWS Bedrock, Google Vertex AI)

### Environment Setup

Create a `.env` file:

```bash
# Portus Configuration
PORTUS_PORT=8080
PORTUS_CONFIG_PATH=./config
PORTKEY_GATEWAY_URL=http://localhost:8787

# Proxy Keys (Format: PORTUS_KEY_APP_NAME=key)
PORTUS_KEY_DEV=pk-dev-xxxxx
PORTUS_KEY_BACKEND=pk-backend-xxxxx

# Provider API Keys
ANTHROPIC_API_KEY=sk-ant-xxxxx
OPENAI_API_KEY=sk-xxxxx

# Google Vertex AI (optional)
GCP_PROJECT_ID=my-project
GCP_REGION=us-central1
GCP_SERVICE_ACCOUNT_JSON={"type":"service_account",...}
```

### Running with Docker Compose

The easiest way to get started (starts both Portkey Gateway and Portus):

```bash
docker-compose up -d
```

### Running Locally

1. Start Portkey Gateway:
```bash
npx @portkey-ai/gateway
```

2. Load environment and start Portus:
```bash
export $(grep -v '^#' .env | xargs)
go run ./cmd/portus
```

## Deployment via Docker

To run Portus using a container, you must mount your configuration directory and provide the necessary environment variables.

```bash
docker run -d \
  -p 8080:8080 \
  -v $(pwd)/config:/app/config:ro \
  -e PORTUS_CONFIG_PATH=/app/config \
  -e PORTKEY_GATEWAY_URL=http://your-portkey-gateway:8787 \
  -e PORTUS_KEY_MYAPP=pk-secret-key \
  -e OPENAI_API_KEY=sk-xxxxx \
  ghcr.io/amscotti/portus:latest
```

## Configuration

Model aliases are defined in JSON files in `config/models/`. The filename (minus `.json`) becomes the alias name.

### Example: Anthropic with Tool Use (`config/models/claude-sonnet.json`)
```json
{
  "provider": "anthropic",
  "api_key": "${ANTHROPIC_API_KEY}",
  "override_params": {
    "model": "claude-sonnet-4-5-20250929",
    "max_tokens": 4096
  },
  "beta_headers": ["prompt-caching-2024-07-31"]
}
```

### Example: Google Vertex AI (`config/models/gemini.json`)
```json
{
  "provider": "vertex-ai",
  "vertex_project_id": "${GCP_PROJECT_ID}",
  "vertex_region": "${GCP_REGION}",
  "vertex_service_account_json": "${GCP_SERVICE_ACCOUNT_JSON}",
  "override_params": {
    "model": "gemini-3-pro-preview"
  }
}
```

## API Usage

### Health Check
```bash
curl http://localhost:8080/health
```

### List Models
```bash
curl http://localhost:8080/v1/models \
  -H "Authorization: Bearer pk-dev-xxxxx"
```

### Chat Completions (OpenAI format)
```bash
curl http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer pk-dev-xxxxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-sonnet",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

### Messages (Anthropic format)
```bash
curl http://localhost:8080/v1/messages \
  -H "x-api-key: pk-dev-xxxxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-sonnet",
    "messages": [{"role": "user", "content": "Hello!"}],
    "max_tokens": 1024
  }'
```

## Architecture

```
┌──────────────┐      ┌──────────┐      ┌───────────────┐      ┌──────────────┐
│   Client     │─────▶│  Portus  │─────▶│Portkey Gateway│─────▶│   Provider   │
│   (SDK)      │      │  Proxy   │      │  (Port 8787)  │      │  (Anthropic, │
└──────────────┘      └──────────┘      └───────────────┘      │ OpenAI, etc) │
                                                               └──────────────┘
```

## Features

- **Model Aliases**: Abstract complex provider configurations with simple names.
- **Proxy Keys**: Authenticate applications via `PORTUS_KEY_*` environment variables.
- **Protocol Translation**: Supports both OpenAI and Anthropic SDK formats.
- **Tool Use**: Full support for tool/function calling and Anthropic's `toolRunner`.
- **Reliability**: Automatic retries and fallback strategies via Portkey Gateway.
- **Vertex AI Support**: Automated handling of Google Vertex AI service account authentication.
- **Streaming**: Native support for streaming responses with robust cancellation handling.
- **Zero-Dependency Core**: Built using only the Go standard library for the core logic.

## Development

### Project Structure

```
.
├── cmd/portus/          # Main application entry point
├── internal/
│   ├── config/         # Configuration loading and validation
│   ├── handlers/       # HTTP request handlers (unified proxy logic)
│   ├── middleware/     # Auth, logging, request ID, and recovery
│   └── models/         # Shared data models
├── config/models/      # Model configuration JSON files
├── Dockerfile          # Multi-stage container build
└── docker-compose.yml  # Full stack development environment
```

### Building

```bash
go build -o portus ./cmd/portus
```

### Testing

#### Unit Tests
```bash
go test ./...
```

## License

Apache License 2.0 - See LICENSE file for details.
