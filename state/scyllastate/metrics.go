package scyllastate

import (
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	// Metric namespace for ScyllaDB state store
	metricsNamespace = "ipfs_cluster"
	metricsSubsystem = "scylladb_state"
)

// PrometheusMetrics holds all Prometheus metrics for ScyllaDB state store
type PrometheusMetrics struct {
	// Operation metrics - latency, throughput, error rates
	operationDuration *prometheus.HistogramVec
	operationCounter  *prometheus.CounterVec
	operationErrors   *prometheus.CounterVec

	// Connection pool metrics
	activeConnections    prometheus.Gauge
	connectionErrors     prometheus.Counter
	connectionPoolStatus *prometheus.GaugeVec

	// State metrics
	totalPins         prometheus.Gauge
	pinOperationsRate *prometheus.CounterVec

	// ScyllaDB specific metrics - timeouts, unavailable errors
	scyllaTimeouts    *prometheus.CounterVec
	scyllaUnavailable *prometheus.CounterVec
	scyllaRetries     *prometheus.CounterVec
	scyllaConsistency *prometheus.GaugeVec

	// Query performance metrics
	queryLatency       *prometheus.HistogramVec
	preparedStatements prometheus.Gauge
	batchOperations    *prometheus.CounterVec
	batchSize          *prometheus.HistogramVec

	// Graceful degradation metrics
	degradationActive prometheus.Gauge
	degradationLevel  *prometheus.GaugeVec
	nodeHealth        *prometheus.GaugeVec
	partitionDetected prometheus.Gauge
}

// NewPrometheusMetrics creates and registers all Prometheus metrics
func NewPrometheusMetrics() *PrometheusMetrics {
	return &PrometheusMetrics{
		// Operation metrics
		operationDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: metricsNamespace,
				Subsystem: metricsSubsystem,
				Name:      "operation_duration_seconds",
				Help:      "Duration of ScyllaDB operations in seconds",
				Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
			},
			[]string{"operation", "consistency_level"},
		),

		operationCounter: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: metricsNamespace,
				Subsystem: metricsSubsystem,
				Name:      "operations_total",
				Help:      "Total number of ScyllaDB operations",
			},
			[]string{"operation", "status"},
		),

		operationErrors: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: metricsNamespace,
				Subsystem: metricsSubsystem,
				Name:      "operation_errors_total",
				Help:      "Total number of ScyllaDB operation errors by type",
			},
			[]string{"operation", "error_type"},
		),

		// Connection pool metrics
		activeConnections: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: metricsNamespace,
				Subsystem: metricsSubsystem,
				Name:      "active_connections",
				Help:      "Number of active connections to ScyllaDB cluster",
			},
		),

		connectionErrors: promauto.NewCounter(
			prometheus.CounterOpts{
				Namespace: metricsNamespace,
				Subsystem: metricsSubsystem,
				Name:      "connection_errors_total",
				Help:      "Total number of connection errors to ScyllaDB",
			},
		),

		connectionPoolStatus: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: metricsNamespace,
				Subsystem: metricsSubsystem,
				Name:      "connection_pool_status",
				Help:      "Status of connection pool (1=healthy, 0=unhealthy)",
			},
			[]string{"host", "datacenter"},
		),

		// State metrics
		totalPins: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: metricsNamespace,
				Subsystem: metricsSubsystem,
				Name:      "total_pins",
				Help:      "Total number of pins stored in ScyllaDB",
			},
		),

		pinOperationsRate: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: metricsNamespace,
				Subsystem: metricsSubsystem,
				Name:      "pin_operations_total",
				Help:      "Total number of pin operations by type",
			},
			[]string{"operation_type"},
		),

		// ScyllaDB specific metrics
		scyllaTimeouts: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: metricsNamespace,
				Subsystem: metricsSubsystem,
				Name:      "scylla_timeouts_total",
				Help:      "Total number of ScyllaDB timeout errors by type",
			},
			[]string{"timeout_type", "operation"},
		),

		scyllaUnavailable: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: metricsNamespace,
				Subsystem: metricsSubsystem,
				Name:      "scylla_unavailable_total",
				Help:      "Total number of ScyllaDB unavailable errors",
			},
			[]string{"operation", "consistency_level"},
		),

		scyllaRetries: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: metricsNamespace,
				Subsystem: metricsSubsystem,
				Name:      "scylla_retries_total",
				Help:      "Total number of ScyllaDB operation retries",
			},
			[]string{"operation", "retry_reason"},
		),

		scyllaConsistency: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: metricsNamespace,
				Subsystem: metricsSubsystem,
				Name:      "scylla_consistency_level",
				Help:      "Current consistency level being used (encoded as number)",
			},
			[]string{"operation_type"},
		),

		// Query performance metrics
		queryLatency: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: metricsNamespace,
				Subsystem: metricsSubsystem,
				Name:      "query_latency_seconds",
				Help:      "Latency of individual CQL queries in seconds",
				Buckets:   []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1},
			},
			[]string{"query_type", "keyspace"},
		),

		preparedStatements: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: metricsNamespace,
				Subsystem: metricsSubsystem,
				Name:      "prepared_statements_cached",
				Help:      "Number of prepared statements currently cached",
			},
		),

		batchOperations: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: metricsNamespace,
				Subsystem: metricsSubsystem,
				Name:      "batch_operations_total",
				Help:      "Total number of batch operations",
			},
			[]string{"batch_type", "status"},
		),

		batchSize: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: metricsNamespace,
				Subsystem: metricsSubsystem,
				Name:      "batch_size",
				Help:      "Size of batch operations (number of statements)",
				Buckets:   []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000},
			},
			[]string{"batch_type"},
		),

		// Graceful degradation metrics
		degradationActive: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: metricsNamespace,
				Subsystem: metricsSubsystem,
				Name:      "degradation_active",
				Help:      "Whether graceful degradation is currently active (1=active, 0=inactive)",
			},
		),

		degradationLevel: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: metricsNamespace,
				Subsystem: metricsSubsystem,
				Name:      "degradation_level",
				Help:      "Current degradation level (0=none, 1=reduced_consistency, 2=local_only, 3=read_only)",
			},
			[]string{"degradation_type"},
		),

		nodeHealth: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: metricsNamespace,
				Subsystem: metricsSubsystem,
				Name:      "node_health",
				Help:      "Health status of ScyllaDB nodes (1=healthy, 0=unhealthy)",
			},
			[]string{"host", "datacenter", "rack"},
		),

		partitionDetected: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: metricsNamespace,
				Subsystem: metricsSubsystem,
				Name:      "partition_detected",
				Help:      "Whether a network partition is currently detected (1=partitioned, 0=not_partitioned)",
			},
		),
	}
}

