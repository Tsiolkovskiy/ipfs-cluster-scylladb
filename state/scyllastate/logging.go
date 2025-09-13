package scyllastate

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gocql/gocql"
	logging "github.com/ipfs/go-log/v2"
)

// CidInterface represents a generic CID interface for logging
type CidInterface interface {
	String() string
	Bytes() []byte
}

// Logger for ScyllaDB state operations
var logger = logging.Logger("scyllastate")

// LogLevel represents different logging levels for structured logging
type LogLevel int

const (
	LogLevelDebug LogLevel = iota
	LogLevelInfo
	LogLevelWarn
	LogLevelError
)

// OperationContext holds contextual information for logging operations
type OperationContext struct {
	Operation   string                 // Operation name (e.g., "Add", "Get", "List")
	CID         CidInterface           // CID being operated on (if applicable)
	PeerID      string                 // Peer ID involved (if applicable)
	Duration    time.Duration          // Operation duration
	Error       error                  // Error that occurred (if any)
	Retries     int                    // Number of retries attempted
	Consistency string                 // Consistency level used
	QueryType   string                 // Type of CQL query executed
	BatchSize   int                    // Size of batch operation (if applicable)
	RecordCount int64                  // Number of records processed
	Metadata    map[string]interface{} // Additional metadata
}

// ScyllaErrorCode represents ScyllaDB-specific error codes for detailed logging
type ScyllaErrorCode string

const (
	ErrorCodeTimeout          ScyllaErrorCode = "TIMEOUT"
	ErrorCodeUnavailable      ScyllaErrorCode = "UNAVAILABLE"
	ErrorCodeReadTimeout      ScyllaErrorCode = "READ_TIMEOUT"
	ErrorCodeWriteTimeout     ScyllaErrorCode = "WRITE_TIMEOUT"
	ErrorCodeReadFailure      ScyllaErrorCode = "READ_FAILURE"
	ErrorCodeWriteFailure     ScyllaErrorCode = "WRITE_FAILURE"
	ErrorCodeSyntax           ScyllaErrorCode = "SYNTAX_ERROR"
	ErrorCodeUnauthorized     ScyllaErrorCode = "UNAUTHORIZED"
	ErrorCodeInvalid          ScyllaErrorCode = "INVALID"
	ErrorCodeConfigError      ScyllaErrorCode = "CONFIG_ERROR"
	ErrorCodeAlreadyExists    ScyllaErrorCode = "ALREADY_EXISTS"
	ErrorCodeUnprepared       ScyllaErrorCode = "UNPREPARED"
	ErrorCodeConnectionFailed ScyllaErrorCode = "CONNECTION_FAILED"
	ErrorCodeUnknown          ScyllaErrorCode = "UNKNOWN"
)

// LoggingConfig holds configuration for structured logging
type LoggingConfig struct {
	TracingEnabled bool
}

// StructuredLogger provides structured logging capabilities for ScyllaDB operations
type StructuredLogger struct {
	logger logging.StandardLogger
	config *LoggingConfig
}

// NewStructuredLogger creates a new structured logger instance
func NewStructuredLogger(config *LoggingConfig) *StructuredLogger {
	return &StructuredLogger{
		logger: logger,
		config: config,
	}
}

// LogOperation logs a ScyllaDB operation with full context and structured fields
func (sl *StructuredLogger) LogOperation(ctx context.Context, opCtx *OperationContext) {
	fields := sl.buildLogFields(opCtx)

	if opCtx.Error != nil {
		errorCode := sl.classifyError(opCtx.Error)
		fields["error_code"] = string(errorCode)
		fields["error_message"] = opCtx.Error.Error()
		fields["error_type"] = fmt.Sprintf("%T", opCtx.Error)

		// Log at appropriate level based on error severity
		switch errorCode {
		case ErrorCodeTimeout, ErrorCodeUnavailable:
			sl.logger.Warnw("ScyllaDB operation failed with retryable error", fields...)
		case ErrorCodeConnectionFailed, ErrorCodeUnauthorized:
			sl.logger.Errorw("ScyllaDB operation failed with critical error", fields...)
		default:
			sl.logger.Errorw("ScyllaDB operation failed", fields...)
		}
	} else {
		// Log successful operations
		if opCtx.Duration > 5*time.Second {
			// Warn about slow operations
			sl.logger.Warnw("ScyllaDB operation completed slowly", fields...)
		} else if sl.config.TracingEnabled {
			// Debug level for successful operations when tracing is enabled
			sl.logger.Debugw("ScyllaDB operation completed", fields...)
		} else {
			// Info level for normal successful operations
			sl.logger.Infow("ScyllaDB operation completed", fields...)
		}
	}
}

