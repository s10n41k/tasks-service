package signature

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type contextKey string

const (
	CtxSignatureVerified contextKey = "signature_verified"
	CtxUserID            contextKey = "user_id"
	CtxSessionID         contextKey = "session_id"
	CtxUserRoles         contextKey = "user_roles"
)

func Middleware(h http.HandlerFunc) http.HandlerFunc {
	secret := os.Getenv("GATEWAY_SIGN")
	if secret == "" {
		secret = os.Getenv("SIGN_SECRET")
	}
	if secret == "" {
		panic("GATEWAY_SIGN or SIGN_SECRET environment variable is required")
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if isPublicRoute(r.URL.Path) {
			h(w, r)
			return
		}

		sig := r.Header.Get("X-Signature")
		timestampStr := r.Header.Get("X-Timestamp")
		serviceName := r.Header.Get("X-Service-Name")

		if sig == "" || timestampStr == "" {
			http.Error(w, `{"error": "Signature required"}`, http.StatusUnauthorized)
			return
		}

		if serviceName != "gateway" {
			http.Error(w, `{"error": "Invalid service source"}`, http.StatusUnauthorized)
			return
		}

		timestamp, err := strconv.ParseInt(timestampStr, 10, 64)
		if err != nil {
			http.Error(w, `{"error": "Invalid timestamp"}`, http.StatusUnauthorized)
			return
		}

		if time.Since(time.Unix(timestamp, 0)) > 5*time.Minute {
			http.Error(w, `{"error": "Request too old"}`, http.StatusUnauthorized)
			return
		}

		userID := r.Header.Get("X-User-ID")
		sessionID := r.Header.Get("X-Session-ID")

		parts := []string{r.Method, r.URL.Path, timestampStr}
		if userID != "" {
			parts = append(parts, userID)
		}
		if sessionID != "" {
			parts = append(parts, sessionID)
		}

		dataToSign := strings.Join(parts, "|")

		hmacHash := hmac.New(sha256.New, []byte(secret))
		hmacHash.Write([]byte(dataToSign))
		expectedSignature := hex.EncodeToString(hmacHash.Sum(nil))

		if !hmac.Equal([]byte(sig), []byte(expectedSignature)) {
			http.Error(w, `{"error": "Invalid signature"}`, http.StatusUnauthorized)
			return
		}

		ctx := r.Context()
		ctx = context.WithValue(ctx, CtxSignatureVerified, true)
		ctx = context.WithValue(ctx, CtxUserID, userID)
		ctx = context.WithValue(ctx, CtxSessionID, sessionID)

		if roles := r.Header.Get("X-User-Roles"); roles != "" {
			ctx = context.WithValue(ctx, CtxUserRoles, strings.Split(roles, ","))
		}

		r = r.WithContext(ctx)
		h(w, r)
	}
}

func isPublicRoute(path string) bool {
	publicRoutes := []string{
		"/health",
		"/metrics",
		"/ready",
		"/live",
		"/docs",
		"/swagger",
		"/favicon.ico",
	}

	for _, route := range publicRoutes {
		if strings.HasPrefix(path, route) {
			return true
		}
	}
	return false
}
