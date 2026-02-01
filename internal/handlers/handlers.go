// Package handlers implements HTTP request handlers for Portus endpoints.
package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/amscotti/portus/internal/middleware"
	"github.com/amscotti/portus/internal/models"
)

const maxBodySize = 10 * 1024 * 1024 // 10 MB

// hopByHopHeaders are headers that should not be forwarded by proxies.
var hopByHopHeaders = map[string]struct{}{
	"Connection":          {},
	"Keep-Alive":          {},
	"Proxy-Authenticate":  {},
	"Proxy-Authorization": {},
	"Te":                  {},
	"Trailers":            {},
	"Transfer-Encoding":   {},
	"Upgrade":             {},
	"Authorization":       {},
	"X-Api-Key":           {},
}

// gatewayTransport is a shared transport for connection pooling to the gateway.
var gatewayTransport = &http.Transport{
	MaxIdleConns:        100,
	MaxIdleConnsPerHost: 100,
	IdleConnTimeout:     90 * time.Second,
	DialContext: (&net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}).DialContext,
}

// gatewayClient is a shared HTTP client for proxying requests to the gateway.
// Per-request timeouts are applied via context instead of on the client.
var gatewayClient = &http.Client{
	Transport: gatewayTransport,
}

// writeJSONError writes a JSON-formatted error response with proper escaping.
func writeJSONError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// HealthHandler returns the health check endpoint handler.
func HealthHandler(store *models.ConfigStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		uptime := time.Since(store.StartTime)

		response := models.HealthResponse{
			Status:  "healthy",
			Version: models.Version,
			Uptime:  uptime.String(),
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}
}

// ModelsHandler returns the models list endpoint handler.
func ModelsHandler(store *models.ConfigStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Build model list using server start time as "created" timestamp
		created := store.StartTime.Unix()

		data := make([]models.ModelObject, 0, len(store.Models))
		for alias := range store.Models {
			data = append(data, models.ModelObject{
				ID:      alias,
				Object:  "model",
				Created: created,
				OwnedBy: "portus",
			})
		}

		response := models.ModelsListResponse{
			Object: "list",
			Data:   data,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}
}

// ChatCompletionsHandler returns the chat completions endpoint handler.
func ChatCompletionsHandler(store *models.ConfigStore, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Parse request body with size limit
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodySize))
		if err != nil {
			var maxBytesErr *http.MaxBytesError
			if errors.As(err, &maxBytesErr) {
				writeJSONError(w, "Request body too large", http.StatusRequestEntityTooLarge)
				return
			}
			logger.Error("failed to read request body", "error", err)
			writeJSONError(w, "Failed to read request body", http.StatusBadRequest)
			return
		}

		var req models.ChatCompletionRequest
		if err := json.Unmarshal(body, &req); err != nil {
			logger.Error("failed to parse request body", "error", err)
			writeJSONError(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		// Validate model alias
		if req.Model == "" {
			writeJSONError(w, "Missing 'model' field in request", http.StatusBadRequest)
			return
		}

		modelConfig, exists := store.Models[req.Model]
		if !exists {
			logger.Warn("unknown model alias", "alias", req.Model)
			writeJSONError(w, "Unknown model alias", http.StatusBadRequest)
			return
		}

		// Get context values
		application, _ := r.Context().Value(middleware.ContextKeyApplication).(string)
		requestID, _ := r.Context().Value(middleware.ContextKeyRequestID).(string)

		// Delegate to shared proxy handler
		handleProxyRequest(w, r, body, "/v1/chat/completions", modelConfig, store, logger, requestID, application, req.Model)
	}
}

