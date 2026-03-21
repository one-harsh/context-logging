# context-logging

`context-logging` is an opinionated Go logging library (zap based) focused on contextual, structured, and safe-by-default logging.

The goal is not to wrap a backend mechanically. The goal is to preserve the behaviors that work well in real systems while shipping a stricter and more reusable API:

- context-bound structured logging by default
- only a context-bound logger may emit log entries
- trace and span enrichment from `context.Context`
- a flat package import for the common API
- a library-owned `LoggingField` type for both built-in and consumer-defined field helpers
- safe-by-default redaction primitives

## Development

### Prerequisites

```bash
brew install goenv
brew install go-task/tap/go-task
```

### Setup

```bash
# Install the pinned Go version
goenv install "$(cat .go-version)"

# Install Go tool dependencies
task install:go-dependencies
```

### Common Tasks

```bash
# Format, test, and lint
task

# Format only
task format

# Tests only
task test

# Lint only
task lint

# Tidy module dependencies
task tidy
```

## Usage

### Basic Logging

```go
package main

import (
	"context"
	"os"

	logging "github.com/one-harsh/context-logging"
)

func main() {
	logger, err := logging.New(logging.Config{
		Output:      os.Stdout,
		Service:     "some-service",
		Version:     "dev",
		Environment: "local",
		Region:      "us-east-west",
	})
	if err != nil {
		panic(err)
	}

	ctx := logging.Bind(context.Background(),
		logging.RequestID("req-123"),
		logging.TenantID("tenant-acme"),
		logging.Operation("foo.create"),
	)

	logger.WithContext(ctx).Info("foo created")
}
```

### Consumer-Defined Fields

The library owns the `LoggingField` type, but consuming packages can define
their own domain-specific helpers without importing backend logging internals.

```go
package objectstore

import logging "github.com/one-harsh/context-logging"

func WorkspaceID(value string) logging.LoggingField {
	return logging.StringField("workspace_id", value)
}

func ControllerName(value string) logging.LoggingField {
	return logging.StringField("controller_name", value)
}
```

That lets consumer code stay explicit and typed at the package boundary:

```go
ctx = logging.Bind(ctx,
	WorkspaceID("01HXYZ..."),
	ControllerName("my-controller"),
)
```

### HTTP Middleware / Request Summary Shape

This is the intended pattern for request-scoped summary logging:

```go
package server

import (
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	logging "github.com/one-harsh/context-logging"
)

func RequestContextMiddleware(logger *logging.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := logging.Bind(r.Context(),
				logging.RequestID(requestIDFromHeaders(r.Header)),
				logging.HTTPMethod(r.Method),
				logging.StringField("path", r.URL.Path),
			)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func SummaryLoggingMiddleware(logger *logging.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sourceIP, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				sourceIP = r.RemoteAddr
			}

			ctx := logging.BindSummary(r.Context(),
				logging.StringField("source_ip", sourceIP),
				logging.StringField("user_agent", r.UserAgent()),
			)
			r = r.WithContext(ctx)

			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			defer func() {
				status := ww.Status()
				level := logging.InfoLevel
				if status >= 500 {
					level = logging.ErrorLevel
				} else if status >= 400 {
					level = logging.WarnLevel
				}

				logger.SummaryWithContext(r.Context()).Log(level, "HTTP request",
					logging.HTTPStatus(status),
					logging.Bytes(ww.BytesWritten()),
					logging.Duration(time.Since(start)),
				)
			}()

			next.ServeHTTP(ww, r)
		})
	}
}

func AuthorizationMiddleware(logger *logging.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tenantID := tenantFromAuthz(r)
			component := "authz"

			ctx := logging.BindSummary(r.Context(),
				logging.TenantID(tenantID),
				logging.Component(component),
			)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
```

Use `Bind(...)` for fields that should be visible to normal logs emitted in the
current request scope. Use `BindSummary(...)` when a deeper layer learns
summary-worthy fields that should appear on the final request summary without
becoming part of every ordinary log line.

`SummaryWithContext(ctx)` sees:

- fields directly visible on the passed context from `Bind(...)`
- fields explicitly promoted through `BindSummary(...)`
- trace and span enrichment from the passed context

If the same key exists in both places, the `BindSummary(...)` value wins for the
summary log line.

### Interceptor-Style Context Propagation

You do not need a framework-specific API to use the library in interceptor-like
flows. The important part is to bind fields early and emit through the bound
logger.

