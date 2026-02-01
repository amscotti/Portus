// Package models defines the data structures used throughout Portus.
package models

import (
	"encoding/json"
	"time"
)

// Version is the current Portus version.
const Version = "1.0.0"

// ModelConfig represents a single model alias configuration.
type ModelConfig struct {
	Provider        string                 `json:"provider,omitempty"`
	APIKey          string                 `json:"api_key,omitempty"`
	Strategy        *StrategyConfig        `json:"strategy,omitempty"`
	Targets         []TargetConfig         `json:"targets,omitempty"`
	OverrideParams  map[string]interface{} `json:"override_params,omitempty"`
	Retry           *RetryConfig           `json:"retry,omitempty"`
	// RequestTimeout is the request timeout in milliseconds.
	RequestTimeout int `json:"request_timeout,omitempty"`
	Thinking        *ThinkingConfig        `json:"thinking,omitempty"`
	BetaHeaders     []string               `json:"beta_headers,omitempty"`
	ReasoningEffort string                 `json:"reasoning_effort,omitempty"`
	ThinkingLevel   string                 `json:"thinking_level,omitempty"`

	// AWS Bedrock specific
	AWSAccessKeyID     string `json:"aws_access_key_id,omitempty"`
	AWSSecretAccessKey string `json:"aws_secret_access_key,omitempty"`
	AWSRegion          string `json:"aws_region,omitempty"`
	AWSSessionToken    string `json:"aws_session_token,omitempty"`

	// Vertex AI specific
	VertexProjectID          string `json:"vertex_project_id,omitempty"`
	VertexRegion             string `json:"vertex_region,omitempty"`
	VertexServiceAccountJSON string `json:"vertex_service_account_json,omitempty"`
}

// StrategyConfig defines the routing strategy (single, fallback, loadbalance).
type StrategyConfig struct {
	Mode          string `json:"mode"`
	OnStatusCodes []int  `json:"on_status_codes,omitempty"`
}

// TargetConfig represents a single target in a multi-target configuration.
type TargetConfig struct {
	Provider       string                 `json:"provider"`
	APIKey         string                 `json:"api_key,omitempty"`
	OverrideParams map[string]interface{} `json:"override_params,omitempty"`
	Weight         int                    `json:"weight,omitempty"`

	// AWS Bedrock specific
	AWSAccessKeyID     string `json:"aws_access_key_id,omitempty"`
	AWSSecretAccessKey string `json:"aws_secret_access_key,omitempty"`
	AWSRegion          string `json:"aws_region,omitempty"`
	AWSSessionToken    string `json:"aws_session_token,omitempty"`
}

// RetryConfig defines retry behavior.
type RetryConfig struct {
	Attempts      int   `json:"attempts"`
	OnStatusCodes []int `json:"on_status_codes,omitempty"`
}

// ThinkingConfig defines extended thinking for Anthropic models.
type ThinkingConfig struct {
	Type         string `json:"type"`
	BudgetTokens int    `json:"budget_tokens"`
}

// ProxyKey represents an authorized proxy key with its associated application name.
type ProxyKey struct {
	Key         string
	Application string
}

// ConfigStore holds all loaded configuration in memory.
type ConfigStore struct {
	Models     map[string]ModelConfig
	ProxyKeys  []ProxyKey
	ServerPort int
	ConfigPath string
	GatewayURL string
	LogLevel   string
	StartTime  time.Time

	// RawConfigs holds the raw (pre-expansion) JSON content of each model config file,
	// keyed by alias. Used during validation to check for missing env vars without
	// re-reading files. Cleared after validation.
	RawConfigs map[string]string
}

// PortkeyConfig is the configuration structure sent to Portkey Gateway.
type PortkeyConfig struct {
	Provider       string                 `json:"provider,omitempty"`
	APIKey         string                 `json:"api_key,omitempty"`
	Strategy       *StrategyConfig        `json:"strategy,omitempty"`
	Targets        []TargetConfig         `json:"targets,omitempty"`
	OverrideParams map[string]interface{} `json:"override_params,omitempty"`
	Retry          *RetryConfig           `json:"retry,omitempty"`
	RequestTimeout int                    `json:"request_timeout,omitempty"`

	// AWS Bedrock specific
	AWSAccessKeyID     string `json:"aws_access_key_id,omitempty"`
	AWSSecretAccessKey string `json:"aws_secret_access_key,omitempty"`
	AWSRegion          string `json:"aws_region,omitempty"`
	AWSSessionToken    string `json:"aws_session_token,omitempty"`

	// Vertex AI specific
	VertexProjectID          string `json:"vertex_project_id,omitempty"`
	VertexRegion             string `json:"vertex_region,omitempty"`
	VertexServiceAccountJSON string `json:"vertex_service_account_json,omitempty"`
}

// ToJSON serializes the config to JSON for the x-portkey-config header.
func (pc *PortkeyConfig) ToJSON() (string, error) {
	bytes, err := json.Marshal(pc)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// HealthResponse represents the health check response.
type HealthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
	Uptime  string `json:"uptime"`
}

// ModelsListResponse represents the OpenAI-compatible models list.
type ModelsListResponse struct {
	Object string        `json:"object"`
	Data   []ModelObject `json:"data"`
}

// ModelObject represents a single model in the list.
type ModelObject struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

// ChatCompletionRequest represents an OpenAI chat completion request.
type ChatCompletionRequest struct {
	Model           string    `json:"model"`
	Messages        []Message `json:"messages"`
	Stream          bool      `json:"stream,omitempty"`
	MaxTokens       int       `json:"max_tokens,omitempty"`
	Temperature     float64   `json:"temperature,omitempty"`
	TopP            float64   `json:"top_p,omitempty"`
	ReasoningEffort string    `json:"reasoning_effort,omitempty"`
	// Additional fields can be added as needed
}

// Message represents a chat message.
type Message struct {
	Role      string      `json:"role"`
	Content   interface{} `json:"content"` // Can be string or array of content parts
	ToolCalls []ToolCall  `json:"tool_calls,omitempty"`
}

// ToolCall represents a tool/function call.
type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// MessagesRequest represents an Anthropic messages request.
type MessagesRequest struct {
	Model       string             `json:"model"`
	Messages    []AnthropicMessage `json:"messages"`
	System      string             `json:"system,omitempty"`
	MaxTokens   int                `json:"max_tokens"`
	Stream      bool               `json:"stream,omitempty"`
	Temperature float64            `json:"temperature,omitempty"`
	TopP        float64            `json:"top_p,omitempty"`
	Thinking    *ThinkingConfig    `json:"thinking,omitempty"`
}

// AnthropicMessage represents a message in Anthropic format.
type AnthropicMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"` // Can be string or array of content blocks
}

// LogEntry represents a request log entry.
type LogEntry struct {
	Timestamp   string `json:"timestamp"`
	RequestID   string `json:"request_id"`
	Application string `json:"application"`
	ModelAlias  string `json:"model_alias"`
	Provider    string `json:"provider"`
	StatusCode  int    `json:"status_code"`
	DurationMs  int64  `json:"duration_ms"`
}