// LogQuery logs individual CQL query execution with detailed context
func (sl *StructuredLogger) LogQuery(ctx context.Context, query string, args []interface{}, duration time.Duration, err error) {
	fields := []interface{}{
		"query_type", sl.extractQueryType(query),
		"query_hash", sl.hashQuery(query),
		"duration_ms", duration.Milliseconds(),
		"args_count", len(args),
	}

	// Add query details for debug tracing
	if sl.config.TracingEnabled {
		fields = append(fields, "query", sl.sanitizeQuery(query))
		if len(args) > 0 {
			fields = append(fields, "args", sl.sanitizeArgs(args))
		}
	}

	if err != nil {
		errorCode := sl.classifyError(err)
		fields = append(fields,
			"error_code", string(errorCode),
			"error_message", err.Error(),
		)
		sl.logger.Errorw("CQL query failed", fields...)
	} else {
		if duration > 1*time.Second {
			sl.logger.Warnw("Slow CQL query detected", fields...)
		} else if sl.config.TracingEnabled {
			sl.logger.Debugw("CQL query executed", fields...)
		}
	}
}

// LogRetry logs retry attempts with exponential backoff context
func (sl *StructuredLogger) LogRetry(operation string, attempt int, delay time.Duration, lastError error, reason string) {
	fields := []interface{}{
		"operation", operation,
		"retry_attempt", attempt,
		"retry_delay_ms", delay.Milliseconds(),
		"retry_reason", reason,
	}

	if lastError != nil {
		errorCode := sl.classifyError(lastError)
		fields = append(fields,
			"last_error_code", string(errorCode),
			"last_error", lastError.Error(),
		)
	}

	sl.logger.Infow("Retrying ScyllaDB operation", fields...)
}

// LogBatchOperation logs batch operations with size and performance metrics
func (sl *StructuredLogger) LogBatchOperation(batchType string, size int, duration time.Duration, err error) {
	fields := []interface{}{
		"batch_type", batchType,
		"batch_size", size,
		"duration_ms", duration.Milliseconds(),
		"throughput_ops_per_sec", float64(size) / duration.Seconds(),
	}

	if err != nil {
		errorCode := sl.classifyError(err)
		fields = append(fields,
			"error_code", string(errorCode),
			"error_message", err.Error(),
		)
		sl.logger.Errorw("Batch operation failed", fields...)
	} else {
		if duration > 10*time.Second {
			sl.logger.Warnw("Slow batch operation detected", fields...)
		} else {
			sl.logger.Infow("Batch operation completed", fields...)
		}
	}
}

// LogConnectionEvent logs connection-related events for monitoring
func (sl *StructuredLogger) LogConnectionEvent(eventType string, host string, datacenter string, details map[string]interface{}) {
	fields := []interface{}{
		"event_type", eventType,
		"host", host,
		"datacenter", datacenter,
	}

	// Add additional details
	for key, value := range details {
		fields = append(fields, key, value)
	}

	switch eventType {
	case "connection_established", "connection_restored":
		sl.logger.Infow("ScyllaDB connection event", fields...)
	case "connection_lost", "connection_failed":
		sl.logger.Warnw("ScyllaDB connection issue", fields...)
	case "node_down", "node_removed":
		sl.logger.Errorw("ScyllaDB node unavailable", fields...)
	default:
		sl.logger.Debugw("ScyllaDB connection event", fields...)
	}
}

// LogDegradationEvent logs graceful degradation events
func (sl *StructuredLogger) LogDegradationEvent(eventType string, level int, details map[string]interface{}) {
	fields := []interface{}{
		"degradation_event", eventType,
		"degradation_level", level,
	}

	for key, value := range details {
		fields = append(fields, key, value)
	}

	switch eventType {
	case "degradation_activated":
		sl.logger.Warnw("Graceful degradation activated", fields...)
	case "degradation_deactivated":
		sl.logger.Infow("Graceful degradation deactivated", fields...)
	case "consistency_downgraded":
		sl.logger.Warnw("Consistency level downgraded", fields...)
	case "consistency_restored":
		sl.logger.Infow("Consistency level restored", fields...)
	default:
		sl.logger.Infow("Degradation event", fields...)
	}
}

