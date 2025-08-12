package enum

type DefaultLogKey string

// DefaultLogKey represents the default keys used in log entries.
// These keys are used to standardize the fields in log entries for consistency.
// They can be used to set default fields in the logger configuration.
// The keys are used to identify specific attributes in log entries, such as time, level, message, error, etc.
// The keys are defined as constants for easy reference and to avoid typos.
// The keys can be used in conjunction with the LogAttr struct to create structured log entries.
// The keys can be used to set default fields in the logger configuration.
// The keys can be used to extract static fields from the environment.
// The keys can be used to extract context fields from the context.
// The keys can be used to set default fields in the logger configuration.
const (
	DefaultLogKeyTime          DefaultLogKey = "time"
	DefaultLogKeyLevel         DefaultLogKey = "level"
	DefaultLogKeyMessage       DefaultLogKey = "message"
	DefaultLogKeyError         DefaultLogKey = "error"
	DefaultLogKeyCaller        DefaultLogKey = "caller"
	DefaultLogKeyContext       DefaultLogKey = "context"
	DefaultLogKeyDuration      DefaultLogKey = "duration"
	DefaultLogKeyFields        DefaultLogKey = "fields"
	DefaultLogKeySource        DefaultLogKey = "source"
	DefaultLogKeyStatic        DefaultLogKey = "static"
	DefaultLogKeyEnv           DefaultLogKey = "env"
	DefaultLogKeyHost          DefaultLogKey = "host"
	DefaultLogKeyService       DefaultLogKey = "service"
	DefaultLogKeyVersion       DefaultLogKey = "version"
	DefaultLogKeyRequest       DefaultLogKey = "request"
	DefaultLogKeyResponse      DefaultLogKey = "response"
	DefaultLogKeyUser          DefaultLogKey = "user"
	DefaultLogKeySession       DefaultLogKey = "session"
	DefaultLogKeyTraceID       DefaultLogKey = "trace_id"
	DefaultLogKeySpanID        DefaultLogKey = "span_id"
	DefaultLogKeyCorrelationID DefaultLogKey = "correlation_id"
	DefaultLogKeyComponent     DefaultLogKey = "component"
	DefaultLogKeyOperation     DefaultLogKey = "operation"
	DefaultLogKeyStatus        DefaultLogKey = "status"
	DefaultLogKeyLatency       DefaultLogKey = "latency"
	DefaultLogKeyRequestID     DefaultLogKey = "request_id"
	DefaultLogKeyResponseTime  DefaultLogKey = "response_time"
	DefaultLogKeyClientIP      DefaultLogKey = "client_ip"
	DefaultLogKeyServerIP      DefaultLogKey = "server_ip"
	DefaultLogKeyProtocol      DefaultLogKey = "protocol"
	DefaultLogKeyMethod        DefaultLogKey = "method"
	DefaultLogKeyURL           DefaultLogKey = "url"
	DefaultLogKeyStatusCode    DefaultLogKey = "status_code"
	DefaultLogKeyContentType   DefaultLogKey = "content_type"
	DefaultLogKeyContentLength DefaultLogKey = "content_length"
	DefaultLogKeyResponseSize  DefaultLogKey = "response_size"
	DefaultLogKeyRequestSize   DefaultLogKey = "request_size"
	DefaultLogKeyUserAgent     DefaultLogKey = "user_agent"
	DefaultLogKeyReferer       DefaultLogKey = "referer"
	DefaultLogKeyForwardedFor  DefaultLogKey = "forwarded_for"
	DefaultLogKeyCustom        DefaultLogKey = "custom"
)
