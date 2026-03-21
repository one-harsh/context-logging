package logging

import (
	"context"
	"fmt"
	"io"
	"maps"
	"os"
	"time"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Level int8

const (
	DebugLevel Level = iota
	InfoLevel
	WarnLevel
	ErrorLevel
	FatalLevel
)

type Config struct {
	Level        string
	Format       string
	Output       io.Writer
	Service      string
	Version      string
	Environment  string
	Region       string
	StrictFields bool
}

type Logger struct {
	logger       *zap.Logger
	fields       map[string]LoggingField
	strictFields bool
}

type BoundLogger struct {
	logger    *zap.Logger
	knownKeys map[string]struct{}
}

func New(cfg Config) (*Logger, error) {
	levelText := cfg.Level
	if levelText == "" {
		levelText = "info"
	}

	level, err := zapcore.ParseLevel(levelText)
	if err != nil {
		return nil, fmt.Errorf("parse log level: %w", err)
	}

	output := cfg.Output
	if output == nil {
		output = os.Stdout
	}

	base := zap.New(
		zapcore.NewCore(
			encoder(cfg.Format),
			zapcore.AddSync(output),
			zap.NewAtomicLevelAt(level),
		),
		zap.AddCaller(),
	)

	fields := make(map[string]LoggingField, 4)
	if cfg.Service != "" {
		fields["service"] = StringField("service", cfg.Service)
	}
	if cfg.Version != "" {
		fields["version"] = StringField("version", cfg.Version)
	}
	if cfg.Environment != "" {
		fields["environment"] = StringField("environment", cfg.Environment)
	}
	if cfg.Region != "" {
		fields[KeyRegion] = Region(cfg.Region)
	}

	return &Logger{logger: base, fields: fields, strictFields: cfg.StrictFields}, nil
}

func Nop() *Logger {
	return &Logger{logger: zap.NewNop()}
}

func (l *Logger) Named(name string) *Logger {
	return &Logger{logger: l.logger.Named(name), fields: l.fields, strictFields: l.strictFields}
}

func (l *Logger) With(fields ...LoggingField) *Logger {
	merged := make(map[string]LoggingField, len(l.fields)+len(fields))
	maps.Copy(merged, l.fields)
	for _, f := range fields {
		merged[f.key] = f
	}
	return &Logger{logger: l.logger, fields: merged, strictFields: l.strictFields}
}

func (l *Logger) Background() *BoundLogger {
	return l.WithContext(context.Background())
}

func (l *Logger) WithContext(ctx context.Context) *BoundLogger {
	return l.bindFromFields(ctx, fieldsFromContext(ctx))
}

func (l *Logger) SummaryWithContext(ctx context.Context) *BoundLogger {
	return l.bindFromFields(ctx, summaryFieldsFromContext(ctx))
}

func (l *Logger) bindFromFields(ctx context.Context, ctxFields []LoggingField) *BoundLogger {
	if ctx == nil {
		ctx = context.Background()
	}

	merged := make(map[string]LoggingField, len(l.fields)+len(ctxFields)+2)
	maps.Copy(merged, l.fields)
	for _, f := range ctxFields {
		merged[f.key] = f
	}
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		merged["trace_id"] = StringField("trace_id", sc.TraceID().String())
		merged["span_id"] = StringField("span_id", sc.SpanID().String())
	}

	var knownKeys map[string]struct{}
	if l.strictFields {
		knownKeys = make(map[string]struct{}, len(merged))
		for k := range merged {
			knownKeys[k] = struct{}{}
		}
	}

	bakedLogger := l.logger
	if len(merged) > 0 {
		bakedLogger = l.logger.With(toZapFields(sortedFields(merged))...)
	}

	return &BoundLogger{logger: bakedLogger, knownKeys: knownKeys}
}

func (l *Logger) Sync() error {
	return l.logger.Sync()
}

