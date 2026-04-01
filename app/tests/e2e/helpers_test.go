//go:build e2e

package e2e

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// signedReq собирает HTTP-запрос с валидной HMAC-подписью.
// userID передаётся через заголовок X-User-ID и включается в подпись.
func signedReq(t *testing.T, method, path, userID string, body interface{}) *http.Request {
	t.Helper()

	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, testServer.URL+path, bodyReader)
	require.NoError(t, err)

	ts := fmt.Sprintf("%d", time.Now().Unix())
	req.Header.Set("X-Timestamp", ts)
	req.Header.Set("X-Service-Name", "gateway")
	if userID != "" {
		req.Header.Set("X-User-ID", userID)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	parts := []string{method, path, ts}
	if userID != "" {
		parts = append(parts, userID)
	}
	data := strings.Join(parts, "|")
	h := hmac.New(sha256.New, []byte(testGatewaySign))
	h.Write([]byte(data))
	req.Header.Set("X-Signature", hex.EncodeToString(h.Sum(nil)))

	return req
}

// do выполняет запрос и возвращает ответ.
func do(t *testing.T, req *http.Request) *http.Response {
	t.Helper()
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

// decodeJSON читает тело ответа и декодирует JSON в dst.
func decodeJSON(t *testing.T, resp *http.Response, dst interface{}) {
	t.Helper()
	defer resp.Body.Close()
	require.NoError(t, json.NewDecoder(resp.Body).Decode(dst))
}

// readBody читает и возвращает тело ответа как строку.
func readBody(resp *http.Response) string {
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return strings.TrimSpace(string(b))
}

// url строит полный URL для теста.
func url(path string) string {
	return testServer.URL + path
}
