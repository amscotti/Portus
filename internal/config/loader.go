// Package config handles loading and validation of Portus configuration.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/amscotti/portus/internal/models"
)

const (
	defaultPort       = 8080
	defaultConfigPath = "./config"
	defaultGatewayURL = "http://localhost:8787"
	defaultLogLevel   = "info"
)

var (
	envVarRegex = regexp.MustCompile(`\$\{([^}]+)\}`)
)

// LoadConfig loads all configuration from files and environment variables.
func LoadConfig() (*models.ConfigStore, error) {
	store := &models.ConfigStore{
		Models:     make(map[string]models.ModelConfig),
		RawConfigs: make(map[string]string),
		ProxyKeys:  []models.ProxyKey{},
		StartTime:  time.Now(),
	}

	// Load server configuration from environment
	if err := loadServerConfig(store); err != nil {
		return nil, fmt.Errorf("failed to load server config: %w", err)
	}

	// Load proxy keys from environment
	loadProxyKeys(store)

	// Load model configurations from files
	if err := loadModelConfigs(store); err != nil {
		return nil, fmt.Errorf("failed to load model configs: %w", err)
	}

	return store, nil
}

// ValidateConfig performs comprehensive validation and returns all errors at once.
func ValidateConfig(store *models.ConfigStore) []error {
	var errors []error

	// Validate proxy keys
	if len(store.ProxyKeys) == 0 {
		errors = append(errors, fmt.Errorf("no proxy keys configured: at least one PORTUS_KEY_* environment variable is required"))
	}

	// Validate model configurations
	if len(store.Models) == 0 {
		errors = append(errors, fmt.Errorf("no model configurations found in %s", store.ConfigPath))
	}

	// Check for missing environment variables using stored raw configs
	missingVars := make(map[string][]string) // var name -> list of files referencing it

	for alias, rawContent := range store.RawConfigs {
		checkMissingEnvVars(alias, rawContent, missingVars)
	}

	if len(missingVars) > 0 {
		for varName, files := range missingVars {
			errors = append(errors, fmt.Errorf("missing environment variable: %s (referenced in: %s)",
				varName, strings.Join(files, ", ")))
		}
	}

	// Validate each model configuration
	for alias, model := range store.Models {
		if err := validateModelConfig(alias, model); err != nil {
			errors = append(errors, err)
		}
	}

	// Clear raw configs after validation — no longer needed
	store.RawConfigs = nil

	return errors
}

func loadServerConfig(store *models.ConfigStore) error {
	// Port
	portStr := os.Getenv("PORTUS_PORT")
	if portStr == "" {
		store.ServerPort = defaultPort
	} else {
		port, err := strconv.Atoi(portStr)
		if err != nil {
			return fmt.Errorf("invalid PORTUS_PORT value: %s", portStr)
		}
		store.ServerPort = port
	}

	// Config path
	store.ConfigPath = os.Getenv("PORTUS_CONFIG_PATH")
	if store.ConfigPath == "" {
		store.ConfigPath = defaultConfigPath
	}

	// Gateway URL
	store.GatewayURL = os.Getenv("PORTKEY_GATEWAY_URL")
	if store.GatewayURL == "" {
		store.GatewayURL = defaultGatewayURL
	}

	// Log level
	store.LogLevel = os.Getenv("PORTUS_LOG_LEVEL")
	if store.LogLevel == "" {
		store.LogLevel = defaultLogLevel
	}

	return nil
}

func loadProxyKeys(store *models.ConfigStore) {
	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := parts[0]
		value := parts[1]

		if strings.HasPrefix(key, "PORTUS_KEY_") {
			appName := strings.TrimPrefix(key, "PORTUS_KEY_")
			store.ProxyKeys = append(store.ProxyKeys, models.ProxyKey{
				Key:         value,
				Application: appName,
			})
		}
	}
}

