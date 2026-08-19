package mcp

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"visionmcp/internal/glm"
	"visionmcp/internal/logging"
	"visionmcp/internal/vision"
)

func httpTestServer(t *testing.T, answer string) (*httptest.Server, *int) {
	t.Helper()
	var calls int
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		calls++
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"` + answer + `"}}]}`))
	}))
	t.Cleanup(api.Close)
	logger, closeFn, err := logging.New("", "error")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(closeFn)
	server := HTTPServer{Server: Server{
		GLM:    glm.Client{Endpoint: api.URL, APIKey: "key", Model: "glm-4.6v-flash"},
		Vision: vision.Service{MaxImageMB: 1},
		Logger: logger,
	}}
	srv := httptest.NewServer(server.Handler())
	t.Cleanup(srv.Close)
	return srv, &calls
}

func postJSON(t *testing.T, url, body, accept string) (*http.Response, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url+HTTPMCPPath, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp, data
}

func TestHTTPInitialize(t *testing.T) {
	srv, _ := httpTestServer(t, "x")
	resp, data := postJSON(t, srv.URL, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`, "application/json")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, data)
	}
	if resp.Header.Get("Mcp-Session-Id") == "" {
		t.Fatalf("missing Mcp-Session-Id header")
	}
	var response Response
	if err := json.Unmarshal(data, &response); err != nil {
		t.Fatal(err)
	}
	if response.Error != nil || response.Result == nil {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestHTTPInitializeSSE(t *testing.T) {
	srv, _ := httpTestServer(t, "x")
	resp, data := postJSON(t, srv.URL, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`, "text/event-stream")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, data)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("Content-Type = %q, want text/event-stream", ct)
	}
	if !strings.Contains(string(data), "event: message") || !strings.Contains(string(data), "data: {") {
		t.Fatalf("unexpected SSE body: %s", data)
	}
}

func TestHTTPNotificationAccepted(t *testing.T) {
	srv, _ := httpTestServer(t, "x")
	resp, data := postJSON(t, srv.URL, `{"jsonrpc":"2.0","method":"notifications/initialized"}`, "application/json")
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d: %s", resp.StatusCode, data)
	}
	if len(data) != 0 {
		t.Fatalf("notification must have empty body, got %s", data)
	}
}

func TestHTTPToolsCall(t *testing.T) {
	srv, calls := httpTestServer(t, "http ok")
	imageData := base64.StdEncoding.EncodeToString(vision.BlankPNG())
	body := `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"analyze_image","arguments":{"prompt":"describe","image":{"data":"` + imageData + `"}}}}`
	resp, data := postJSON(t, srv.URL, body, "application/json")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, data)
	}
	var response struct {
		Result CallToolResult `json:"result"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		t.Fatal(err)
	}
	if response.Result.IsError || response.Result.Content[0]["text"] != "http ok" {
		t.Fatalf("result = %+v", response.Result)
	}
	if *calls != 1 {
		t.Fatalf("model calls = %d, want 1", *calls)
	}
}

func TestHTTPParseError(t *testing.T) {
	srv, _ := httpTestServer(t, "x")
	resp, data := postJSON(t, srv.URL, "not-json", "application/json")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	if !strings.Contains(string(data), "-32700") {
		t.Fatalf("body = %s, want parse error code", data)
	}
}

func TestHTTPMethodNotAllowed(t *testing.T) {
	srv, _ := httpTestServer(t, "x")
	req, err := http.NewRequest(http.MethodGet, srv.URL+HTTPMCPPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", resp.StatusCode)
	}
}

func TestHTTPConcurrentCalls(t *testing.T) {
	srv, calls := httpTestServer(t, "concurrent ok")
	imageData := base64.StdEncoding.EncodeToString(vision.BlankPNG())
	body := `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"analyze_image","arguments":{"prompt":"describe","image":{"data":"` + imageData + `"}}}}`
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, data := postJSON(t, srv.URL, body, "application/json")
			if resp.StatusCode != http.StatusOK {
				t.Errorf("status = %d: %s", resp.StatusCode, data)
				return
			}
			var response struct {
				Result CallToolResult `json:"result"`
			}
			if err := json.Unmarshal(data, &response); err != nil {
				t.Error(err)
				return
			}
			if response.Result.IsError || response.Result.Content[0]["text"] != "concurrent ok" {
				t.Errorf("result = %+v", response.Result)
			}
		}()
	}
	wg.Wait()
	if *calls != 8 {
		t.Fatalf("model calls = %d, want 8", *calls)
	}
}
