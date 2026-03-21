package logging

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
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
