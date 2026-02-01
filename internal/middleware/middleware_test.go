package middleware

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/amscotti/portus/internal/models"
)

func TestGenerateRandomString_Uniqueness(t *testing.T) {
	t.Parallel()
	seen := make(map[string]struct{})
	for i := 0; i < 100; i++ {
		s := generateRandomString(8)
		if len(s) != 8 {
			t.Fatalf("expected length 8, got %d", len(s))
		}
		seen[s] = struct{}{}
	}
	// With 36^8 possible values, 100 calls should produce at least 90 unique strings.
	if len(seen) < 90 {
		t.Errorf("expected at least 90 unique strings out of 100, got %d", len(seen))
	}
}

func TestGenerateRandomString_Charset(t *testing.T) {
	t.Parallel()
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	allowed := make(map[byte]struct{})
	for i := 0; i < len(charset); i++ {
		allowed[charset[i]] = struct{}{}
	}
	s := generateRandomString(1000)
	for i := 0; i < len(s); i++ {
		if _, ok := allowed[s[i]]; !ok {
			t.Fatalf("unexpected character %q at index %d", s[i], i)
		}
	}
}

func TestResponseWriter_Flush(t *testing.T) {
	t.Parallel()
	recorder := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: recorder, statusCode: http.StatusOK}

	// Recorder implements http.Flusher, so Flush should not panic.
	rw.Flush()

	if !recorder.Flushed {
		t.Error("expected underlying writer to be flushed")
	}
}

func TestResponseWriter_Unwrap(t *testing.T) {
	t.Parallel()
	recorder := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: recorder, statusCode: http.StatusOK}

	if rw.Unwrap() != recorder {
		t.Error("Unwrap should return the underlying ResponseWriter")
	}
}

func TestResponseWriter_WriteHeader(t *testing.T) {
	t.Parallel()
	recorder := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: recorder, statusCode: http.StatusOK}

	rw.WriteHeader(http.StatusNotFound)

	if rw.statusCode != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, rw.statusCode)
	}
	if recorder.Code != http.StatusNotFound {
		t.Errorf("expected recorder status %d, got %d", http.StatusNotFound, recorder.Code)
	}
}

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestAuthMiddleware_ValidBearerToken(t *testing.T) {
	t.Parallel()
	logger := newTestLogger()
	keys := []models.ProxyKey{{Key: "test-key-123", Application: "testapp"}}

	handler := AuthMiddleware(keys, logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		app, _ := r.Context().Value(ContextKeyApplication).(string)
		if app != "testapp" {
			t.Errorf("expected application 'testapp', got %q", app)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer test-key-123")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
}

func TestAuthMiddleware_ValidXAPIKey(t *testing.T) {
	t.Parallel()
	logger := newTestLogger()
	keys := []models.ProxyKey{{Key: "api-key-456", Application: "apiapp"}}

	handler := AuthMiddleware(keys, logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("x-api-key", "api-key-456")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
}

func TestAuthMiddleware_MissingToken(t *testing.T) {
	t.Parallel()
	logger := newTestLogger()
	keys := []models.ProxyKey{{Key: "test-key", Application: "app"}}

	handler := AuthMiddleware(keys, logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rec.Code)
	}
}

func TestAuthMiddleware_InvalidToken(t *testing.T) {
	t.Parallel()
	logger := newTestLogger()
	keys := []models.ProxyKey{{Key: "valid-key", Application: "app"}}

	handler := AuthMiddleware(keys, logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer wrong-key")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rec.Code)
	}
}

func TestAuthMiddleware_SetsApplicationOnResponseWriter(t *testing.T) {
	t.Parallel()
	logger := newTestLogger()
	keys := []models.ProxyKey{{Key: "key1", Application: "myapp"}}

	var captured string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if rw, ok := w.(*responseWriter); ok {
			captured = rw.application
		}
		w.WriteHeader(http.StatusOK)
	})

	// Wrap with LoggingMiddleware (creates responseWriter) then AuthMiddleware
	logging := LoggingMiddleware(logger)(AuthMiddleware(keys, logger)(inner))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer key1")
	rec := httptest.NewRecorder()
	logging.ServeHTTP(rec, req)

	if captured != "myapp" {
		t.Errorf("expected application 'myapp' on responseWriter, got %q", captured)
	}
}

func TestRecoverMiddleware_PanicRecovery(t *testing.T) {
	t.Parallel()
	logger := newTestLogger()

	handler := RecoverMiddleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", rec.Code)
	}
}

func TestRequestIDMiddleware_SetsHeader(t *testing.T) {
	t.Parallel()

	handler := RequestIDMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := r.Context().Value(ContextKeyRequestID).(string)
		if id == "" {
			t.Error("expected request ID in context")
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Header().Get("X-Request-ID") == "" {
		t.Error("expected X-Request-ID response header")
	}
}