// MessagesHandler returns the Anthropic messages endpoint handler.
func MessagesHandler(store *models.ConfigStore, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Parse request body with size limit
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodySize))
		if err != nil {
			var maxBytesErr *http.MaxBytesError
			if errors.As(err, &maxBytesErr) {
				writeJSONError(w, "Request body too large", http.StatusRequestEntityTooLarge)
				return
			}
			logger.Error("failed to read request body", "error", err)
			writeJSONError(w, "Failed to read request body", http.StatusBadRequest)
			return
		}

		var req models.MessagesRequest
		if err := json.Unmarshal(body, &req); err != nil {
			logger.Error("failed to parse request body", "error", err)
			writeJSONError(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		// Validate model alias
		if req.Model == "" {
			writeJSONError(w, "Missing 'model' field in request", http.StatusBadRequest)
			return
		}

		modelConfig, exists := store.Models[req.Model]
		if !exists {
			logger.Warn("unknown model alias", "alias", req.Model)
			writeJSONError(w, "Unknown model alias", http.StatusBadRequest)
			return
		}

		// Ensure max_tokens is set
		if req.MaxTokens == 0 {
			// Try to get from model config override params
			if modelConfig.OverrideParams != nil {
				if mt, ok := modelConfig.OverrideParams["max_tokens"].(float64); ok {
					req.MaxTokens = int(mt)
				}
			}
			// Default if still not set
			if req.MaxTokens == 0 {
				req.MaxTokens = 4096
			}
			// Update the body with the injected max_tokens
			bodyMap := make(map[string]interface{})
			if err := json.Unmarshal(body, &bodyMap); err == nil {
				bodyMap["max_tokens"] = req.MaxTokens
				if updatedBody, err := json.Marshal(bodyMap); err == nil {
					body = updatedBody
				}
			}
		}

		// Inject thinking configuration if present in model config
		if modelConfig.Thinking != nil && req.Thinking == nil {
			bodyMap := make(map[string]interface{})
			if err := json.Unmarshal(body, &bodyMap); err == nil {
				bodyMap["thinking"] = modelConfig.Thinking
				if updatedBody, err := json.Marshal(bodyMap); err == nil {
					body = updatedBody
				}
			}
		}

		// Get context values
		application, _ := r.Context().Value(middleware.ContextKeyApplication).(string)
		requestID, _ := r.Context().Value(middleware.ContextKeyRequestID).(string)

		// Delegate to shared proxy handler
		handleProxyRequest(w, r, body, "/v1/messages", modelConfig, store, logger, requestID, application, req.Model)
	}
}

