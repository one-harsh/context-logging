package logging

import (
	"context"
	"maps"
	"sort"
	"sync"
)

type boundFieldsKey struct{}
type summaryAccumulatorKey struct{}

type summaryAccumulator struct {
	mu     sync.Mutex
	fields map[string]LoggingField
}

// Bind attaches structured fields to a plain context.Context.
// Later bindings override earlier values for the same field key.
func Bind(ctx context.Context, fields ...LoggingField) context.Context {
	ctx = normalizedContext(ctx)

	existing, _ := ctx.Value(boundFieldsKey{}).(map[string]LoggingField)
	next := make(map[string]LoggingField, len(existing)+len(fields))
	maps.Copy(next, existing)
	for _, field := range fields {
		next[field.key] = field
	}

	ctx = context.WithValue(ctx, boundFieldsKey{}, next)
	ctx, _ = summaryAccumulatorFromContext(ctx)
	return ctx
}

// BindSummary promotes fields into the request summary view without changing the
// ordinary fields visible to WithContext on the returned context.
func BindSummary(ctx context.Context, fields ...LoggingField) context.Context {
	ctx = normalizedContext(ctx)
	ctx, acc := summaryAccumulatorFromContext(ctx)
	acc.set(fields...)
	return ctx
}

func fieldsFromContext(ctx context.Context) []LoggingField {
	if ctx == nil {
		return nil
	}

	fieldMap, ok := ctx.Value(boundFieldsKey{}).(map[string]LoggingField)
	if !ok || len(fieldMap) == 0 {
		return nil
	}
	return sortedFields(fieldMap)
}

func summaryFieldsFromContext(ctx context.Context) []LoggingField {
	if ctx == nil {
		return nil
	}

	merged := make(map[string]LoggingField)

	fieldMap, ok := ctx.Value(boundFieldsKey{}).(map[string]LoggingField)
	if ok {
		merged = make(map[string]LoggingField, len(fieldMap))
		maps.Copy(merged, fieldMap)
	}

	acc, ok := ctx.Value(summaryAccumulatorKey{}).(*summaryAccumulator)
	if ok && acc != nil {
		for _, field := range acc.fieldsSnapshot() {
			merged[field.key] = field
		}
	}

	if len(merged) == 0 {
		return nil
	}
	return sortedFields(merged)
}

func sortedFields(fieldMap map[string]LoggingField) []LoggingField {
	keys := make([]string, 0, len(fieldMap))
	for key := range fieldMap {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	fields := make([]LoggingField, 0, len(fieldMap))
	for _, key := range keys {
		fields = append(fields, fieldMap[key])
	}
	return fields
}

func (a *summaryAccumulator) set(fields ...LoggingField) {
	a.mu.Lock()
	defer a.mu.Unlock()

	for _, field := range fields {
		a.fields[field.key] = field
	}
}

func (a *summaryAccumulator) fieldsSnapshot() []LoggingField {
	a.mu.Lock()
	defer a.mu.Unlock()

	if len(a.fields) == 0 {
		return nil
	}

	copied := make(map[string]LoggingField, len(a.fields))
	maps.Copy(copied, a.fields)
	return sortedFields(copied)
}

func normalizedContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func summaryAccumulatorFromContext(ctx context.Context) (context.Context, *summaryAccumulator) {
	if acc, ok := ctx.Value(summaryAccumulatorKey{}).(*summaryAccumulator); ok && acc != nil {
		return ctx, acc
	}

	acc := &summaryAccumulator{
		fields: make(map[string]LoggingField),
	}
	return context.WithValue(ctx, summaryAccumulatorKey{}, acc), acc
}

// RequestIDFromContext extracts `request_id` if it was previously bound as a string field.
func RequestIDFromContext(ctx context.Context) string {
	fieldMap, ok := ctx.Value(boundFieldsKey{}).(map[string]LoggingField)
	if !ok {
		return ""
	}

	field, exists := fieldMap[KeyRequestID]
	if !exists {
		return ""
	}
	value, ok := field.stringValue()
	if !ok {
		return ""
	}
	return value
}