// LogPerformanceMetrics logs performance-related metrics for monitoring
func (sl *StructuredLogger) LogPerformanceMetrics(metrics map[string]interface{}) {
	fields := []interface{}{}

	for key, value := range metrics {
		fields = append(fields, key, value)
	}

	sl.logger.Infow("ScyllaDB performance metrics", fields...)
}

// LogMigrationEvent logs data migration events
func (sl *StructuredLogger) LogMigrationEvent(eventType string, progress map[string]interface{}) {
	fields := []interface{}{
		"migration_event", eventType,
	}

	for key, value := range progress {
		fields = append(fields, key, value)
	}

	switch eventType {
	case "migration_started":
		sl.logger.Infow("Data migration started", fields...)
	case "migration_progress":
		sl.logger.Infow("Data migration progress", fields...)
	case "migration_completed":
		sl.logger.Infow("Data migration completed", fields...)
	case "migration_failed":
		sl.logger.Errorw("Data migration failed", fields...)
	default:
		sl.logger.Debugw("Migration event", fields...)
	}
}

// buildLogFields constructs structured log fields from operation context
func (sl *StructuredLogger) buildLogFields(opCtx *OperationContext) []interface{} {
	fields := []interface{}{
		"operation", opCtx.Operation,
		"duration_ms", opCtx.Duration.Milliseconds(),
	}

	// Add CID if present
	if opCtx.CID != nil {
		fields = append(fields, "cid", opCtx.CID.String())
	}

	// Add peer ID if present
	if opCtx.PeerID != "" {
		fields = append(fields, "peer_id", opCtx.PeerID)
	}

	// Add retry information
	if opCtx.Retries > 0 {
		fields = append(fields, "retries", opCtx.Retries)
	}

	// Add consistency level
	if opCtx.Consistency != "" {
		fields = append(fields, "consistency", opCtx.Consistency)
	}

	// Add query type
	if opCtx.QueryType != "" {
		fields = append(fields, "query_type", opCtx.QueryType)
	}

	// Add batch size if applicable
	if opCtx.BatchSize > 0 {
		fields = append(fields, "batch_size", opCtx.BatchSize)
	}

	// Add record count if applicable
	if opCtx.RecordCount > 0 {
		fields = append(fields, "record_count", opCtx.RecordCount)
	}

	// Add custom metadata
	for key, value := range opCtx.Metadata {
		fields = append(fields, key, value)
	}

	return fields
}

// classifyError determines the ScyllaDB error code from a Go error
func (sl *StructuredLogger) classifyError(err error) ScyllaErrorCode {
	if err == nil {
		return ""
	}

	errStr := strings.ToLower(err.Error())

	// Check for specific gocql error types first
	switch err.(type) {
	case *gocql.RequestErrReadTimeout:
		return ErrorCodeReadTimeout
	case *gocql.RequestErrWriteTimeout:
		return ErrorCodeWriteTimeout
	case *gocql.RequestErrReadFailure:
		return ErrorCodeReadFailure
	case *gocql.RequestErrWriteFailure:
		return ErrorCodeWriteFailure
	case *gocql.RequestErrUnavailable:
		return ErrorCodeUnavailable
	case *gocql.RequestErrAlreadyExists:
		return ErrorCodeAlreadyExists
	case *gocql.RequestErrUnprepared:
		return ErrorCodeUnprepared
	}

	// Fallback to string matching for other error types
	switch {
	case strings.Contains(errStr, "timeout"):
		return ErrorCodeTimeout
	case strings.Contains(errStr, "unavailable"):
		return ErrorCodeUnavailable
	case strings.Contains(errStr, "unauthorized") || strings.Contains(errStr, "authentication"):
		return ErrorCodeUnauthorized
	case strings.Contains(errStr, "syntax") || strings.Contains(errStr, "invalid"):
		return ErrorCodeSyntax
	case strings.Contains(errStr, "connection") || strings.Contains(errStr, "connect"):
		return ErrorCodeConnectionFailed
	case strings.Contains(errStr, "config"):
		return ErrorCodeConfigError
	default:
		return ErrorCodeUnknown
	}
}

