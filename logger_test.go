package logging

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/one-harsh/context-logging/loggingtest"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestLogger_Fatal_LogsAndExits(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=TestLoggerFatalHelperProcess")
	cmd.Env = append(os.Environ(), "CONTEXT_LOGGING_FATAL_HELPER=1")

	output, err := cmd.CombinedOutput()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected subprocess exit error, got %v with output %s", err, string(output))
	}
	if exitErr.ExitCode() != 1 {
		t.Fatalf("exit code = %d, want 1, output=%s", exitErr.ExitCode(), string(output))
	}

	logOutput := string(output)
	if !strings.Contains(logOutput, `"level":"fatal"`) {
		t.Fatalf("expected fatal log level in output: %s", logOutput)
	}
	if !strings.Contains(logOutput, `"msg":"fatal failure"`) {
		t.Fatalf("expected fatal message in output: %s", logOutput)
	}
	if !strings.Contains(logOutput, `"tenant_id":"tenant-abc"`) {
		t.Fatalf("expected tenant_id in output: %s", logOutput)
	}
}

func TestFromZap_UsesProvidedLogger(t *testing.T) {
	var buf bytes.Buffer

	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
		zapcore.AddSync(&buf),
		zap.NewAtomicLevelAt(zapcore.InfoLevel),
	)
	z := zap.New(core)

	logger := FromZap(z,
		WithFields(Region("us-west-2"), Component("gateway")),
		WithStrictFields(),
	)

	ctx := Bind(context.Background(), RequestID("req-456"), TenantID("tenant-xyz"))
	logger.WithContext(ctx).Info("handled")

	entry := loggingtest.LastEntryFromBytes(t, buf.Bytes())
	if got := entry["region"]; got != "us-west-2" {
		t.Fatalf("region = %v, want us-west-2", got)
	}
	if got := entry["component"]; got != "gateway" {
		t.Fatalf("component = %v, want gateway", got)
	}
	if got := entry["request_id"]; got != "req-456" {
		t.Fatalf("request_id = %v, want req-456", got)
	}
	if got := entry["tenant_id"]; got != "tenant-xyz" {
		t.Fatalf("tenant_id = %v, want tenant-xyz", got)
	}
}

func TestFromZap_StrictFields_DedupsInline(t *testing.T) {
	var buf bytes.Buffer

	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
		zapcore.AddSync(&buf),
		zap.NewAtomicLevelAt(zapcore.InfoLevel),
	)
	z := zap.New(core)

	logger := FromZap(z, WithStrictFields())

	ctx := Bind(context.Background(), RequestID("req-456"))
	logger.WithContext(ctx).Info("handled", RequestID("req-inline"))

	if count := strings.Count(buf.String(), `"request_id"`); count != 1 {
		t.Fatalf("expected 1 request_id key, got %d in: %s", count, buf.String())
	}
}

func TestFromZap_NoOptions(t *testing.T) {
	var buf bytes.Buffer

	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
		zapcore.AddSync(&buf),
		zap.NewAtomicLevelAt(zapcore.InfoLevel),
	)
	z := zap.New(core)

	logger := FromZap(z)
	logger.Background().Info("bare")

	entry := loggingtest.LastEntryFromBytes(t, buf.Bytes())
	if got := entry["msg"]; got != "bare" {
		t.Fatalf("msg = %v, want bare", got)
	}
}

func TestLoggerFatalHelperProcess(t *testing.T) {
	if os.Getenv("CONTEXT_LOGGING_FATAL_HELPER") != "1" {
		t.Skip("Not a subprocess call")
	}

	logger, err := New(Config{Output: os.Stdout})
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}

	logger.Background().Fatal("fatal failure", TenantID("tenant-abc"))
}
