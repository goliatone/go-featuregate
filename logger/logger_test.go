package logger

import (
	"bytes"
	"strings"
	"testing"
)

func TestBasicLoggerWritesArgs(t *testing.T) {
	var buf bytes.Buffer
	lgr := &BasicLogger{Writer: &buf}

	lgr.Info("hello", "key", "value")
	output := buf.String()
	if !strings.Contains(output, "[INFO] hello") {
		t.Fatalf("expected output to contain message, got %q", output)
	}
	if !strings.Contains(output, "key") || !strings.Contains(output, "value") {
		t.Fatalf("expected output to include args, got %q", output)
	}
}

func TestBasicLoggerWithFields(t *testing.T) {
	var buf bytes.Buffer
	lgr := &BasicLogger{Writer: &buf}
	withFields := lgr.WithFields(map[string]any{
		"feature": "users.signup",
	})

	withFields.Debug("check", "enabled", true)
	output := buf.String()
	if !strings.Contains(output, "feature") || !strings.Contains(output, "users.signup") {
		t.Fatalf("expected output to include fields, got %q", output)
	}
	if !strings.Contains(output, "enabled") || !strings.Contains(output, "true") {
		t.Fatalf("expected output to include args, got %q", output)
	}
}
