package conformance_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	logging "github.com/one-harsh/context-logging"
	"github.com/one-harsh/context-logging/loggingtest"
	"go.opentelemetry.io/otel/trace"
)

func TestLogger_EmitsProcessMetadata(t *testing.T) {
	var buf bytes.Buffer

	logger, err := logging.New(logging.Config{
		Output:      &buf,
		Service:     "context-logging",
		Version:     "dev",
		Environment: "test",
		Region:      "sea1",
	})
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}

	logger.Background().Info("hello")

	entry := loggingtest.LastEntryFromBytes(t, buf.Bytes())
	if got := entry["service"]; got != "context-logging" {
		t.Fatalf("service = %v, want context-logging", got)
	}
	if got := entry["version"]; got != "dev" {
		t.Fatalf("version = %v, want dev", got)
	}
	if got := entry["environment"]; got != "test" {
		t.Fatalf("environment = %v, want test", got)
	}
	if got := entry["region"]; got != "sea1" {
		t.Fatalf("region = %v, want sea1", got)
	}
}

func TestLogger_WithContext_ProjectsBoundFields(t *testing.T) {
	var buf bytes.Buffer

	logger, err := logging.New(logging.Config{Output: &buf})
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}

	ctx := logging.Bind(context.Background(),
		logging.RequestID("req-123"),
		logging.TenantID("tenant-abc"),
		logging.Component("proxy"),
	)

	logger.WithContext(ctx).Info("request completed")

	entry := loggingtest.LastEntryFromBytes(t, buf.Bytes())
	if got := entry["request_id"]; got != "req-123" {
		t.Fatalf("request_id = %v, want req-123", got)
	}
	if got := entry["tenant_id"]; got != "tenant-abc" {
		t.Fatalf("tenant_id = %v, want tenant-abc", got)
	}
	if got := entry["component"]; got != "proxy" {
		t.Fatalf("component = %v, want proxy", got)
	}
}

func TestBind_NilContext_DoesNotPanicAndProjectsFields(t *testing.T) {
	var buf bytes.Buffer

	logger, err := logging.New(logging.Config{Output: &buf})
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}

	ctx := logging.Bind(nil, logging.RequestID("req-123"))

	logger.WithContext(ctx).Info("request completed")

	entry := loggingtest.LastEntryFromBytes(t, buf.Bytes())
	if got := entry["request_id"]; got != "req-123" {
		t.Fatalf("request_id = %v, want req-123", got)
	}
}

func TestBindSummary_NilContext_PromotesSummaryFields(t *testing.T) {
	var buf bytes.Buffer

	logger, err := logging.New(logging.Config{Output: &buf})
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}

	ctx := logging.BindSummary(nil,
		logging.RequestID("req-123"),
		logging.Component("authz"),
	)

	logger.SummaryWithContext(ctx).Info("request summary")

	entry := loggingtest.LastEntryFromBytes(t, buf.Bytes())
	if got := entry["request_id"]; got != "req-123" {
		t.Fatalf("request_id = %v, want req-123", got)
	}
	if got := entry["component"]; got != "authz" {
		t.Fatalf("component = %v, want authz", got)
	}
}

func TestLogger_SummaryWithContext_UsesOnlyFieldsVisibleOnPassedContext(t *testing.T) {
	var buf bytes.Buffer

	logger, err := logging.New(logging.Config{Output: &buf})
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}

	rootCtx := logging.Bind(context.Background(),
		logging.RequestID("req-123"),
		logging.HTTPMethod("GET"),
	)

	derivedCtx := context.WithValue(rootCtx, struct{}{}, "derived")
	derivedCtx = logging.Bind(derivedCtx,
		logging.TenantID("tenant-abc"),
		logging.Component("authz"),
	)

	logger.SummaryWithContext(rootCtx).Info("request summary")

	summaryEntry := loggingtest.LastEntryFromBytes(t, buf.Bytes())
	if got := summaryEntry["request_id"]; got != "req-123" {
		t.Fatalf("summary request_id = %v, want req-123", got)
	}
	if got := summaryEntry["method"]; got != "GET" {
		t.Fatalf("summary method = %v, want GET", got)
	}
	if _, exists := summaryEntry["tenant_id"]; exists {
		t.Fatalf("summary unexpectedly includes derived tenant_id: %v", summaryEntry)
	}
	if _, exists := summaryEntry["component"]; exists {
		t.Fatalf("summary unexpectedly includes derived component: %v", summaryEntry)
	}
}

