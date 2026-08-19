package mcp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"visionmcp/internal/config"
	"visionmcp/internal/glm"
	"visionmcp/internal/logging"
	"visionmcp/internal/vision"
)

func TestEndToEndAnalyzeImage(t *testing.T) {
	var gotPath string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if gotPath = r.URL.Path; gotPath != "/" {
			t.Errorf("path = %q", gotPath)
		}
		var req glm.Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("Decode() error = %v", err)
		}
		if req.Model != "glm-4.6v-flash" {
			t.Errorf("model = %q", req.Model)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"integration ok"}}]}`))
	}))
	defer api.Close()

	logger, closeFn, err := logging.New("", "error")
	if err != nil {
		t.Fatal(err)
	}
	defer closeFn()
	server := Server{GLM: glm.Client{Endpoint: api.URL, APIKey: "key", Model: "glm-4.6v-flash"}, Vision: vision.Service{MaxImageMB: 1}, Logger: logger}
	input := bytes.NewBuffer(nil)
	encoder := json.NewEncoder(input)
	_ = encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize"})
	_ = encoder.Encode(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	_ = encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": map[string]any{
		"name": "analyze_image",
		"arguments": map[string]any{
			"prompt": "Describe the image.",
			"image":  map[string]string{"data": base64.StdEncoding.EncodeToString(vision.BlankPNG())},
		},
	}})
	var output bytes.Buffer
	if err := server.Serve(context.Background(), input, &output); err != nil {
		t.Fatal(err)
	}
	lines := bytes.Split(bytes.TrimSpace(output.Bytes()), []byte("\n"))
	if len(lines) != 2 {
		t.Fatalf("response lines = %d: %s", len(lines), output.String())
	}
	var result struct {
		Result CallToolResult `json:"result"`
	}
	if err := json.Unmarshal(lines[1], &result); err != nil {
		t.Fatal(err)
	}
	if result.Result.IsError || result.Result.Content[0]["text"] != "integration ok" {
		t.Fatalf("result = %+v", result.Result)
	}
}

func TestEndToEndOpenAICompatBaseURL(t *testing.T) {
	var gotPath string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.Header.Get("Authorization") != "Bearer sk-test" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		var req glm.Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("Decode() error = %v", err)
		}
		if req.Model != "gpt-4o" {
			t.Errorf("model = %q", req.Model)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"openai ok"}}]}`))
	}))
	defer api.Close()

	cfg, err := config.Parse([]string{
		"--api-key", "sk-test",
		"--base-url", api.URL,
		"--model", "gpt-4o",
	}, func(string) string { return "" })
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if cfg.APIEndpoint != api.URL+"/chat/completions" {
		t.Fatalf("APIEndpoint = %q", cfg.APIEndpoint)
	}
	logger, closeFn, err := logging.New("", "error")
	if err != nil {
		t.Fatal(err)
	}
	defer closeFn()
	server := Server{GLM: glm.Client{Endpoint: cfg.APIEndpoint, APIKey: cfg.APIKey, Model: cfg.Model}, Vision: vision.Service{MaxImageMB: 1}, Logger: logger}
	input := bytes.NewBuffer(nil)
	encoder := json.NewEncoder(input)
	_ = encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{
		"name": "analyze_image",
		"arguments": map[string]any{
			"prompt": "What is in this image?",
			"image":  map[string]string{"data": base64.StdEncoding.EncodeToString(vision.BlankPNG())},
		},
	}})
	var output bytes.Buffer
	if err := server.Serve(context.Background(), input, &output); err != nil {
		t.Fatal(err)
	}
	var result struct {
		Result CallToolResult `json:"result"`
	}
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Result.IsError || result.Result.Content[0]["text"] != "openai ok" {
		t.Fatalf("result = %+v", result.Result)
	}
	if gotPath != "/chat/completions" {
		t.Fatalf("path = %q, want /chat/completions", gotPath)
	}
}