func (l *BoundLogger) With(fields ...LoggingField) *BoundLogger {
	if l.knownKeys == nil {
		return &BoundLogger{logger: l.logger.With(toZapFields(fields)...)}
	}

	var newFields []LoggingField
	knownKeys := make(map[string]struct{}, len(l.knownKeys)+len(fields))
	maps.Copy(knownKeys, l.knownKeys)
	for _, f := range fields {
		if _, exists := l.knownKeys[f.key]; !exists {
			newFields = append(newFields, f)
		}
		knownKeys[f.key] = struct{}{}
	}
	if len(newFields) == 0 {
		return &BoundLogger{logger: l.logger, knownKeys: knownKeys}
	}
	return &BoundLogger{
		logger:    l.logger.With(toZapFields(newFields)...),
		knownKeys: knownKeys,
	}
}

func (l *BoundLogger) WithAuditEvent(event AuditEvent) *BoundLogger {
	return l.With(AuditField(event))
}

func (l *BoundLogger) Debug(msg string, fields ...LoggingField) {
	l.logger.Debug(msg, l.resolveInline(fields)...)
}

func (l *BoundLogger) Info(msg string, fields ...LoggingField) {
	l.logger.Info(msg, l.resolveInline(fields)...)
}

func (l *BoundLogger) Warn(msg string, fields ...LoggingField) {
	l.logger.Warn(msg, l.resolveInline(fields)...)
}

func (l *BoundLogger) Error(msg string, fields ...LoggingField) {
	l.logger.Error(msg, l.resolveInline(fields)...)
}

func (l *BoundLogger) Fatal(msg string, fields ...LoggingField) {
	zapFields := l.resolveInline(fields)
	if checked := l.logger.Check(zapcore.FatalLevel, msg); checked != nil {
		checked.Write(zapFields...)
	}
	_ = l.logger.Sync()
	os.Exit(1)
}

func (l *BoundLogger) Log(level Level, msg string, fields ...LoggingField) {
	if level == FatalLevel {
		l.Fatal(msg, fields...)
		return
	}
	l.logger.Log(toZapLevel(level), msg, l.resolveInline(fields)...)
}

func (l *BoundLogger) Sync() error {
	return l.logger.Sync()
}

func (l *BoundLogger) resolveInline(fields []LoggingField) []zap.Field {
	if len(fields) == 0 {
		return nil
	}
	if l.knownKeys == nil {
		return toZapFields(fields)
	}
	filtered := make([]zap.Field, 0, len(fields))
	for _, f := range fields {
		if _, exists := l.knownKeys[f.key]; !exists {
			filtered = append(filtered, toSingleZapField(f))
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	return filtered
}

func toSingleZapField(f LoggingField) zap.Field {
	switch f.kind {
	case fieldKindString:
		value, _ := f.value.(string)
		return zap.String(f.key, value)
	case fieldKindInt:
		value, _ := f.value.(int)
		return zap.Int(f.key, value)
	case fieldKindInt64:
		value, _ := f.value.(int64)
		return zap.Int64(f.key, value)
	case fieldKindBool:
		value, _ := f.value.(bool)
		return zap.Bool(f.key, value)
	case fieldKindDuration:
		value, _ := f.value.(time.Duration)
		return zap.Duration(f.key, value)
	case fieldKindError:
		value, _ := f.value.(error)
		return zap.Error(value)
	default:
		return zap.Any(f.key, f.value)
	}
}

func toZapFields(fields []LoggingField) []zap.Field {
	if len(fields) == 0 {
		return nil
	}
	zapFields := make([]zap.Field, 0, len(fields))
	for _, f := range fields {
		zapFields = append(zapFields, toSingleZapField(f))
	}
	return zapFields
}

func toZapLevel(level Level) zapcore.Level {
	switch level {
	case DebugLevel:
		return zapcore.DebugLevel
	case WarnLevel:
		return zapcore.WarnLevel
	case ErrorLevel:
		return zapcore.ErrorLevel
	case FatalLevel:
		return zapcore.FatalLevel
	case InfoLevel:
		fallthrough
	default:
		return zapcore.InfoLevel
	}
}

func encoder(format string) zapcore.Encoder {
	if format == "console" {
		cfg := zap.NewDevelopmentEncoderConfig()
		cfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
		cfg.EncodeTime = zapcore.TimeEncoderOfLayout("15:04:05")
		return zapcore.NewConsoleEncoder(cfg)
	}

	cfg := zap.NewProductionEncoderConfig()
	cfg.EncodeTime = zapcore.ISO8601TimeEncoder
	return zapcore.NewJSONEncoder(cfg)
}
