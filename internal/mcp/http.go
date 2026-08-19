package mcp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// HTTPMCPPath is the endpoint path served by the HTTP mode. Codex connects to
// <base-url>/mcp when the server is registered with a url like
// http://127.0.0.1:8765/mcp.
const HTTPMCPPath = "/mcp"

// HTTPServer implements the MCP Streamable HTTP transport on top of the same
// JSON-RPC handling used by the stdio server. A single process serves every
// agent (Codex, Claude, etc.) that connects to it, so image analysis and model
// calls are shared instead of being re-created per client.
type HTTPServer struct {
	Server Server
}

// Handler returns an http.Handler for the Streamable HTTP transport.
func (h HTTPServer) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(HTTPMCPPath, h.handle)
	return mux
}

// handle processes a single Streamable HTTP request.
//
// The Streamable HTTP transport (MCP spec) works like this:
//   - POST a JSON-RPC message with Accept: application/json or
//     text/event-stream.
//   - Responses are either a single JSON-RPC document (application/json) or an
//     SSE stream. Non-streaming notifications are acknowledged with 202.
//   - The initialize request returns a Mcp-Session-Id header that the client
//     echoes on later requests.
func (h HTTPServer) handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 32<<20))
	if err != nil {
		http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
		return
	}
	var request Request
	if err := json.Unmarshal(body, &request); err != nil {
		writeHTTPError(w, CodeParseError, "parse error")
		return
	}
	stream := wantsSSE(r)
	sessionID := ""
	if request.Method == "initialize" {
		sessionID = newSessionID()
	}
	response := h.Server.handleLine(r.Context(), string(body))
	if response == nil {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	if sessionID != "" {
		w.Header().Set("Mcp-Session-Id", sessionID)
	}
	if stream {
		writeSSE(w, response)
		return
	}
	writeJSON(w, response)
}

// Serve blocks until the context is cancelled, serving HTTP requests on the
// given address. It is used by main in --http mode.
func (h HTTPServer) Serve(ctx context.Context, addr string) error {
	server := &http.Server{Addr: addr, Handler: h.Handler()}
	errCh := make(chan error, 1)
	go func() {
		err := server.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			errCh <- err
			return
		}
		errCh <- nil
	}()
	select {
	case <-ctx.Done():
		_ = server.Shutdown(context.Background())
		return nil
	case err := <-errCh:
		return err
	}
}

func wantsSSE(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	return strings.Contains(accept, "text/event-stream")
}

func newSessionID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "session"
	}
	return hex.EncodeToString(raw[:])
}

func writeJSON(w http.ResponseWriter, response *Response) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

func writeSSE(w http.ResponseWriter, response *Response) {
	data, err := json.Marshal(response)
	if err != nil {
		writeHTTPError(w, CodeInternalError, "failed to encode response")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, response)
		return
	}
	fmt.Fprintf(w, "event: message\ndata: %s\n\n", data)
	flusher.Flush()
}

func writeHTTPError(w http.ResponseWriter, code int, message string) {
	response := &Response{JSONRPC: "2.0", Error: &Error{Code: code, Message: message}}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(response)
}
