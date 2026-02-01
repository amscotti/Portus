package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/amscotti/portus/internal/models"
)

func TestWriteJSONError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		msg     string
		code    int
		wantMsg string
	}{
		{
			name:    "basic error",
			msg:     "Something went wrong",
			code:    http.StatusBadRequest,
			wantMsg: "Something went wrong",
		},
		{
			name:    "special characters are escaped",
			msg:     `model "foo<bar>" not found`,
			code:    http.StatusBadRequest,
			wantMsg: `model "foo<bar>" not found`,
		},
		{
			name:    "injection attempt",
			msg:     `", "admin": true, "x": "`,
			code:    http.StatusBadRequest,
			wantMsg: `", "admin": true, "x": "`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rec := httptest.NewRecorder()
			writeJSONError(rec, tt.msg, tt.code)

			if rec.Code != tt.code {
				t.Errorf("expected status %d, got %d", tt.code, rec.Code)
			}

			ct := rec.Header().Get("Content-Type")
			if ct != "application/json" {
				t.Errorf("expected Content-Type application/json, got %q", ct)
			}

			var body map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("response is not valid JSON: %v\nbody: %s", err, rec.Body.String())
			}

			if body["error"] != tt.wantMsg {
				t.Errorf("expected error %q, got %q", tt.wantMsg, body["error"])
			}
		})
	}
}

func TestBuildPortkeyConfig_SingleProvider(t *testing.T) {
	t.Parallel()

	model := models.ModelConfig{
		Provider: "openai",
		APIKey:   "sk-test",
		OverrideParams: map[string]interface{}{
			"model":       "gpt-4",
			"temperature": 0.7,
		},
		Retry: &models.RetryConfig{Attempts: 3},
	}

	config := buildPortkeyConfig(model)

	if config.Provider != "openai" {
		t.Errorf("expected provider 'openai', got %q", config.Provider)
	}
	if config.APIKey != "sk-test" {
		t.Errorf("expected api_key 'sk-test', got %q", config.APIKey)
	}
	if config.OverrideParams["model"] != "gpt-4" {
		t.Errorf("expected model 'gpt-4', got %v", config.OverrideParams["model"])
	}
	if config.Retry == nil || config.Retry.Attempts != 3 {
		t.Error("expected retry config with 3 attempts")
	}
}

func TestBuildPortkeyConfig_MultiTarget(t *testing.T) {
	t.Parallel()

	model := models.ModelConfig{
		Strategy: &models.StrategyConfig{Mode: "fallback"},
		Targets: []models.TargetConfig{
			{Provider: "openai", APIKey: "sk-1"},
			{Provider: "anthropic", APIKey: "sk-2"},
		},
	}

	config := buildPortkeyConfig(model)

	if config.Strategy == nil {
		t.Fatal("expected strategy to be set")
	}
	if config.Strategy.Mode != "fallback" {
		t.Errorf("expected strategy mode 'fallback', got %q", config.Strategy.Mode)
	}
	if len(config.Targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(config.Targets))
	}
}

func TestBuildPortkeyConfig_Bedrock(t *testing.T) {
	t.Parallel()

	model := models.ModelConfig{
		Provider:           "bedrock",
		AWSAccessKeyID:     "AKIA",
		AWSSecretAccessKey: "secret",
		AWSRegion:          "us-east-1",
		AWSSessionToken:    "token",
	}

	config := buildPortkeyConfig(model)

	if config.AWSAccessKeyID != "AKIA" {
		t.Errorf("expected AWS key 'AKIA', got %q", config.AWSAccessKeyID)
	}
	if config.AWSSessionToken != "token" {
		t.Errorf("expected session token 'token', got %q", config.AWSSessionToken)
	}
}

func TestBuildPortkeyConfig_VertexAI(t *testing.T) {
	t.Parallel()

	model := models.ModelConfig{
		Provider:                 "vertex-ai",
		VertexProjectID:          "my-project",
		VertexRegion:             "us-central1",
		VertexServiceAccountJSON: `{"type": "service_account"}`,
	}

	config := buildPortkeyConfig(model)

	if config.VertexProjectID != "my-project" {
		t.Errorf("expected project 'my-project', got %q", config.VertexProjectID)
	}
	if config.VertexRegion != "us-central1" {
		t.Errorf("expected region 'us-central1', got %q", config.VertexRegion)
	}
	if config.VertexServiceAccountJSON != `{"type": "service_account"}` {
		t.Errorf("expected service account JSON, got %q", config.VertexServiceAccountJSON)
	}
}

func TestGetTimeout(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		model    models.ModelConfig
		expected int
	}{
		{
			name:     "default timeout",
			model:    models.ModelConfig{},
			expected: 60,
		},
		{
			name:     "custom timeout 30s",
			model:    models.ModelConfig{RequestTimeout: 30000},
			expected: 30,
		},
		{
			name:     "custom timeout 120s",
			model:    models.ModelConfig{RequestTimeout: 120000},
			expected: 120,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := getTimeout(tt.model)
			if result != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, result)
			}
		})
	}
}

func TestGetProviderFromConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		model    models.ModelConfig
		expected string
	}{
		{
			name:     "single provider",
			model:    models.ModelConfig{Provider: "openai"},
			expected: "openai",
		},
		{
			name: "from targets",
			model: models.ModelConfig{
				Targets: []models.TargetConfig{{Provider: "anthropic"}},
			},
			expected: "anthropic",
		},
		{
			name:     "unknown",
			model:    models.ModelConfig{},
			expected: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := getProviderFromConfig(tt.model)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestGetModelFromConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		model    models.ModelConfig
		expected string
	}{
		{
			name: "from override_params",
			model: models.ModelConfig{
				OverrideParams: map[string]interface{}{"model": "gpt-4"},
			},
			expected: "gpt-4",
		},
		{
			name: "from target override_params",
			model: models.ModelConfig{
				Targets: []models.TargetConfig{
					{OverrideParams: map[string]interface{}{"model": "claude-3"}},
				},
			},
			expected: "claude-3",
		},
		{
			name:     "unknown",
			model:    models.ModelConfig{},
			expected: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := getModelFromConfig(tt.model)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestCopyHeaders_SkipsHopByHop(t *testing.T) {
	t.Parallel()

	src := http.Header{}
	src.Set("Content-Type", "application/json")
	src.Set("Authorization", "Bearer secret")
	src.Set("X-Api-Key", "secret-key")
	src.Set("Connection", "keep-alive")
	src.Set("X-Custom-Header", "value")

	dst := http.Header{}
	copyHeaders(src, dst)

	if dst.Get("Content-Type") != "application/json" {
		t.Error("expected Content-Type to be copied")
	}
	if dst.Get("X-Custom-Header") != "value" {
		t.Error("expected X-Custom-Header to be copied")
	}
	if dst.Get("Authorization") != "" {
		t.Error("expected Authorization to be skipped")
	}
	if dst.Get("X-Api-Key") != "" {
		t.Error("expected X-Api-Key to be skipped")
	}
	if dst.Get("Connection") != "" {
		t.Error("expected Connection to be skipped")
	}
}

func TestHealthHandler(t *testing.T) {
	t.Parallel()

	store := &models.ConfigStore{
		Models:    map[string]models.ModelConfig{"test-model": {}},
		StartTime: time.Now(),
	}

	handler := HealthHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp models.HealthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.Status != "healthy" {
		t.Errorf("expected status 'healthy', got %q", resp.Status)
	}
	if resp.Version != models.Version {
		t.Errorf("expected version %q, got %q", models.Version, resp.Version)
	}
}

func TestHealthHandler_MethodNotAllowed(t *testing.T) {
	t.Parallel()

	store := &models.ConfigStore{StartTime: time.Now()}
	handler := HealthHandler(store)

	req := httptest.NewRequest(http.MethodPost, "/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", rec.Code)
	}
}

func TestModelsHandler(t *testing.T) {
	t.Parallel()

	store := &models.ConfigStore{
		Models: map[string]models.ModelConfig{
			"gpt4":   {Provider: "openai"},
			"claude": {Provider: "anthropic"},
		},
		StartTime: time.Now(),
	}

	handler := ModelsHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp models.ModelsListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.Object != "list" {
		t.Errorf("expected object 'list', got %q", resp.Object)
	}
	if len(resp.Data) != 2 {
		t.Errorf("expected 2 models, got %d", len(resp.Data))
	}
}

func TestHealthHandler_NoModelAliasesLeaked(t *testing.T) {
	t.Parallel()

	store := &models.ConfigStore{
		Models: map[string]models.ModelConfig{
			"secret-model": {Provider: "openai", APIKey: "sk-secret"},
		},
		StartTime: time.Now(),
	}

	handler := HealthHandler(store)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var raw map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if _, ok := raw["model_aliases"]; ok {
		t.Error("health response should not contain model_aliases")
	}
	if _, ok := raw["proxy_key_count"]; ok {
		t.Error("health response should not contain proxy_key_count")
	}
}

func TestJoinBetaHeaders(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		headers  []string
		expected string
	}{
		{
			name:     "single header",
			headers:  []string{"max-tokens-3-5-sonnet-2024-07-15"},
			expected: "max-tokens-3-5-sonnet-2024-07-15",
		},
		{
			name:     "multiple headers",
			headers:  []string{"header1", "header2", "header3"},
			expected: "header1,header2,header3",
		},
		{
			name:     "empty",
			headers:  []string{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := joinBetaHeaders(tt.headers)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}
