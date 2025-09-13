package scyllastate

import (
	"context"

	"github.com/gocql/gocql"
)

// QueryObserver implements gocql.QueryObserver for detailed query tracing
type QueryObserver struct {
	logger *StructuredLogger
}

// NewQueryObserver creates a new query observer for tracing
func NewQueryObserver(logger *StructuredLogger) *QueryObserver {
	return &QueryObserver{
		logger: logger,
	}
}

// ObserveQuery is called for each query execution when tracing is enabled
func (qo *QueryObserver) ObserveQuery(ctx context.Context, q gocql.ObservedQuery) {
	// Extract query information
	queryType := qo.logger.extractQueryType(q.Statement)

	// Log the query execution with detailed context
	qo.logger.LogQuery(
		ctx,
		q.Statement,
		q.Values,
		q.End.Sub(q.Start),
		q.Err,
	)

	// Log additional tracing information if available
	if q.Metrics != nil {
		metrics := map[string]interface{}{
			"query_type":    queryType,
			"attempts":      q.Metrics.Attempts,
			"total_latency": q.Metrics.TotalLatency,
		}

		qo.logger.LogPerformanceMetrics(metrics)
	}
}

// BatchObserver implements gocql.BatchObserver for batch operation tracing
type BatchObserver struct {
	logger *StructuredLogger
}

// NewBatchObserver creates a new batch observer for tracing
func NewBatchObserver(logger *StructuredLogger) *BatchObserver {
	return &BatchObserver{
		logger: logger,
	}
}

// ObserveBatch is called for each batch execution when tracing is enabled
func (bo *BatchObserver) ObserveBatch(ctx context.Context, b gocql.ObservedBatch) {
	// Determine batch type
	batchType := "LOGGED"
	switch b.BatchType {
	case gocql.UnloggedBatch:
		batchType = "UNLOGGED"
	case gocql.CounterBatch:
		batchType = "COUNTER"
	}

	// Log the batch execution
	bo.logger.LogBatchOperation(
		batchType,
		len(b.Statements),
		b.End.Sub(b.Start),
		b.Err,
	)
}

// ConnectObserver implements connection event observation
type ConnectObserver struct {
	logger *StructuredLogger
}

// NewConnectObserver creates a new connection observer
func NewConnectObserver(logger *StructuredLogger) *ConnectObserver {
	return &ConnectObserver{
		logger: logger,
	}
}

// ObserveConnect is called when connection events occur
func (co *ConnectObserver) ObserveConnect(o gocql.ObservedConnect) {
	details := map[string]interface{}{
		"duration_ms": o.End.Sub(o.Start).Milliseconds(),
		"host":        o.Host.ConnectAddress().String(),
	}

	if o.Err != nil {
		details["error"] = o.Err.Error()
		co.logger.LogConnectionEvent("connection_failed", o.Host.ConnectAddress().String(), o.Host.DataCenter(), details)
	} else {
		co.logger.LogConnectionEvent("connection_established", o.Host.ConnectAddress().String(), o.Host.DataCenter(), details)
	}
}
