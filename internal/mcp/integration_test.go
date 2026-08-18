package mcp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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