// handleProxyRequest executes the shared proxy logic for both chat completions and messages endpoints.
func handleProxyRequest(w http.ResponseWriter, r *http.Request, body []byte, targetPath string, modelConfig models.ModelConfig, store *models.ConfigStore, logger *slog.Logger, requestID, application, modelAlias string) {
	// Build Portkey configuration
	portkeyConfig := buildPortkeyConfig(modelConfig)

	// Create proxy request to Portkey Gateway with per-request timeout
	timeout := time.Duration(getTimeout(modelConfig)) * time.Second
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()

	proxyReq, err := http.NewRequestWithContext(ctx, r.Method, store.GatewayURL+targetPath, bytes.NewReader(body))
	if err != nil {
		logger.Error("failed to create proxy request", "error", err)
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Copy headers from original request, skipping hop-by-hop headers
	copyHeaders(r.Header, proxyReq.Header)

	// Set Portkey-specific headers
	if err := setPortkeyHeaders(proxyReq, portkeyConfig, modelConfig); err != nil {
		logger.Error("failed to set Portkey headers", "error", err)
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Execute proxy request
	start := time.Now()
	resp, err := gatewayClient.Do(proxyReq)
	if err != nil {
		logger.Error("failed to proxy request to gateway", "error", err)
		writeJSONError(w, "Failed to reach gateway", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	duration := time.Since(start)

	// Log the request
	provider := getProviderFromConfig(modelConfig)
	resolvedModel := getModelFromConfig(modelConfig)
	logger.Info("proxy request completed",
		"request_id", requestID,
		"application", application,
		"endpoint", targetPath,
		"model_alias", modelAlias,
		"provider", provider,
		"resolved_model", resolvedModel,
		"status", resp.StatusCode,
		"duration_ms", duration.Milliseconds(),
	)

	// Copy response headers
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	w.WriteHeader(resp.StatusCode)

	// Stream or copy response body
	if flusher, ok := w.(http.Flusher); ok {
		buf := make([]byte, 4096)
		for {
			n, err := resp.Body.Read(buf)
			if n > 0 {
				if _, wErr := w.Write(buf[:n]); wErr != nil {
					logger.Warn("client disconnected during stream", "error", wErr)
					break
				}
				flusher.Flush()
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				// Check for context cancellation error
				if errors.Is(err, context.Canceled) {
					logger.Warn("request canceled by client")
				} else {
					logger.Error("error reading stream", "error", err)
				}
				break
			}
		}
	} else {
		io.Copy(w, resp.Body)
	}
}

// buildPortkeyConfig constructs the Portkey configuration from model config.
func buildPortkeyConfig(model models.ModelConfig) *models.PortkeyConfig {
	config := &models.PortkeyConfig{
		Retry:          model.Retry,
		RequestTimeout: model.RequestTimeout,
	}

	if model.Strategy != nil {
		// Multi-target configuration
		config.Strategy = model.Strategy
		config.Targets = make([]models.TargetConfig, len(model.Targets))
		copy(config.Targets, model.Targets)
	} else {
		// Single provider configuration
		config.Provider = model.Provider
		config.APIKey = model.APIKey
		config.OverrideParams = make(map[string]interface{})

		// Copy override params
		for k, v := range model.OverrideParams {
			config.OverrideParams[k] = v
		}

		// Add AWS credentials for Bedrock
		if model.Provider == "bedrock" {
			config.AWSAccessKeyID = model.AWSAccessKeyID
			config.AWSSecretAccessKey = model.AWSSecretAccessKey
			config.AWSRegion = model.AWSRegion
			config.AWSSessionToken = model.AWSSessionToken
		}

		// Add Vertex AI config
		if model.Provider == "vertex-ai" {
			config.VertexProjectID = model.VertexProjectID
			config.VertexRegion = model.VertexRegion
			config.VertexServiceAccountJSON = model.VertexServiceAccountJSON
		}
	}

	return config
}

// setPortkeyHeaders sets the appropriate Portkey headers on the request.
func setPortkeyHeaders(req *http.Request, config *models.PortkeyConfig, model models.ModelConfig) error {
	// Set the x-portkey-config header
	configJSON, err := config.ToJSON()
	if err != nil {
		return fmt.Errorf("failed to marshal portkey config: %w", err)
	}
	req.Header.Set("x-portkey-config", configJSON)

	// Set provider-specific headers
	provider := getProviderFromConfig(model)
	req.Header.Set("x-portkey-provider", provider)

	// Set Vertex-specific headers
	if provider == "vertex-ai" {
		req.Header.Set("x-portkey-vertex-project-id", model.VertexProjectID)
		req.Header.Set("x-portkey-vertex-region", model.VertexRegion)
	}

	// Set Anthropic beta headers if configured
	if provider == "anthropic" && len(model.BetaHeaders) > 0 {
		req.Header.Set("x-portkey-anthropic-beta", joinBetaHeaders(model.BetaHeaders))
	}

	return nil
}

// getProviderFromConfig extracts the provider from model config.
func getProviderFromConfig(model models.ModelConfig) string {
	if model.Provider != "" {
		return model.Provider
	}
	if len(model.Targets) > 0 {
		return model.Targets[0].Provider
	}
	return "unknown"
}

// getModelFromConfig extracts the model name from model config.
func getModelFromConfig(model models.ModelConfig) string {
	if model.OverrideParams != nil {
		if m, ok := model.OverrideParams["model"].(string); ok {
			return m
		}
	}
	if len(model.Targets) > 0 && model.Targets[0].OverrideParams != nil {
		if m, ok := model.Targets[0].OverrideParams["model"].(string); ok {
			return m
		}
	}
	return "unknown"
}

// getTimeout returns the timeout in seconds from model config.
func getTimeout(model models.ModelConfig) int {
	if model.RequestTimeout > 0 {
		return model.RequestTimeout / 1000 // Convert ms to seconds
	}
	return 60 // Default 60 seconds
}

// joinBetaHeaders joins beta headers with commas.
func joinBetaHeaders(headers []string) string {
	result := ""
	for i, h := range headers {
		if i > 0 {
			result += ","
		}
		result += h
	}
	return result
}

// copyHeaders copies headers from src to dst, skipping hop-by-hop and proxy credential headers.
func copyHeaders(src, dst http.Header) {
	for key, values := range src {
		if _, skip := hopByHopHeaders[http.CanonicalHeaderKey(key)]; skip {
			continue
		}
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}