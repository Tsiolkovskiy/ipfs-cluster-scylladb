package scyllastate

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gocql/gocql"
	logging "github.com/ipfs/go-log/v2"
)

// Logger for ScyllaDB state operations
var structuredLogger = logging.Logger("scyllastate")

// CidInterface represents a generic CID interface for logging
type CidInterface interface {
	String() string
	Bytes() []byte
}

// LoggingConfig holds configuration for structured logging
type LoggingConfig struct {
	TracingEnabled bool
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

// StructuredLogger provides structured logging capabilities for ScyllaDB operations
type StructuredLogger struct {
	logger logging.StandardLogger
	config *LoggingConfig
}

// NewStructuredLogger creates a new structured logger instance
func NewStructuredLogger(config *LoggingConfig) *StructuredLogger {
	return &StructuredLogger{
		logger: structuredLogger,
		config: config,
	}
}

// LogOperation logs a ScyllaDB operation with full context and structured fields
func (sl *StructuredLogger) LogOperation(ctx context.Context, opCtx *OperationContext) {
	fields := sl.buildLogFields(opCtx)

	if opCtx.Error != nil {
		errorCode := sl.classifyError(opCtx.Error)

		// Create log message with error details
		msg := fmt.Sprintf("ScyllaDB operation failed: %s (error_code=%s, error_type=%T)",
			opCtx.Error.Error(), string(errorCode), opCtx.Error)

		// Log at appropriate level based on error severity
		switch errorCode {
		case ErrorCodeTimeout, ErrorCodeUnavailable:
			sl.logger.Warnf("%s %s", msg, sl.formatFields(fields))
		case ErrorCodeConnectionFailed, ErrorCodeUnauthorized:
			sl.logger.Errorf("%s %s", msg, sl.formatFields(fields))
		default:
			sl.logger.Errorf("%s %s", msg, sl.formatFields(fields))
		}
	} else {
		// Log successful operations
		msg := fmt.Sprintf("ScyllaDB operation completed: %s", sl.formatFields(fields))

		if opCtx.Duration > 5*time.Second {
			// Warn about slow operations
			sl.logger.Warnf("Slow operation detected: %s", msg)
		} else if sl.config.TracingEnabled {
			// Debug level for successful operations when tracing is enabled
			sl.logger.Debugf("Operation trace: %s", msg)
		} else {
			// Info level for normal successful operations
			sl.logger.Infof("Operation completed: %s", msg)
		}
	}
}

// LogQuery logs individual CQL query execution with detailed context
func (sl *StructuredLogger) LogQuery(ctx context.Context, query string, args []interface{}, duration time.Duration, err error) {
	queryType := sl.extractQueryType(query)
	queryHash := sl.hashQuery(query)

	fields := map[string]interface{}{
		"query_type":  queryType,
		"query_hash":  queryHash,
		"duration_ms": duration.Milliseconds(),
		"args_count":  len(args),
	}

	// Add query details for debug tracing
	if sl.config.TracingEnabled {
		fields["query"] = sl.sanitizeQuery(query)
		if len(args) > 0 {
			fields["args"] = sl.sanitizeArgs(args)
		}
	}

	if err != nil {
		errorCode := sl.classifyError(err)
		fields["error_code"] = string(errorCode)
		fields["error_message"] = err.Error()

		sl.logger.Errorf("CQL query failed: %s", sl.formatFields(fields))
	} else {
		if duration > 1*time.Second {
			sl.logger.Warnf("Slow CQL query detected: %s", sl.formatFields(fields))
		} else if sl.config.TracingEnabled {
			sl.logger.Debugf("CQL query executed: %s", sl.formatFields(fields))
		}
	}
}

// LogRetry logs retry attempts with exponential backoff context
func (sl *StructuredLogger) LogRetry(operation string, attempt int, delay time.Duration, lastError error, reason string) {
	fields := map[string]interface{}{
		"operation":      operation,
		"retry_attempt":  attempt,
		"retry_delay_ms": delay.Milliseconds(),
		"retry_reason":   reason,
	}

	if lastError != nil {
		errorCode := sl.classifyError(lastError)
		fields["last_error_code"] = string(errorCode)
		fields["last_error"] = lastError.Error()
	}

	sl.logger.Infof("Retrying ScyllaDB operation: %s", sl.formatFields(fields))
}

// LogBatchOperation logs batch operations with size and performance metrics
func (sl *StructuredLogger) LogBatchOperation(batchType string, size int, duration time.Duration, err error) {
	fields := map[string]interface{}{
		"batch_type":             batchType,
		"batch_size":             size,
		"duration_ms":            duration.Milliseconds(),
		"throughput_ops_per_sec": float64(size) / duration.Seconds(),
	}

	if err != nil {
		errorCode := sl.classifyError(err)
		fields["error_code"] = string(errorCode)
		fields["error_message"] = err.Error()

		sl.logger.Errorf("Batch operation failed: %s", sl.formatFields(fields))
	} else {
		if duration > 10*time.Second {
			sl.logger.Warnf("Slow batch operation detected: %s", sl.formatFields(fields))
		} else {
			sl.logger.Infof("Batch operation completed: %s", sl.formatFields(fields))
		}
	}
}

// LogConnectionEvent logs connection-related events for monitoring
func (sl *StructuredLogger) LogConnectionEvent(eventType string, host string, datacenter string, details map[string]interface{}) {
	fields := map[string]interface{}{
		"event_type": eventType,
		"host":       host,
		"datacenter": datacenter,
	}

	// Add additional details
	for key, value := range details {
		fields[key] = value
	}

	switch eventType {
	case "connection_established", "connection_restored":
		sl.logger.Infof("ScyllaDB connection event: %s", sl.formatFields(fields))
	case "connection_lost", "connection_failed":
		sl.logger.Warnf("ScyllaDB connection issue: %s", sl.formatFields(fields))
	case "node_down", "node_removed":
		sl.logger.Errorf("ScyllaDB node unavailable: %s", sl.formatFields(fields))
	default:
		sl.logger.Debugf("ScyllaDB connection event: %s", sl.formatFields(fields))
	}
}

// LogDegradationEvent logs graceful degradation events
func (sl *StructuredLogger) LogDegradationEvent(eventType string, level int, details map[string]interface{}) {
	fields := map[string]interface{}{
		"degradation_event": eventType,
		"degradation_level": level,
	}

	for key, value := range details {
		fields[key] = value
	}

	switch eventType {
	case "degradation_activated":
		sl.logger.Warnf("Graceful degradation activated: %s", sl.formatFields(fields))
	case "degradation_deactivated":
		sl.logger.Infof("Graceful degradation deactivated: %s", sl.formatFields(fields))
	case "consistency_downgraded":
		sl.logger.Warnf("Consistency level downgraded: %s", sl.formatFields(fields))
	case "consistency_restored":
		sl.logger.Infof("Consistency level restored: %s", sl.formatFields(fields))
	default:
		sl.logger.Infof("Degradation event: %s", sl.formatFields(fields))
	}
}

// LogPerformanceMetrics logs performance-related metrics for monitoring
func (sl *StructuredLogger) LogPerformanceMetrics(metrics map[string]interface{}) {
	sl.logger.Infof("ScyllaDB performance metrics: %s", sl.formatFields(metrics))
}

// LogMigrationEvent logs data migration events
func (sl *StructuredLogger) LogMigrationEvent(eventType string, progress map[string]interface{}) {
	fields := map[string]interface{}{
		"migration_event": eventType,
	}

	for key, value := range progress {
		fields[key] = value
	}

	switch eventType {
	case "migration_started":
		sl.logger.Infof("Data migration started: %s", sl.formatFields(fields))
	case "migration_progress":
		sl.logger.Infof("Data migration progress: %s", sl.formatFields(fields))
	case "migration_completed":
		sl.logger.Infof("Data migration completed: %s", sl.formatFields(fields))
	case "migration_failed":
		sl.logger.Errorf("Data migration failed: %s", sl.formatFields(fields))
	default:
		sl.logger.Debugf("Migration event: %s", sl.formatFields(fields))
	}
}

// buildLogFields constructs structured log fields from operation context
func (sl *StructuredLogger) buildLogFields(opCtx *OperationContext) map[string]interface{} {
	fields := map[string]interface{}{
		"operation":   opCtx.Operation,
		"duration_ms": opCtx.Duration.Milliseconds(),
	}

	// Add CID if present
	if opCtx.CID != nil {
		fields["cid"] = opCtx.CID.String()
	}

	// Add peer ID if present
	if opCtx.PeerID != "" {
		fields["peer_id"] = opCtx.PeerID
	}

	// Add retry information
	if opCtx.Retries > 0 {
		fields["retries"] = opCtx.Retries
	}

	// Add consistency level
	if opCtx.Consistency != "" {
		fields["consistency"] = opCtx.Consistency
	}

	// Add query type
	if opCtx.QueryType != "" {
		fields["query_type"] = opCtx.QueryType
	}

	// Add batch size if applicable
	if opCtx.BatchSize > 0 {
		fields["batch_size"] = opCtx.BatchSize
	}

	// Add record count if applicable
	if opCtx.RecordCount > 0 {
		fields["record_count"] = opCtx.RecordCount
	}

	// Add custom metadata
	for key, value := range opCtx.Metadata {
		fields[key] = value
	}

	return fields
}

// formatFields formats a map of fields into a readable string
func (sl *StructuredLogger) formatFields(fields map[string]interface{}) string {
	if len(fields) == 0 {
		return ""
	}

	var parts []string
	for key, value := range fields {
		parts = append(parts, fmt.Sprintf("%s=%v", key, value))
	}

	return strings.Join(parts, " ")
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
	// Simple regex-like replacement for quoted strings
	result := ""
	inQuote := false
	for _, char := range sanitized {
		if char == '\'' {
			if !inQuote {
				result += "'"
				inQuote = true
			} else {
				result += "***'"
				inQuote = false
			}
		} else if !inQuote {
			result += string(char)
		}
	}
	sanitized = result

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
