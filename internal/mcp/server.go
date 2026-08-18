package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"visionmcp/internal/glm"
	"visionmcp/internal/logging"
	"visionmcp/internal/vision"
)

type Server struct {
	GLM     glm.Client
	Vision  vision.Service
	Logger  *logging.Logger
	Timeout time.Duration
}

type AnalyzeArguments struct {
	Prompt string        `json:"prompt"`
	Image  vision.Source `json:"image"`
}

func (s Server) Serve(ctx context.Context, input io.Reader, output io.Writer) error {
	reader := bufio.NewReader(input)
	writer := newSyncWriter(output)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF && strings.TrimSpace(line) == "" {
				return nil
			}
			return err
		}
		response := s.handleLine(ctx, line)
		if response == nil {
			continue
		}
		encoded, err := json.Marshal(response)
		if err != nil {
			encoded = []byte(fmt.Sprintf(`{"jsonrpc":"2.0","id":null,"error":{"code":-32603,"message":"failed to encode response"}}`))
		}
		if _, err := writer.write(encoded); err != nil {
			return err
		}
	}
}

type syncWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func newSyncWriter(w io.Writer) *syncWriter { return &syncWriter{w: w} }

func (w *syncWriter) write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.w.Write(append(data, '\n'))
}

func (s Server) handleLine(ctx context.Context, line string) *Response {
	var request Request
	if err := json.Unmarshal([]byte(line), &request); err != nil {
		return &Response{JSONRPC: "2.0", Error: &Error{Code: CodeParseError, Message: "parse error"}}
	}
	logger := s.Logger.With(map[string]any{"method": request.Method})
	if len(request.ID) > 0 {
		logger = logger.With(map[string]any{"request_id": string(request.ID)})
	}
	switch request.Method {
	case "initialize":
		result := InitializeResult{
			ProtocolVersion: ProtocolVersion,
			Capabilities:    map[string]any{"tools": map[string]any{}},
			ServerInfo:      map[string]any{"name": "visionmcp", "version": "0.1.0"},
			Instructions:    "Use analyze_image whenever visual perception is needed. Supply a concise task prompt and exactly one local absolute or relative path, HTTP(S) URL, or base64 image.",
		}
		logger.Infof("initialized")
		return &Response{JSONRPC: "2.0", ID: request.ID, Result: result}
	case "ping":
		return &Response{JSONRPC: "2.0", ID: request.ID, Result: map[string]any{}}
	case "notifications/initialized":
		logger.Debugf("client initialized")
		return nil
	case "tools/list":
		return &Response{JSONRPC: "2.0", ID: request.ID, Result: ListToolsResult{Tools: []Tool{{
			Name:        "analyze_image",
			Description: "Analyze one image with GLM-4.6V-Flash and return text for non-multimodal models.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"prompt": map[string]any{"type": "string", "description": "Question or extraction task for the image"},
					"image": map[string]any{
						"type":        "object",
						"description": "Exactly one image source",
						"properties": map[string]any{
							"url":  map[string]any{"type": "string", "description": "HTTP(S) image URL"},
							"path": map[string]any{"type": "string", "description": "Absolute image path, or a path relative to the MCP server working directory"},
							"data": map[string]any{"type": "string", "description": "Raw base64 image bytes"},
						},
						"additionalProperties": false,
					},
				},
				"required":             []string{"prompt", "image"},
				"additionalProperties": false,
			},
		}}}}
	case "tools/call":
		var params struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(request.Params, &params); err != nil || params.Name != "analyze_image" {
			return &Response{JSONRPC: "2.0", ID: request.ID, Error: NewTextError("unknown tool")}
		}
		var arguments AnalyzeArguments
		if err := json.Unmarshal(params.Arguments, &arguments); err != nil {
			return &Response{JSONRPC: "2.0", ID: request.ID, Error: NewTextError("invalid arguments")}
		}
		if strings.TrimSpace(arguments.Prompt) == "" {
			return &Response{JSONRPC: "2.0", ID: request.ID, Error: NewTextError("prompt is required")}
		}
		imageURL, err := s.Vision.Load(arguments.Image)
		if err != nil {
			logger.ErrorFields("failed to load image", map[string]any{"error": err.Error()})
			return &Response{JSONRPC: "2.0", ID: request.ID, Result: ErrorResult(err)}
		}
		callCtx, cancel := context.WithTimeout(ctx, s.timeout())
		answer, err := s.GLM.Complete(callCtx, arguments.Prompt, imageURL)
		cancel()
		if err != nil {
			logger.ErrorFields("failed to analyze image", map[string]any{"error": err.Error()})
			return &Response{JSONRPC: "2.0", ID: request.ID, Result: ErrorResult(err)}
		}
		logger.Infof("analyzed image")
		return &Response{JSONRPC: "2.0", ID: request.ID, Result: TextResult(answer)}
	default:
		return &Response{JSONRPC: "2.0", ID: request.ID, Error: NewMethodError(request.Method)}
	}
}

func (s Server) timeout() time.Duration {
	if s.Timeout <= 0 {
		return 120 * time.Second
	}
	return s.Timeout
}
