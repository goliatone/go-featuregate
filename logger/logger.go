package logger

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
)

// Logger defines a minimal logging contract compatible with go-logger.
type Logger interface {
	Trace(msg string, args ...any)
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
	Fatal(msg string, args ...any)
	WithContext(ctx context.Context) Logger
}

// LoggerProvider returns named loggers.
type LoggerProvider interface {
	GetLogger(name string) Logger
}

// FieldsLogger allows attaching structured fields to a logger.
type FieldsLogger interface {
	WithFields(fields map[string]any) Logger
}

// BasicLogger writes logs to a writer using fmt.Fprintf.
type BasicLogger struct {
	Writer io.Writer
	fields map[string]any
	mu     sync.Mutex
}

// Default returns a usable logger when none is provided.
func Default() Logger {
	return defaultLogger
}

// NewBasicLogger constructs a BasicLogger that logs to stdout by default.
func NewBasicLogger() *BasicLogger {
	return &BasicLogger{
		Writer: os.Stdout,
	}
}

// WithFields implements FieldsLogger.
func (l *BasicLogger) WithFields(fields map[string]any) Logger {
	if l == nil {
		return &BasicLogger{Writer: os.Stdout, fields: copyFields(fields)}
	}
	if len(fields) == 0 {
		return l
	}
	merged := copyFields(l.fields)
	if merged == nil {
		merged = make(map[string]any, len(fields))
	}
	for key, value := range fields {
		merged[key] = value
	}
	return &BasicLogger{
		Writer: l.Writer,
		fields: merged,
	}
}

// WithContext implements Logger.
func (l *BasicLogger) WithContext(ctx context.Context) Logger {
	return l
}

// Trace implements Logger.
func (l *BasicLogger) Trace(msg string, args ...any) { l.log("TRACE", msg, args...) }

// Debug implements Logger.
func (l *BasicLogger) Debug(msg string, args ...any) { l.log("DEBUG", msg, args...) }

// Info implements Logger.
func (l *BasicLogger) Info(msg string, args ...any) { l.log("INFO", msg, args...) }

// Warn implements Logger.
func (l *BasicLogger) Warn(msg string, args ...any) { l.log("WARN", msg, args...) }

// Error implements Logger.
func (l *BasicLogger) Error(msg string, args ...any) { l.log("ERROR", msg, args...) }

// Fatal implements Logger.
func (l *BasicLogger) Fatal(msg string, args ...any) { l.log("FATAL", msg, args...) }

func (l *BasicLogger) log(level string, msg string, args ...any) {
	if l == nil {
		return
	}
	out := l.Writer
	if out == nil {
		out = os.Stdout
	}
	combined := append(fieldsToArgs(l.fields), args...)
	l.mu.Lock()
	defer l.mu.Unlock()
	fmt.Fprintf(out, "[%s] %s %v\n", level, msg, combined)
}

func copyFields(fields map[string]any) map[string]any {
	if len(fields) == 0 {
		return nil
	}
	out := make(map[string]any, len(fields))
	for key, value := range fields {
		out[key] = value
	}
	return out
}

func fieldsToArgs(fields map[string]any) []any {
	if len(fields) == 0 {
		return nil
	}
	args := make([]any, 0, len(fields)*2)
	for key, value := range fields {
		args = append(args, key, value)
	}
	return args
}

var defaultLogger Logger = NewBasicLogger()

var _ Logger = (*BasicLogger)(nil)
var _ FieldsLogger = (*BasicLogger)(nil)
