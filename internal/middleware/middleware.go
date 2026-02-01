// Package middleware provides HTTP middleware for Portus.
package middleware

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"strings"
	"time"

	"github.com/amscotti/portus/internal/models"
)

// contextKey is a custom type for context keys to avoid collisions.
type contextKey int

const (
	// ContextKeyApplication stores the application name in the request context.
	ContextKeyApplication contextKey = iota
	// ContextKeyRequestID stores the request ID in the request context.
	ContextKeyRequestID
)

// AuthMiddleware validates proxy keys and adds application info to context.
func AuthMiddleware(proxyKeys []models.ProxyKey, logger *slog.Logger) func(http.Handler) http.Handler {
	// Build a map for quick lookup
	keyMap := make(map[string]string) // key -> application name
	for _, pk := range proxyKeys {
		keyMap[pk.Key] = pk.Application
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var token string
			var authSource string

			// Check for Authorization header (OpenAI SDK style)
			authHeader := r.Header.Get("Authorization")
			if authHeader != "" {
				authSource = "Authorization"
				// Extract the token (remove "Bearer " prefix if present)
				token = authHeader
				if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
					token = authHeader[7:]
				}
			} else {
				// Check for x-api-key header (Anthropic SDK style)
				apiKey := r.Header.Get("x-api-key")
				if apiKey != "" {
					authSource = "x-api-key"
					token = apiKey
				}
			}

			if token == "" {
				logger.Warn("missing authorization header",
					"path", r.URL.Path,
					"remote_addr", r.RemoteAddr,
				)
				http.Error(w, `{"error": "Missing Authorization header"}`, http.StatusUnauthorized)
				return
			}

			// Validate the key
			application, valid := keyMap[token]
			if !valid {
				logger.Warn("invalid authorization key",
					"path", r.URL.Path,
					"remote_addr", r.RemoteAddr,
					"source", authSource,
				)
				http.Error(w, `{"error": "Invalid Authorization key"}`, http.StatusUnauthorized)
				return
			}

			// Add application to context
			ctx := context.WithValue(r.Context(), ContextKeyApplication, application)
			r = r.WithContext(ctx)

			// Set application on responseWriter if available
			if rw, ok := w.(*responseWriter); ok {
				rw.application = application
			}

			next.ServeHTTP(w, r)
		})
	}
}

// LoggingMiddleware logs all HTTP requests with structured logging.
func LoggingMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Create a response writer wrapper to capture status code
			wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			next.ServeHTTP(wrapped, r)

			duration := time.Since(start)

			// Log the request
			logger.Info("request completed",
				"method", r.Method,
				"path", r.URL.Path,
				"application", wrapped.application,
				"status", wrapped.statusCode,
				"duration_ms", duration.Milliseconds(),
				"remote_addr", r.RemoteAddr,
			)
		})
	}
}

// RequestIDMiddleware generates and adds a request ID to the context.
func RequestIDMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := generateRequestID()

			ctx := context.WithValue(r.Context(), ContextKeyRequestID, requestID)
			r = r.WithContext(ctx)

			// Add request ID to response headers
			w.Header().Set("X-Request-ID", requestID)

			next.ServeHTTP(w, r)
		})
	}
}

// responseWriter wraps http.ResponseWriter to capture the status code.
type responseWriter struct {
	http.ResponseWriter
	statusCode  int
	application string
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Flush implements http.Flusher by delegating to the underlying writer.
func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap returns the underlying ResponseWriter, allowing the http package
// to find additional interface implementations.
func (rw *responseWriter) Unwrap() http.ResponseWriter {
	return rw.ResponseWriter
}

// generateRequestID generates a unique request ID.
func generateRequestID() string {
	return time.Now().Format("20060102150405") + "-" + generateRandomString(8)
}

func generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	result := make([]byte, length)
	for i := range result {
		result[i] = charset[rand.IntN(len(charset))]
	}
	return string(result)
}

// RecoverMiddleware recovers from panics and logs them.
func RecoverMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					logger.Error("panic recovered",
						"error", err,
						"path", r.URL.Path,
						"method", r.Method,
					)
					http.Error(w, `{"error": "Internal server error"}`, http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
