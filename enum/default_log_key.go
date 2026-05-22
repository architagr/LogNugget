package enum

// DefaultLogKey is the string key type for the standard fields written into
// every log entry. Pass these constants to config.SetDefaultFields to rename
// a core field, or use them as map keys when reading config.DefaultFields.
type DefaultLogKey string

const (
	// DefaultLogKeyTime is the key for the log entry timestamp field.
	DefaultLogKeyTime DefaultLogKey = "time"
	// DefaultLogKeyLevel is the key for the severity level field.
	DefaultLogKeyLevel DefaultLogKey = "level"
	// DefaultLogKeyMessage is the key for the human-readable log message field.
	DefaultLogKeyMessage DefaultLogKey = "message"
	// DefaultLogKeyError is the key for the error string field.
	DefaultLogKeyError DefaultLogKey = "error"
	// DefaultLogKeyCaller is the key for the call-site function name field.
	DefaultLogKeyCaller DefaultLogKey = "caller"
	// DefaultLogKeyContext is the key for per-request context fields.
	DefaultLogKeyContext DefaultLogKey = "context"
	// DefaultLogKeyDuration is the key for an operation duration field.
	DefaultLogKeyDuration DefaultLogKey = "duration"
	// DefaultLogKeyFields is the key for an arbitrary extra-fields container.
	DefaultLogKeyFields DefaultLogKey = "fields"
	// DefaultLogKeySource is the key for the originating source identifier field.
	DefaultLogKeySource DefaultLogKey = "source"
	// DefaultLogKeyStatic is the key for static environment fields baked in at startup.
	DefaultLogKeyStatic DefaultLogKey = "static"
	// DefaultLogKeyEnv is the key for the deployment environment label (e.g. "prod").
	DefaultLogKeyEnv DefaultLogKey = "env"
	// DefaultLogKeyHost is the key for the hostname field.
	DefaultLogKeyHost DefaultLogKey = "host"
	// DefaultLogKeyService is the key for the service name field.
	DefaultLogKeyService DefaultLogKey = "service"
	// DefaultLogKeyVersion is the key for the application version field.
	DefaultLogKeyVersion DefaultLogKey = "version"
	// DefaultLogKeyRequest is the key for a request payload or identifier field.
	DefaultLogKeyRequest DefaultLogKey = "request"
	// DefaultLogKeyResponse is the key for a response payload or identifier field.
	DefaultLogKeyResponse DefaultLogKey = "response"
	// DefaultLogKeyUser is the key for the acting user identifier field.
	DefaultLogKeyUser DefaultLogKey = "user"
	// DefaultLogKeySession is the key for the session identifier field.
	DefaultLogKeySession DefaultLogKey = "session"
	// DefaultLogKeyTraceID is the key for the distributed-tracing trace ID field.
	DefaultLogKeyTraceID DefaultLogKey = "trace_id"
	// DefaultLogKeySpanID is the key for the distributed-tracing span ID field.
	DefaultLogKeySpanID DefaultLogKey = "span_id"
	// DefaultLogKeyCorrelationID is the key for a cross-service correlation ID field.
	DefaultLogKeyCorrelationID DefaultLogKey = "correlation_id"
	// DefaultLogKeyComponent is the key for the logical component or module name field.
	DefaultLogKeyComponent DefaultLogKey = "component"
	// DefaultLogKeyOperation is the key for the operation or action name field.
	DefaultLogKeyOperation DefaultLogKey = "operation"
	// DefaultLogKeyStatus is the key for an operation status descriptor field.
	DefaultLogKeyStatus DefaultLogKey = "status"
	// DefaultLogKeyLatency is the key for the request latency measurement field.
	DefaultLogKeyLatency DefaultLogKey = "latency"
	// DefaultLogKeyRequestID is the key for a unique request identifier field.
	DefaultLogKeyRequestID DefaultLogKey = "request_id"
	// DefaultLogKeyResponseTime is the key for the total response time field.
	DefaultLogKeyResponseTime DefaultLogKey = "response_time"
	// DefaultLogKeyClientIP is the key for the client IP address field.
	DefaultLogKeyClientIP DefaultLogKey = "client_ip"
	// DefaultLogKeyServerIP is the key for the server IP address field.
	DefaultLogKeyServerIP DefaultLogKey = "server_ip"
	// DefaultLogKeyProtocol is the key for the network protocol field (e.g. "HTTP/1.1").
	DefaultLogKeyProtocol DefaultLogKey = "protocol"
	// DefaultLogKeyMethod is the key for the HTTP (or RPC) method field.
	DefaultLogKeyMethod DefaultLogKey = "method"
	// DefaultLogKeyURL is the key for the request URL field.
	DefaultLogKeyURL DefaultLogKey = "url"
	// DefaultLogKeyStatusCode is the key for the HTTP response status code field.
	DefaultLogKeyStatusCode DefaultLogKey = "status_code"
	// DefaultLogKeyContentType is the key for the Content-Type header field.
	DefaultLogKeyContentType DefaultLogKey = "content_type"
	// DefaultLogKeyContentLength is the key for the Content-Length header field.
	DefaultLogKeyContentLength DefaultLogKey = "content_length"
	// DefaultLogKeyResponseSize is the key for the response body size in bytes field.
	DefaultLogKeyResponseSize DefaultLogKey = "response_size"
	// DefaultLogKeyRequestSize is the key for the request body size in bytes field.
	DefaultLogKeyRequestSize DefaultLogKey = "request_size"
	// DefaultLogKeyUserAgent is the key for the User-Agent header field.
	DefaultLogKeyUserAgent DefaultLogKey = "user_agent"
	// DefaultLogKeyReferer is the key for the Referer header field.
	DefaultLogKeyReferer DefaultLogKey = "referer"
	// DefaultLogKeyForwardedFor is the key for the X-Forwarded-For header field.
	DefaultLogKeyForwardedFor DefaultLogKey = "forwarded_for"
	// DefaultLogKeyCustom is the key for user-defined custom metadata fields.
	DefaultLogKeyCustom DefaultLogKey = "custom"
)