func TestLogger_SummaryWithContext_IncludesPromotedFieldsAcrossDerivedContexts(t *testing.T) {
	var buf bytes.Buffer

	logger, err := logging.New(logging.Config{Output: &buf})
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}

	rootCtx := logging.Bind(context.Background(),
		logging.RequestID("req-123"),
		logging.HTTPMethod("GET"),
	)

	derivedCtx := context.WithValue(rootCtx, struct{}{}, "derived")
	derivedCtx = logging.BindSummary(derivedCtx,
		logging.TenantID("tenant-abc"),
		logging.Component("authz"),
	)

	logger.SummaryWithContext(rootCtx).Info("request summary")

	summaryEntry := loggingtest.LastEntryFromBytes(t, buf.Bytes())
	if got := summaryEntry["request_id"]; got != "req-123" {
		t.Fatalf("summary request_id = %v, want req-123", got)
	}
	if got := summaryEntry["method"]; got != "GET" {
		t.Fatalf("summary method = %v, want GET", got)
	}
	if got := summaryEntry["tenant_id"]; got != "tenant-abc" {
		t.Fatalf("summary tenant_id = %v, want tenant-abc", got)
	}
	if got := summaryEntry["component"]; got != "authz" {
		t.Fatalf("summary component = %v, want authz", got)
	}
}

func TestLogger_WithContext_DoesNotIncludeSummaryOnlyPromotedFields(t *testing.T) {
	var buf bytes.Buffer

	logger, err := logging.New(logging.Config{Output: &buf})
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}

	rootCtx := logging.Bind(context.Background(), logging.RequestID("req-123"))
	rootCtx = logging.BindSummary(rootCtx, logging.TenantID("tenant-abc"))

	logger.WithContext(rootCtx).Info("request event")

	entry := loggingtest.LastEntryFromBytes(t, buf.Bytes())
	if got := entry["request_id"]; got != "req-123" {
		t.Fatalf("request_id = %v, want req-123", got)
	}
	if _, exists := entry["tenant_id"]; exists {
		t.Fatalf("event unexpectedly includes summary-only tenant_id: %v", entry)
	}
}

func TestLogger_WithContext_EnrichesTraceFields(t *testing.T) {
	var buf bytes.Buffer

	logger, err := logging.New(logging.Config{Output: &buf})
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}

	traceID := trace.TraceID{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}
	spanID := trace.SpanID{2, 2, 2, 2, 2, 2, 2, 2}
	spanCtx := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: traceID,
		SpanID:  spanID,
		Remote:  true,
	})
	ctx := trace.ContextWithSpanContext(context.Background(), spanCtx)

	logger.WithContext(ctx).Info("traced")

	entry := loggingtest.LastEntryFromBytes(t, buf.Bytes())
	if got := entry["trace_id"]; got != traceID.String() {
		t.Fatalf("trace_id = %v, want %s", got, traceID.String())
	}
	if got := entry["span_id"]; got != spanID.String() {
		t.Fatalf("span_id = %v, want %s", got, spanID.String())
	}
}

func TestLogger_WithAuditEvent_AttachesTypedAuditField(t *testing.T) {
	var buf bytes.Buffer

	logger, err := logging.New(logging.Config{Output: &buf})
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}

	logger.Background().WithAuditEvent(logging.AuthnSuccess).Info("request authenticated")

	entry := loggingtest.LastEntryFromBytes(t, buf.Bytes())
	if got := entry["audit_event"]; got != string(logging.AuthnSuccess) {
		t.Fatalf("audit_event = %v, want %s", got, logging.AuthnSuccess)
	}
}

func TestLogger_Dedup_ConfigOverriddenByContext(t *testing.T) {
	var buf bytes.Buffer

	logger, err := logging.New(logging.Config{Output: &buf, Region: "us-east-1"})
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}

	ctx := logging.Bind(context.Background(), logging.Region("us-west-2"))
	logger.WithContext(ctx).Info("msg")

	entry := loggingtest.LastEntryFromBytes(t, buf.Bytes())
	if got := entry["region"]; got != "us-west-2" {
		t.Fatalf("region = %v, want us-west-2", got)
	}
	if count := strings.Count(buf.String(), `"region"`); count != 1 {
		t.Fatalf("expected 1 region key, got %d in: %s", count, buf.String())
	}
}