func loadModelConfigs(store *models.ConfigStore) error {
	modelsDir := filepath.Join(store.ConfigPath, "models")

	entries, err := os.ReadDir(modelsDir)
	if err != nil {
		if os.IsNotExist(err) {
			// Models directory doesn't exist, which is ok - we'll just have no models
			return nil
		}
		return fmt.Errorf("failed to read models directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		alias := strings.TrimSuffix(entry.Name(), ".json")
		path := filepath.Join(modelsDir, entry.Name())

		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read model config %s: %w", path, err)
		}

		// Store raw content before expansion for env var checking during validation
		store.RawConfigs[alias] = string(data)

		// Expand environment variables
		expandedData := expandEnvVars(string(data))

		var config models.ModelConfig
		if err := json.Unmarshal([]byte(expandedData), &config); err != nil {
			return fmt.Errorf("failed to parse model config %s: %w", path, err)
		}

		store.Models[alias] = config
	}

	return nil
}

func expandEnvVars(content string) string {
	return envVarRegex.ReplaceAllStringFunc(content, func(match string) string {
		// Extract variable name from ${VAR_NAME}
		varName := match[2 : len(match)-1]
		value := os.Getenv(varName)
		return value
	})
}

func checkMissingEnvVars(alias string, rawContent string, missingVars map[string][]string) {
	matches := envVarRegex.FindAllStringSubmatch(rawContent, -1)

	for _, match := range matches {
		if len(match) >= 2 {
			varName := match[1]
			if os.Getenv(varName) == "" {
				missingVars[varName] = append(missingVars[varName], alias+".json")
			}
		}
	}
}

func validateModelConfig(alias string, model models.ModelConfig) error {
	// Check if using strategy/targets or single provider
	if model.Strategy != nil {
		// Multi-target configuration
		if len(model.Targets) == 0 {
			return fmt.Errorf("model %s has strategy but no targets", alias)
		}
		if model.Strategy.Mode != "fallback" && model.Strategy.Mode != "loadbalance" {
			return fmt.Errorf("model %s has invalid strategy mode: %s (must be 'fallback' or 'loadbalance')", alias, model.Strategy.Mode)
		}

		// Validate each target
		for i, target := range model.Targets {
			if target.Provider == "" {
				return fmt.Errorf("model %s target %d has no provider", alias, i)
			}
			if err := validateProviderConfig(alias, target.Provider, i, target); err != nil {
				return err
			}
		}
	} else {
		// Single provider configuration
		if model.Provider == "" {
			return fmt.Errorf("model %s has no provider (and no strategy/targets)", alias)
		}
		if err := validateSingleProviderConfig(alias, model); err != nil {
			return err
		}
	}

	return nil
}

func validateProviderConfig(alias string, provider string, targetIndex int, target models.TargetConfig) error {
	switch provider {
	case "anthropic", "openai", "google":
		// These providers need an API key
		if target.APIKey == "" {
			return fmt.Errorf("model %s target %d (provider %s) missing api_key", alias, targetIndex, provider)
		}
	case "bedrock":
		// Bedrock needs AWS credentials
		if target.AWSAccessKeyID == "" || target.AWSSecretAccessKey == "" || target.AWSRegion == "" {
			return fmt.Errorf("model %s target %d (provider bedrock) missing AWS credentials", alias, targetIndex)
		}
	case "vertex-ai":
		// Vertex AI needs project, region, and service account
		// Note: These are checked in the parent model config, not target
	}

	return nil
}

func validateSingleProviderConfig(alias string, model models.ModelConfig) error {
	switch model.Provider {
	case "anthropic", "openai", "google":
		if model.APIKey == "" {
			return fmt.Errorf("model %s (provider %s) missing api_key", alias, model.Provider)
		}
	case "bedrock":
		if model.AWSAccessKeyID == "" || model.AWSSecretAccessKey == "" || model.AWSRegion == "" {
			return fmt.Errorf("model %s (provider bedrock) missing AWS credentials", alias)
		}
	case "vertex-ai":
		if model.VertexProjectID == "" || model.VertexRegion == "" || model.VertexServiceAccountJSON == "" {
			return fmt.Errorf("model %s (provider vertex-ai) missing Vertex AI configuration", alias)
		}
	default:
		return fmt.Errorf("model %s has unknown provider: %s", alias, model.Provider)
	}

	return nil
}
