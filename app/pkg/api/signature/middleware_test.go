package signature

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

const testSecret = "test-gateway-secret"

// buildRequest создаёт запрос с валидной подписью для тестов.
func buildRequest(t *testing.T, method, path, userID string, timestampUnix int64) *http.Request {
	t.Helper()
	r := httptest.NewRequest(method, path, nil)
	ts := strconv.FormatInt(timestampUnix, 10)
	r.Header.Set("X-Timestamp", ts)
	r.Header.Set("X-Service-Name", "gateway")
	if userID != "" {
		r.Header.Set("X-User-ID", userID)
	}

	parts := []string{method, path, ts}
	if userID != "" {
		parts = append(parts, userID)
	}
	data := strings.Join(parts, "|")
	h := hmac.New(sha256.New, []byte(testSecret))
	h.Write([]byte(data))
	r.Header.Set("X-Signature", hex.EncodeToString(h.Sum(nil)))
	return r
}

func wrapMiddleware(h http.HandlerFunc) http.HandlerFunc {
	t := &testing.T{}
	_ = t
	return Middleware(h)
}

func TestSignature_ValidRequest(t *testing.T) {
	t.Setenv("GATEWAY_SIGN", testSecret)

	called := false
	handler := Middleware(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	ts := time.Now().Unix()
	r := buildRequest(t, http.MethodGet, "/tasks", "user-123", ts)
	w := httptest.NewRecorder()
	handler(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, called)
}

func TestSignature_MissingSignature(t *testing.T) {
	t.Setenv("GATEWAY_SIGN", testSecret)

	handler := Middleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	r := httptest.NewRequest(http.MethodGet, "/tasks", nil)
	r.Header.Set("X-Service-Name", "gateway")
	r.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
	// X-Signature не задан

	w := httptest.NewRecorder()
	handler(w, r)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestSignature_ExpiredTimestamp(t *testing.T) {
	t.Setenv("GATEWAY_SIGN", testSecret)

	handler := Middleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Timestamp 10 минут назад — запрос просрочен
	expired := time.Now().Add(-10 * time.Minute).Unix()
	r := buildRequest(t, http.MethodGet, "/tasks", "user-1", expired)
	w := httptest.NewRecorder()
	handler(w, r)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestSignature_InvalidSignature(t *testing.T) {
	t.Setenv("GATEWAY_SIGN", testSecret)

	handler := Middleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	r := buildRequest(t, http.MethodGet, "/tasks", "user-1", time.Now().Unix())
	r.Header.Set("X-Signature", "deadbeefdeadbeef") // заведомо неверная подпись
	w := httptest.NewRecorder()
	handler(w, r)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestSignature_PublicRoute(t *testing.T) {
	t.Setenv("GATEWAY_SIGN", testSecret)

	called := false
	handler := Middleware(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	// /metrics — публичный роут, подпись не проверяется
	r := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	handler(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, called)
}

func TestSignature_InvalidServiceName(t *testing.T) {
	t.Setenv("GATEWAY_SIGN", testSecret)

	handler := Middleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	ts := time.Now().Unix()
	r := buildRequest(t, http.MethodGet, "/tasks", "user-1", ts)
	r.Header.Set("X-Service-Name", "unknown-service") // не gateway
	w := httptest.NewRecorder()
	handler(w, r)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