// extractQueryType extracts the query type from a CQL statement
func (sl *StructuredLogger) extractQueryType(query string) string {
	query = strings.TrimSpace(strings.ToUpper(query))

	switch {
	case strings.HasPrefix(query, "SELECT"):
		return "SELECT"
	case strings.HasPrefix(query, "INSERT"):
		return "INSERT"
	case strings.HasPrefix(query, "UPDATE"):
		return "UPDATE"
	case strings.HasPrefix(query, "DELETE"):
		return "DELETE"
	case strings.HasPrefix(query, "CREATE"):
		return "CREATE"
	case strings.HasPrefix(query, "DROP"):
		return "DROP"
	case strings.HasPrefix(query, "ALTER"):
		return "ALTER"
	case strings.HasPrefix(query, "TRUNCATE"):
		return "TRUNCATE"
	case strings.HasPrefix(query, "BEGIN"):
		return "BATCH"
	default:
		return "OTHER"
	}
}

// hashQuery creates a hash of the query for grouping similar queries
func (sl *StructuredLogger) hashQuery(query string) string {
	// Simple hash based on query structure (remove parameters)
	normalized := strings.ToUpper(strings.TrimSpace(query))

	// Replace parameter placeholders with generic markers
	normalized = strings.ReplaceAll(normalized, "?", "PARAM")

	// Create a simple hash (in production, use a proper hash function)
	hash := 0
	for _, char := range normalized {
		hash = hash*31 + int(char)
	}

	return fmt.Sprintf("q_%x", hash&0xFFFF)
}

// sanitizeQuery removes sensitive information from queries for logging
func (sl *StructuredLogger) sanitizeQuery(query string) string {
	// Remove potential sensitive data patterns
	sanitized := query

	// Replace string literals that might contain sensitive data
	sanitized = strings.ReplaceAll(sanitized, "'", "'***'")

	// Truncate very long queries
	if len(sanitized) > 500 {
		sanitized = sanitized[:500] + "..."
	}

	return sanitized
}

// sanitizeArgs removes sensitive information from query arguments
func (sl *StructuredLogger) sanitizeArgs(args []interface{}) []interface{} {
	sanitized := make([]interface{}, len(args))

	for i, arg := range args {
		switch v := arg.(type) {
		case string:
			// Mask string arguments that might contain sensitive data
			if len(v) > 10 {
				sanitized[i] = v[:3] + "***" + v[len(v)-3:]
			} else {
				sanitized[i] = "***"
			}
		case []byte:
			// Mask binary data
			sanitized[i] = fmt.Sprintf("[]byte{len=%d}", len(v))
		default:
			// Keep other types as-is (numbers, booleans, etc.)
			sanitized[i] = arg
		}
	}

	return sanitized
}

// Helper functions for creating operation contexts

// NewOperationContext creates a new operation context for logging
func NewOperationContext(operation string) *OperationContext {
	return &OperationContext{
		Operation: operation,
		Metadata:  make(map[string]interface{}),
	}
}

// WithCID adds CID context to the operation
func (oc *OperationContext) WithCID(cid CidInterface) *OperationContext {
	oc.CID = cid
	return oc
}

// WithPeerID adds peer ID context to the operation
func (oc *OperationContext) WithPeerID(peerID string) *OperationContext {
	oc.PeerID = peerID
	return oc
}

// WithDuration adds duration context to the operation
func (oc *OperationContext) WithDuration(duration time.Duration) *OperationContext {
	oc.Duration = duration
	return oc
}

// WithError adds error context to the operation
func (oc *OperationContext) WithError(err error) *OperationContext {
	oc.Error = err
	return oc
}

// WithRetries adds retry context to the operation
func (oc *OperationContext) WithRetries(retries int) *OperationContext {
	oc.Retries = retries
	return oc
}

// WithConsistency adds consistency level context to the operation
func (oc *OperationContext) WithConsistency(consistency string) *OperationContext {
	oc.Consistency = consistency
	return oc
}

// WithQueryType adds query type context to the operation
func (oc *OperationContext) WithQueryType(queryType string) *OperationContext {
	oc.QueryType = queryType
	return oc
}

// WithBatchSize adds batch size context to the operation
func (oc *OperationContext) WithBatchSize(size int) *OperationContext {
	oc.BatchSize = size
	return oc
}

// WithRecordCount adds record count context to the operation
func (oc *OperationContext) WithRecordCount(count int64) *OperationContext {
	oc.RecordCount = count
	return oc
}

// WithMetadata adds custom metadata to the operation
func (oc *OperationContext) WithMetadata(key string, value interface{}) *OperationContext {
	oc.Metadata[key] = value
	return oc
}
