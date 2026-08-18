package mcp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"visionmcp/internal/glm"
	"visionmcp/internal/logging"
	"visionmcp/internal/vision"
)

func testServer(t *testing.T, prompt string, imageURL string, answer string) Server {
	t.Helper()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req glm.Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("Decode() error = %v", err)
		}
		if req.Model != "glm-4.6v-flash" || len(req.Messages) != 1 || len(req.Messages[0].Content) != 2 {
			t.Errorf("unexpected request: %+v", req)
		}
		if req.Messages[0].Content[0].ImageURL.URL != imageURL || req.Messages[0].Content[1].Text != prompt {
			t.Errorf("unexpected prompt/image: %+v", req.Messages[0].Content)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"` + answer + `"}}]}`))
	}))
	t.Cleanup(api.Close)
	logger, closeFn, err := logging.New("", "error")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(closeFn)
	return Server{
		GLM:    glm.Client{Endpoint: api.URL, APIKey: "test", Model: "glm-4.6v-flash"},
		Vision: vision.Service{MaxImageMB: 1},
		Logger: logger,
	}
}

func TestInitialize(t *testing.T) {
	server := testServer(t, "", "", "")
	input := bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n")
	var output bytes.Buffer
	if err := server.Serve(context.Background(), input, &output); err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
	var response Response
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if response.Error != nil || response.Result == nil {
		t.Fatalf("unexpected response: %+v", response)
	}
	var result map[string]any
	resultBytes, _ := json.Marshal(response.Result)
	if err := json.Unmarshal(resultBytes, &result); err != nil {
		t.Fatal(err)
	}
	if result["protocolVersion"] != ProtocolVersion {
		t.Fatalf("protocolVersion = %v", result["protocolVersion"])
	}
}

func TestListToolsSchema(t *testing.T) {
	server := testServer(t, "", "", "")
	input := bytes.NewBufferString(`{"jsonrpc":"2.0","id":"list","method":"tools/list"}` + "\n")
	var output bytes.Buffer
	if err := server.Serve(context.Background(), input, &output); err != nil {
		t.Fatal(err)
	}
	var response struct {
		Result ListToolsResult `json:"result"`
	}
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Result.Tools) != 1 || response.Result.Tools[0].Name != "analyze_image" {
		t.Fatalf("tools = %+v", response.Result.Tools)
	}
	schema := response.Result.Tools[0].InputSchema
	required, _ := schema["required"].([]any)
	if len(required) != 2 {
		t.Fatalf("required = %#v", schema["required"])
	}
}

func TestCallTool(t *testing.T) {
	imageData := base64.StdEncoding.EncodeToString(vision.BlankPNG())
	dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(vision.BlankPNG())
	server := testServer(t, "describe", dataURL, "a blank image")
	request := map[string]any{"jsonrpc": "2.0", "id": 42, "method": "tools/call", "params": map[string]any{
		"name":      "analyze_image",
		"arguments": map[string]any{"prompt": "describe", "image": map[string]string{"data": imageData}},
	}}
	encoded, _ := json.Marshal(request)
	input := bytes.NewBuffer(append(encoded, '\n'))
	var output bytes.Buffer
	if err := server.Serve(context.Background(), input, &output); err != nil {
		t.Fatal(err)
	}
	var response struct {
		Result CallToolResult `json:"result"`
	}
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Result.IsError || len(response.Result.Content) != 1 || response.Result.Content[0]["text"] != "a blank image" {
		t.Fatalf("result = %+v", response.Result)
	}
}

func TestCallToolInputError(t *testing.T) {
	server := testServer(t, "", "", "")
	input := bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"analyze_image","arguments":{"prompt":"","image":{"data":"abc"}}}}` + "\n")
	var output bytes.Buffer
	if err := server.Serve(context.Background(), input, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"prompt is required"`) {
		t.Fatalf("output = %s", output.String())
	}
}

func TestNotificationNoResponse(t *testing.T) {
	server := testServer(t, "", "", "")
	input := bytes.NewBufferString(`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n")
	var output bytes.Buffer
	if err := server.Serve(context.Background(), input, &output); err != nil {
		t.Fatal(err)
	}
	if output.Len() != 0 {
		t.Fatalf("output = %s", output.String())
	}
}

func TestUnknownMethodAndParseError(t *testing.T) {
	server := testServer(t, "", "", "")
	input := bytes.NewBufferString("not-json\n{\"jsonrpc\":\"2.0\",\"id\":2,\"method\":\"unknown\"}\n")
	var output bytes.Buffer
	if err := server.Serve(context.Background(), input, &output); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], "-32700") || !strings.Contains(lines[1], "-32601") {
		t.Fatalf("output = %s", output.String())
	}
}
