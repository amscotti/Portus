// Portus - Configuration Proxy for Portkey Gateway
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/amscotti/portus/internal/config"
	"github.com/amscotti/portus/internal/handlers"
	"github.com/amscotti/portus/internal/middleware"
	"github.com/amscotti/portus/internal/models"
)

func main() {
	// Setup structured logging
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: getLogLevel(),
	}))

	logger.Info("starting Portus", "version", models.Version)

	// Load configuration
	logger.Info("loading configuration...")
	store, err := config.LoadConfig()
	if err != nil {
		logger.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	// Validate configuration
	logger.Info("validating configuration...")
	validationErrors := config.ValidateConfig(store)
	if len(validationErrors) > 0 {
		fmt.Fprintf(os.Stderr, "\nERROR: Configuration validation failed\n\n")

		for _, err := range validationErrors {
			fmt.Fprintf(os.Stderr, "  - %s\n", err)
			logger.Error("validation error", "error", err)
		}

		fmt.Fprintf(os.Stderr, "\nPlease set all required environment variables and restart.\n")
		os.Exit(1)
	}

	logger.Info("configuration loaded successfully",
		"models", len(store.Models),
		"proxy_keys", len(store.ProxyKeys),
		"port", store.ServerPort,
	)

	// Setup HTTP router
	mux := http.NewServeMux()

	// Health endpoint (no auth required)
	mux.HandleFunc("/health", handlers.HealthHandler(store))

	// Protected endpoints
	authMiddleware := middleware.AuthMiddleware(store.ProxyKeys, logger)
	requestIDMiddleware := middleware.RequestIDMiddleware()

	// Models endpoint
	mux.Handle("/v1/models", chain(
		handlers.ModelsHandler(store),
		authMiddleware,
		requestIDMiddleware,
	))

	// Chat completions endpoint
	mux.Handle("/v1/chat/completions", chain(
		handlers.ChatCompletionsHandler(store, logger),
		authMiddleware,
		requestIDMiddleware,
	))

	// Anthropic messages endpoint
	mux.Handle("/v1/messages", chain(
		handlers.MessagesHandler(store, logger),
		authMiddleware,
		requestIDMiddleware,
	))

	// Apply global middleware
	handler := middleware.RecoverMiddleware(logger)(
		middleware.LoggingMiddleware(logger)(mux),
	)

	// Create HTTP server
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", store.ServerPort),
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start server in a goroutine
	go func() {
		logger.Info("server listening", "addr", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down server...")

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Error("server shutdown error", "error", err)
		os.Exit(1)
	}

	logger.Info("server stopped")
}

// chain applies middleware to a handler in reverse order.
func chain(h http.Handler, middleware ...func(http.Handler) http.Handler) http.Handler {
	for i := len(middleware) - 1; i >= 0; i-- {
		h = middleware[i](h)
	}
	return h
}

// getLogLevel returns the configured log level.
func getLogLevel() slog.Level {
	level := os.Getenv("PORTUS_LOG_LEVEL")
	switch level {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
