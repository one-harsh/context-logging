package logging

import "time"

type fieldKind uint8

const (
	fieldKindString fieldKind = iota + 1
	fieldKindInt
	fieldKindInt64
	fieldKindBool
	fieldKindDuration
	fieldKindAny
	fieldKindError
)

type LoggingField struct {
	key   string
	kind  fieldKind
	value any
}

const (
	KeyAuditEvent    = "audit_event"
	KeyRequestID     = "request_id"
	KeyCorrelationID = "correlation_id"
	KeyTenantID      = "tenant_id"
	KeyRegion        = "region"
	KeyComponent     = "component"
	KeyOperation     = "operation"
	KeyHTTPMethod    = "method"
	KeyHTTPStatus    = "status"
	KeyGRPCMethod    = "grpc_method"
	KeyDuration      = "duration"
	KeyBytes         = "bytes"
)

type AuditEvent string

const (
	AuthnSuccess   AuditEvent = "authn.success"
	AuthnFailure   AuditEvent = "authn.failure"
	AuthzAllowed   AuditEvent = "authz.allowed"
	AuthzDenied    AuditEvent = "authz.denied"
	ResourceCreate AuditEvent = "resource.create"
	ResourceUpdate AuditEvent = "resource.update"
	ResourceDelete AuditEvent = "resource.delete"
)

func StringField(key, value string) LoggingField {
	return LoggingField{key: key, kind: fieldKindString, value: value}
}

func IntField(key string, value int) LoggingField {
	return LoggingField{key: key, kind: fieldKindInt, value: value}
}

func Int64Field(key string, value int64) LoggingField {
	return LoggingField{key: key, kind: fieldKindInt64, value: value}
}

func BoolField(key string, value bool) LoggingField {
	return LoggingField{key: key, kind: fieldKindBool, value: value}
}

func DurationField(key string, value time.Duration) LoggingField {
	return LoggingField{key: key, kind: fieldKindDuration, value: value}
}

func AnyField(key string, value any) LoggingField {
	return LoggingField{key: key, kind: fieldKindAny, value: value}
}

func ErrorField(err error) LoggingField {
	return LoggingField{key: "error", kind: fieldKindError, value: err}
}

func AuditField(event AuditEvent) LoggingField {
	return StringField(KeyAuditEvent, string(event))
}

func RequestID(value string) LoggingField       { return StringField(KeyRequestID, value) }
func CorrelationID(value string) LoggingField   { return StringField(KeyCorrelationID, value) }
func TenantID(value string) LoggingField        { return StringField(KeyTenantID, value) }
func Region(value string) LoggingField          { return StringField(KeyRegion, value) }
func Component(value string) LoggingField       { return StringField(KeyComponent, value) }
func Operation(value string) LoggingField       { return StringField(KeyOperation, value) }
func HTTPMethod(value string) LoggingField      { return StringField(KeyHTTPMethod, value) }
func HTTPStatus(value int) LoggingField         { return IntField(KeyHTTPStatus, value) }
func GRPCMethod(value string) LoggingField      { return StringField(KeyGRPCMethod, value) }
func Duration(value time.Duration) LoggingField { return DurationField(KeyDuration, value) }
func Bytes(value int) LoggingField              { return IntField(KeyBytes, value) }

func (f LoggingField) stringValue() (string, bool) {
	if f.kind != fieldKindString {
		return "", false
	}
	value, ok := f.value.(string)
	return value, ok
}
