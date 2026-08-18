package mcp

import (
	"encoding/json"
	"fmt"
)

const ProtocolVersion = "2025-06-18"

type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternalError  = -32603
)

type InitializeResult struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    map[string]any `json:"capabilities"`
	ServerInfo      map[string]any `json:"serverInfo"`
	Instructions    string         `json:"instructions"`
}

type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type ListToolsResult struct {
	Tools []Tool `json:"tools"`
}

type CallToolResult struct {
	Content []map[string]any `json:"content"`
	IsError bool             `json:"isError,omitempty"`
}

func TextResult(value string) CallToolResult {
	return CallToolResult{Content: []map[string]any{{"type": "text", "text": value}}}
}

func ErrorResult(err error) CallToolResult {
	return CallToolResult{Content: []map[string]any{{"type": "text", "text": err.Error()}}, IsError: true}
}

func NewTextError(message string) *Error {
	return &Error{Code: CodeInvalidParams, Message: message}
}

func NewMethodError(method string) *Error {
	return &Error{Code: CodeMethodNotFound, Message: fmt.Sprintf("method not found: %s", method)}
}