// RecordOperation records metrics for a completed operation
func (m *PrometheusMetrics) RecordOperation(operation string, duration time.Duration, consistencyLevel string, err error) {
	labels := prometheus.Labels{
		"operation":         operation,
		"consistency_level": consistencyLevel,
	}

	// Record operation duration for successful operations
	if err == nil {
		m.operationDuration.With(labels).Observe(duration.Seconds())
		m.operationCounter.With(prometheus.Labels{
			"operation": operation,
			"status":    "success",
		}).Inc()
	} else {
		m.operationCounter.With(prometheus.Labels{
			"operation": operation,
			"status":    "error",
		}).Inc()

		// Record specific error types
		errorType := classifyError(err)
		m.operationErrors.With(prometheus.Labels{
			"operation":  operation,
			"error_type": errorType,
		}).Inc()

		// Record ScyllaDB specific errors
		if isTimeoutError(err) {
			timeoutType := classifyTimeoutError(err)
			m.scyllaTimeouts.With(prometheus.Labels{
				"timeout_type": timeoutType,
				"operation":    operation,
			}).Inc()
		}

		if isUnavailableError(err) {
			m.scyllaUnavailable.With(prometheus.Labels{
				"operation":         operation,
				"consistency_level": consistencyLevel,
			}).Inc()
		}
	}
}

// RecordQuery records metrics for individual CQL queries
func (m *PrometheusMetrics) RecordQuery(queryType, keyspace string, duration time.Duration) {
	m.queryLatency.With(prometheus.Labels{
		"query_type": queryType,
		"keyspace":   keyspace,
	}).Observe(duration.Seconds())
}

// RecordRetry records a retry attempt
func (m *PrometheusMetrics) RecordRetry(operation, reason string) {
	m.scyllaRetries.With(prometheus.Labels{
		"operation":    operation,
		"retry_reason": reason,
	}).Inc()
}

// RecordBatchOperation records metrics for batch operations
func (m *PrometheusMetrics) RecordBatchOperation(batchType string, size int, err error) {
	status := "success"
	if err != nil {
		status = "error"
	}

	m.batchOperations.With(prometheus.Labels{
		"batch_type": batchType,
		"status":     status,
	}).Inc()

	m.batchSize.With(prometheus.Labels{
		"batch_type": batchType,
	}).Observe(float64(size))
}

// RecordPinOperation records pin-specific operations
func (m *PrometheusMetrics) RecordPinOperation(operationType string) {
	m.pinOperationsRate.With(prometheus.Labels{
		"operation_type": operationType,
	}).Inc()
}

