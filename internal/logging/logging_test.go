package logging

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestJSONLoggingAndLevels(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log.ndjson")
	logger, closeFn, err := New(path, "info")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer closeFn()
	logger.Debugf("hidden %s", "detail")
	logger.With(map[string]any{"request_id": "abc"}).Infof("started")
	logger.Errorf("failed")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("lines = %d, want 2: %q", len(lines), string(data))
	}
	if !strings.Contains(lines[0], `"level":"info"`) || !strings.Contains(lines[0], `"request_id":"abc"`) {
		t.Fatalf("unexpected first line: %s", lines[0])
	}
	if !strings.Contains(lines[1], `"level":"error"`) {
		t.Fatalf("unexpected second line: %s", lines[1])
	}
}

func TestParseLevel(t *testing.T) {
	if got, _ := ParseLevel("warning"); got != Warn {
		t.Fatalf("ParseLevel(warning) = %v", got)
	}
	if _, err := ParseLevel("bad"); err == nil {
		t.Fatal("ParseLevel(bad) expected error")
	}
}