```go
package rpc

import (
	"context"

	logging "github.com/one-harsh/context-logging"
)

func LogUnaryCall(
	ctx context.Context,
	logger *logging.Logger,
	fullMethod string,
	handler func(context.Context) error,
) error {
	ctx = logging.Bind(ctx,
		logging.RequestID("req-123"),
		logging.GRPCMethod(fullMethod),
	)

	bound := logger.WithContext(ctx)
	bound.Info("rpc started")

	err := handler(ctx)
	if err != nil {
		bound.Error("rpc failed", logging.ErrorField(err))
		return err
	}

	bound.Info("rpc completed")
	return nil
}
```

### Background / Startup Paths

For startup checks, workers, and other non-request paths, use `Background()`
explicitly:

```go
if err := initialize(); err != nil {
	logger.Background().Fatal("failed to initialize", logging.ErrorField(err))
}
```

## Field Guidelines

### Context Fields vs Summary Fields vs Inline Fields

Fields in this library fall into three categories:

**Context fields** are bound via `Bind(ctx, ...)` and propagate through the call
chain. Use these for identity and routing data that every log line in a request
scope should carry:

- `RequestID`, `CorrelationID`, `TenantID`
- `Component`, `Operation`
- `HTTPMethod`, `GRPCMethod`

**Summary fields** are bound via `BindSummary(ctx, ...)` and contribute only to
`SummaryWithContext(ctx)`. Use these when a deeper layer learns a field that
should appear on the final request or operation summary, but should not become
part of every ordinary log line:

- final tenant or principal identity established late in the flow
- response-source dimensions gathered near the top-level summary middleware
- selected component or outcome dimensions promoted intentionally for summaries

**Inline fields** are passed directly at the log call site. Use these for
outcomes and measurements that are local to the current call and do not need
propagation:

- `HTTPStatus`, `Duration`, `Bytes`
- `ErrorField`

```go
// Context fields — bound once, visible to every log line in scope.
ctx = logging.Bind(ctx,
	logging.RequestID("req-123"),
	logging.TenantID("tenant-acme"),
)

// Summary fields — only for the final summary logger.
ctx = logging.BindSummary(ctx,
	logging.Component("authz"),
)

// Inline fields — local to this log line only.
logger.WithContext(ctx).Info("request completed",
	logging.HTTPStatus(200),
	logging.Duration(elapsed),
)
```

### Field Deduplication

`WithContext(ctx)` and `SummaryWithContext(ctx)` do not merge the same sources.

`WithContext(ctx)` uses:

**Config < `Logger.With` < context (`Bind`) < trace enrichment**

`SummaryWithContext(ctx)` uses:

**Config < `Logger.With` < context (`Bind`) < summary (`BindSummary`) < trace enrichment**

If `Config` sets `Region: "us-east-1"` and the context binds
`Region("us-west-2")`, the bound logger emits only `"region":"us-west-2"`.

If a context binds `Component("proxy")` and a deeper layer later promotes
`Component("authz")` through `BindSummary(...)`, the summary logger emits only
`"component":"authz"`.

### Strict Field Mode

When `StrictFields` is enabled in `Config`, the library additionally drops
inline fields at emission time if their key is already present on the bound
logger. This guards against defensive duplication — where a developer passes a
field inline without checking whether it was already bound in context.

```go
logger, _ := logging.New(logging.Config{
	Output:       os.Stdout,
	Service:      "my-service",
	StrictFields: true,
})

ctx := logging.Bind(context.Background(), logging.RequestID("req-123"))

// The inline RequestID is silently dropped — the context-bound value wins.
logger.WithContext(ctx).Info("handled", logging.RequestID("req-123"))
```

When `StrictFields` is off (the default), inline fields are passed through to
the backend as-is, with no filtering overhead on the emission path.

## Current Scope

The current bootstrap focuses on:

- the minimal core logger API
- a `Logger` that binds context and a `BoundLogger` that emits
- `Background()` as the explicit non-request emission path
- `SummaryWithContext(ctx)` as the summary projection over local context fields and explicit summary promotion
- context binding, trace enrichment, and explicit summary promotion
- a public `LoggingField` type plus primitive field constructors
- typed audit and field helpers built on top of that field type
- redaction helpers for sensitive logging surfaces
- conformance scaffolding that encodes expected runtime behavior
- field deduplication across Config, `Logger.With`, context binding, and summary promotion (always on), with optional strict-mode inline filtering via `StrictFields`

Application-specific middleware and repo-specific lint policy remain outside the initial library boundary.
