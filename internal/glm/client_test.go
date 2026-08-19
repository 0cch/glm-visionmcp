package glm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCompleteRequestAndResponse(t *testing.T) {
	var receivedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer key" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		if r.URL.Path != "/chat/completions" {
			t.Errorf("Path = %q", r.URL.Path)
		}
		var err error
		receivedBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("ReadAll() error = %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":" red "},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()

	client := Client{Endpoint: server.URL + "/chat/completions", APIKey: "key", Model: "glm-4.6v-flash"}
	got, err := client.Complete(context.Background(), "what color?", "data:image/png;base64,abc")
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if got != "red" {
		t.Fatalf("Complete() = %q", got)
	}
	var request Request
	if err := json.Unmarshal(receivedBody, &request); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if request.Model != "glm-4.6v-flash" || len(request.Messages) != 1 || len(request.Messages[0].Content) != 2 {
		t.Fatalf("unexpected request: %+v", request)
	}
	if request.Messages[0].Content[0].ImageURL.URL != "data:image/png;base64,abc" || request.Messages[0].Content[1].Text != "what color?" {
		t.Fatalf("unexpected request content: %+v", request.Messages[0].Content)
	}
}

func TestCompleteAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"code":"1002","message":"invalid key","type":"invalid_request_error"}}`))
	}))
	defer server.Close()
	client := Client{Endpoint: server.URL, APIKey: "bad", Model: "glm"}
	_, err := client.Complete(context.Background(), "prompt", "data:image/png;base64,abc")
	if err == nil || !strings.Contains(err.Error(), "invalid key") {
		t.Fatalf("Complete() error = %v", err)
	}
}

func TestCompleteEmptyChoices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[]}`))
	}))
	defer server.Close()
	client := Client{Endpoint: server.URL, APIKey: "key", Model: "glm"}
	_, err := client.Complete(context.Background(), "prompt", "data:image/png;base64,abc")
	if err == nil || !strings.Contains(err.Error(), "no choices") {
		t.Fatalf("Complete() error = %v", err)
	}
}

func TestCompleteContextCancel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(20 * time.Millisecond)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	client := Client{Endpoint: server.URL, APIKey: "key", Model: "glm"}
	if _, err := client.Complete(ctx, "prompt", "data:image/png;base64,abc"); err == nil || !strings.Contains(err.Error(), "call API") {
		t.Fatalf("Complete() error = %v", err)
	}
}

func TestCompleteRetriesOverload(t *testing.T) {
	count := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
		if count < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"code":1305,"message":"overloaded"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer server.Close()
	client := Client{Endpoint: server.URL, APIKey: "key", Model: "glm", Retries: 2, RetryInterval: time.Millisecond}
	got, err := client.Complete(context.Background(), "prompt", "data:image/png;base64,abc")
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if got != "ok" || count != 3 {
		t.Fatalf("Complete() = %q, count = %d", got, count)
	}
}

func TestCompleteDoesNotRetryAuthError(t *testing.T) {
	count := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"code":"401","message":"invalid key"}}`))
	}))
	defer server.Close()
	client := Client{Endpoint: server.URL, APIKey: "bad", Model: "glm", Retries: 5, RetryInterval: time.Millisecond}
	_, err := client.Complete(context.Background(), "prompt", "data:image/png;base64,abc")
	if err == nil || !strings.Contains(err.Error(), "invalid key") {
		t.Fatalf("Complete() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("request count = %d", count)
	}
}

func TestShouldRetry(t *testing.T) {
	if !shouldRetry(APIError{Code: 1305}) || !shouldRetry(fmt.Errorf("wrap: %w", &APIError{Code: "1305"})) {
		t.Fatal("expected overload errors to retry")
	}
	if shouldRetry(&APIError{Code: "401"}) {
		t.Fatal("expected auth error not to retry")
	}
}
