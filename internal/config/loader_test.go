package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/amscotti/portus/internal/models"
)

func TestExpandEnvVars(t *testing.T) {
	t.Setenv("TEST_VAR_A", "hello")
	t.Setenv("TEST_VAR_B", "world")

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "single var",
			input:    `{"key": "${TEST_VAR_A}"}`,
			expected: `{"key": "hello"}`,
		},
		{
			name:     "multiple vars",
			input:    `{"a": "${TEST_VAR_A}", "b": "${TEST_VAR_B}"}`,
			expected: `{"a": "hello", "b": "world"}`,
		},
		{
			name:     "missing var returns empty",
			input:    `{"key": "${NONEXISTENT_VAR_XYZ}"}`,
			expected: `{"key": ""}`,
		},
		{
			name:     "no vars",
			input:    `{"key": "value"}`,
			expected: `{"key": "value"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := expandEnvVars(tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestValidateModelConfig_SingleProvider(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		alias   string
		model   models.ModelConfig
		wantErr bool
	}{
		{
			name:  "valid openai",
			alias: "gpt4",
			model: models.ModelConfig{
				Provider: "openai",
				APIKey:   "sk-test",
			},
			wantErr: false,
		},
		{
			name:  "missing api_key",
			alias: "gpt4",
			model: models.ModelConfig{
				Provider: "openai",
			},
			wantErr: true,
		},
		{
			name:  "missing provider",
			alias: "test",
			model: models.ModelConfig{},
			// no provider AND no strategy → error
			wantErr: true,
		},
		{
			name:  "unknown provider",
			alias: "test",
			model: models.ModelConfig{
				Provider: "fakeprovider",
			},
			wantErr: true,
		},
		{
			name:  "valid bedrock",
			alias: "bedrock-model",
			model: models.ModelConfig{
				Provider:           "bedrock",
				AWSAccessKeyID:     "AKIA...",
				AWSSecretAccessKey: "secret",
				AWSRegion:          "us-east-1",
			},
			wantErr: false,
		},
		{
			name:  "bedrock missing region",
			alias: "bedrock-model",
			model: models.ModelConfig{
				Provider:           "bedrock",
				AWSAccessKeyID:     "AKIA...",
				AWSSecretAccessKey: "secret",
			},
			wantErr: true,
		},
		{
			name:  "valid vertex-ai",
			alias: "vertex-model",
			model: models.ModelConfig{
				Provider:                 "vertex-ai",
				VertexProjectID:          "my-project",
				VertexRegion:             "us-central1",
				VertexServiceAccountJSON: "{}",
			},
			wantErr: false,
		},
		{
			name:  "vertex-ai missing service account",
			alias: "vertex-model",
			model: models.ModelConfig{
				Provider:        "vertex-ai",
				VertexProjectID: "my-project",
				VertexRegion:    "us-central1",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateModelConfig(tt.alias, tt.model)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateModelConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateModelConfig_MultiTarget(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		alias   string
		model   models.ModelConfig
		wantErr bool
	}{
		{
			name:  "valid fallback",
			alias: "multi",
			model: models.ModelConfig{
				Strategy: &models.StrategyConfig{Mode: "fallback"},
				Targets: []models.TargetConfig{
					{Provider: "openai", APIKey: "sk-1"},
					{Provider: "anthropic", APIKey: "sk-2"},
				},
			},
			wantErr: false,
		},
		{
			name:  "strategy with no targets",
			alias: "multi",
			model: models.ModelConfig{
				Strategy: &models.StrategyConfig{Mode: "fallback"},
				Targets:  []models.TargetConfig{},
			},
			wantErr: true,
		},
		{
			name:  "invalid strategy mode",
			alias: "multi",
			model: models.ModelConfig{
				Strategy: &models.StrategyConfig{Mode: "roundrobin"},
				Targets: []models.TargetConfig{
					{Provider: "openai", APIKey: "sk-1"},
				},
			},
			wantErr: true,
		},
		{
			name:  "target missing provider",
			alias: "multi",
			model: models.ModelConfig{
				Strategy: &models.StrategyConfig{Mode: "fallback"},
				Targets: []models.TargetConfig{
					{APIKey: "sk-1"},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateModelConfig(tt.alias, tt.model)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateModelConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoadProxyKeys(t *testing.T) {
	t.Setenv("PORTUS_KEY_BACKEND", "pk-backend-123")
	t.Setenv("PORTUS_KEY_FRONTEND", "pk-frontend-456")

	store := &models.ConfigStore{
		ProxyKeys: []models.ProxyKey{},
	}

	loadProxyKeys(store)

	if len(store.ProxyKeys) < 2 {
		t.Fatalf("expected at least 2 proxy keys, got %d", len(store.ProxyKeys))
	}

	found := make(map[string]string)
	for _, pk := range store.ProxyKeys {
		found[pk.Application] = pk.Key
	}

	if found["BACKEND"] != "pk-backend-123" {
		t.Errorf("expected BACKEND key 'pk-backend-123', got %q", found["BACKEND"])
	}
	if found["FRONTEND"] != "pk-frontend-456" {
		t.Errorf("expected FRONTEND key 'pk-frontend-456', got %q", found["FRONTEND"])
	}
}

func TestCheckMissingEnvVars(t *testing.T) {
	t.Setenv("EXISTING_VAR", "value")

	rawContent := `{"api_key": "${EXISTING_VAR}", "secret": "${MISSING_VAR_XYZ}"}`

	missingVars := make(map[string][]string)
	checkMissingEnvVars("test-model", rawContent, missingVars)

	if _, ok := missingVars["EXISTING_VAR"]; ok {
		t.Error("EXISTING_VAR should not be in missingVars")
	}
	if files, ok := missingVars["MISSING_VAR_XYZ"]; !ok {
		t.Error("MISSING_VAR_XYZ should be in missingVars")
	} else if len(files) != 1 || files[0] != "test-model.json" {
		t.Errorf("unexpected files for MISSING_VAR_XYZ: %v", files)
	}
}

func TestLoadModelConfigs(t *testing.T) {
	// Create a temp directory structure
	dir := t.TempDir()
	modelsDir := filepath.Join(dir, "models")
	if err := os.MkdirAll(modelsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	configJSON := `{
		"provider": "openai",
		"api_key": "sk-test",
		"override_params": {"model": "gpt-4"}
	}`
	if err := os.WriteFile(filepath.Join(modelsDir, "gpt4.json"), []byte(configJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	store := &models.ConfigStore{
		Models:     make(map[string]models.ModelConfig),
		RawConfigs: make(map[string]string),
		ConfigPath: dir,
	}

	if err := loadModelConfigs(store); err != nil {
		t.Fatalf("loadModelConfigs() error: %v", err)
	}

	if len(store.Models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(store.Models))
	}

	model, ok := store.Models["gpt4"]
	if !ok {
		t.Fatal("expected model alias 'gpt4'")
	}
	if model.Provider != "openai" {
		t.Errorf("expected provider 'openai', got %q", model.Provider)
	}

	// Check raw config was stored
	if _, ok := store.RawConfigs["gpt4"]; !ok {
		t.Error("expected raw config to be stored for 'gpt4'")
	}
}