func TestLogger_StrictFields_InlineDuplicateSkipped(t *testing.T) {
	var buf bytes.Buffer

	logger, err := logging.New(logging.Config{Output: &buf, StrictFields: true})
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}

	ctx := logging.Bind(context.Background(), logging.RequestID("req-123"))
	logger.WithContext(ctx).Info("msg", logging.RequestID("req-123"))

	entry := loggingtest.LastEntryFromBytes(t, buf.Bytes())
	if got := entry["request_id"]; got != "req-123" {
		t.Fatalf("request_id = %v, want req-123", got)
	}
	if count := strings.Count(buf.String(), `"request_id"`); count != 1 {
		t.Fatalf("expected 1 request_id key, got %d in: %s", count, buf.String())
	}
}

func TestLogger_SummaryWithContext_PromotedFieldsOverridePassedContext(t *testing.T) {
	var buf bytes.Buffer

	logger, err := logging.New(logging.Config{Output: &buf})
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}

	rootCtx := logging.Bind(context.Background(), logging.Component("proxy"))
	rootCtx = logging.BindSummary(rootCtx, logging.Component("authz"))

	logger.SummaryWithContext(rootCtx).Info("request summary")

	entry := loggingtest.LastEntryFromBytes(t, buf.Bytes())
	if got := entry["component"]; got != "authz" {
		t.Fatalf("component = %v, want authz", got)
	}
	if count := strings.Count(buf.String(), `"component"`); count != 1 {
		t.Fatalf("expected 1 component key, got %d in: %s", count, buf.String())
	}
}

func TestLogger_StrictFields_InlineDuplicateSkippedAgainstSummaryView(t *testing.T) {
	var buf bytes.Buffer

	logger, err := logging.New(logging.Config{Output: &buf, StrictFields: true})
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}

	rootCtx := logging.Bind(context.Background(), logging.RequestID("req-123"))
	rootCtx = logging.BindSummary(rootCtx, logging.TenantID("tenant-abc"))

	logger.SummaryWithContext(rootCtx).Info("msg",
		logging.RequestID("req-inline"),
		logging.TenantID("tenant-inline"),
	)

	entry := loggingtest.LastEntryFromBytes(t, buf.Bytes())
	if got := entry["request_id"]; got != "req-123" {
		t.Fatalf("request_id = %v, want req-123", got)
	}
	if got := entry["tenant_id"]; got != "tenant-abc" {
		t.Fatalf("tenant_id = %v, want tenant-abc", got)
	}
	if count := strings.Count(buf.String(), `"request_id"`); count != 1 {
		t.Fatalf("expected 1 request_id key, got %d in: %s", count, buf.String())
	}
	if count := strings.Count(buf.String(), `"tenant_id"`); count != 1 {
		t.Fatalf("expected 1 tenant_id key, got %d in: %s", count, buf.String())
	}
}

func TestLogger_Dedup_WithOverriddenByContext(t *testing.T) {
	var buf bytes.Buffer

	logger, err := logging.New(logging.Config{Output: &buf})
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}

	ctx := logging.Bind(context.Background(), logging.Component("authz"))
	logger.With(logging.Component("proxy")).WithContext(ctx).Info("msg")

	entry := loggingtest.LastEntryFromBytes(t, buf.Bytes())
	if got := entry["component"]; got != "authz" {
		t.Fatalf("component = %v, want authz", got)
	}
	if count := strings.Count(buf.String(), `"component"`); count != 1 {
		t.Fatalf("expected 1 component key, got %d in: %s", count, buf.String())
	}
}

func TestRedactionHelpers_HideSensitiveData(t *testing.T) {
	redactedURL := logging.RedactURLString("https://storage.example.com/foo?X-Super-Secret=secret&X-Credential=abc")
	if strings.Contains(redactedURL, "X-Super-Secret") {
		t.Fatalf("redacted URL still contains signature: %s", redactedURL)
	}
	if logging.RedactSecret("super-secret") != "[REDACTED]" {
		t.Fatalf("secret redaction did not redact")
	}
}
