package logging

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Level int

const (
	Debug Level = iota
	Info
	Warn
	Error
)

func ParseLevel(value string) (Level, error) {
	switch strings.ToLower(value) {
	case "debug":
		return Debug, nil
	case "info", "":
		return Info, nil
	case "warn", "warning":
		return Warn, nil
	case "error":
		return Error, nil
	default:
		return Info, fmt.Errorf("invalid log level %q", value)
	}
}

func (l Level) String() string {
	switch l {
	case Debug:
		return "debug"
	case Info:
		return "info"
	case Warn:
		return "warn"
	default:
		return "error"
	}
}

type Logger struct {
	mu     sync.Mutex
	out    io.Writer
	level  Level
	fields map[string]any
}

func New(path, level string) (*Logger, func(), error) {
	parsedLevel, err := ParseLevel(level)
	if err != nil {
		return nil, nil, err
	}
	logger := &Logger{level: parsedLevel, fields: map[string]any{}}
	if path == "" {
		logger.out = os.Stderr
		return logger, func() {}, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, nil, err
	}
	logger.out = file
	return logger, func() { _ = file.Close() }, nil
}

func (l *Logger) With(fields map[string]any) *Logger {
	if l == nil {
		return nil
	}
	next := &Logger{out: l.out, level: l.level, fields: map[string]any{}}
	for key, value := range l.fields {
		next.fields[key] = value
	}
	for key, value := range fields {
		next.fields[key] = value
	}
	return next
}

func (l *Logger) Log(level Level, message string, fields map[string]any) {
	if l == nil || level < l.level {
		return
	}
	entry := map[string]any{
		"ts":    time.Now().UTC().Format(time.RFC3339Nano),
		"level": level.String(),
		"msg":   message,
	}
	for key, value := range l.fields {
		entry[key] = value
	}
	for key, value := range fields {
		entry[key] = value
	}
	data, err := json.Marshal(entry)
	if err != nil {
		data = []byte(fmt.Sprintf(`{"ts":%q,"level":"error","msg":"failed to encode log entry"}`, time.Now().UTC().Format(time.RFC3339Nano)))
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = fmt.Fprintln(l.out, string(data))
}

func (l *Logger) Debugf(format string, args ...any) { l.Log(Debug, fmt.Sprintf(format, args...), nil) }
func (l *Logger) Infof(format string, args ...any)  { l.Log(Info, fmt.Sprintf(format, args...), nil) }
func (l *Logger) Warnf(format string, args ...any)  { l.Log(Warn, fmt.Sprintf(format, args...), nil) }
func (l *Logger) Errorf(format string, args ...any) { l.Log(Error, fmt.Sprintf(format, args...), nil) }

func (l *Logger) ErrorFields(message string, fields map[string]any) { l.Log(Error, message, fields) }