// UpdateConnectionMetrics updates connection-related metrics
func (m *PrometheusMetrics) UpdateConnectionMetrics(activeConns int, poolStatus map[string]bool) {
	m.activeConnections.Set(float64(activeConns))

	for host, healthy := range poolStatus {
		healthValue := float64(0)
		if healthy {
			healthValue = 1
		}
		// Note: datacenter info would need to be passed in for full labeling
		m.connectionPoolStatus.With(prometheus.Labels{
			"host":       host,
			"datacenter": "unknown", // Would be populated with actual DC info
		}).Set(healthValue)
	}
}

// UpdateStateMetrics updates state-related metrics
func (m *PrometheusMetrics) UpdateStateMetrics(totalPins int64) {
	m.totalPins.Set(float64(totalPins))
}

// UpdatePreparedStatements updates prepared statement cache metrics
func (m *PrometheusMetrics) UpdatePreparedStatements(count int) {
	m.preparedStatements.Set(float64(count))
}

// UpdateConsistencyLevel updates the current consistency level metric
func (m *PrometheusMetrics) UpdateConsistencyLevel(operationType string, level string) {
	levelValue := encodeConsistencyLevel(level)
	m.scyllaConsistency.With(prometheus.Labels{
		"operation_type": operationType,
	}).Set(float64(levelValue))
}

// UpdateDegradationMetrics updates graceful degradation metrics
func (m *PrometheusMetrics) UpdateDegradationMetrics(active bool, level int, degradationType string) {
	activeValue := float64(0)
	if active {
		activeValue = 1
	}
	m.degradationActive.Set(activeValue)

	m.degradationLevel.With(prometheus.Labels{
		"degradation_type": degradationType,
	}).Set(float64(level))
}

// UpdateNodeHealth updates node health metrics
func (m *PrometheusMetrics) UpdateNodeHealth(host, datacenter, rack string, healthy bool) {
	healthValue := float64(0)
	if healthy {
		healthValue = 1
	}
	m.nodeHealth.With(prometheus.Labels{
		"host":       host,
		"datacenter": datacenter,
		"rack":       rack,
	}).Set(healthValue)
}

// UpdatePartitionStatus updates network partition detection metric
func (m *PrometheusMetrics) UpdatePartitionStatus(partitioned bool) {
	partitionValue := float64(0)
	if partitioned {
		partitionValue = 1
	}
	m.partitionDetected.Set(partitionValue)
}

// RecordConnectionError records a connection error
func (m *PrometheusMetrics) RecordConnectionError() {
	m.connectionErrors.Inc()
}

// classifyError classifies errors into categories for metrics
func classifyError(err error) string {
	if err == nil {
		return "none"
	}

	errStr := err.Error()
	switch {
	case isTimeoutError(err):
		return "timeout"
	case isUnavailableError(err):
		return "unavailable"
	case contains(errStr, "connection"):
		return "connection"
	case contains(errStr, "authentication"):
		return "authentication"
	case contains(errStr, "authorization"):
		return "authorization"
	case contains(errStr, "syntax"):
		return "syntax"
	case contains(errStr, "invalid"):
		return "invalid_request"
	case contains(errStr, "overloaded"):
		return "overloaded"
	case contains(errStr, "truncated"):
		return "truncated"
	default:
		return "other"
	}
}

// classifyTimeoutError classifies timeout errors into specific types
func classifyTimeoutError(err error) string {
	if err == nil {
		return "none"
	}

	errStr := err.Error()
	switch {
	case contains(errStr, "read timeout"):
		return "read_timeout"
	case contains(errStr, "write timeout"):
		return "write_timeout"
	case contains(errStr, "connect timeout"):
		return "connect_timeout"
	case contains(errStr, "request timeout"):
		return "request_timeout"
	default:
		return "general_timeout"
	}
}

// encodeConsistencyLevel encodes consistency level strings to numbers for metrics
func encodeConsistencyLevel(level string) int {
	switch level {
	case "ANY":
		return 0
	case "ONE":
		return 1
	case "TWO":
		return 2
	case "THREE":
		return 3
	case "QUORUM":
		return 4
	case "ALL":
		return 5
	case "LOCAL_QUORUM":
		return 6
	case "EACH_QUORUM":
		return 7
	case "LOCAL_ONE":
		return 8
	case "SERIAL":
		return 9
	case "LOCAL_SERIAL":
		return 10
	default:
		return -1
	}
}

// contains is a helper function to check if a string contains a substring (case-insensitive)
func contains(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

// isTimeoutError checks if an error is a timeout error
func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "timeout") || strings.Contains(errStr, "Timeout")
}

// isUnavailableError checks if an error is an unavailable error
func isUnavailableError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "unavailable") || strings.Contains(errStr, "Unavailable")
}
