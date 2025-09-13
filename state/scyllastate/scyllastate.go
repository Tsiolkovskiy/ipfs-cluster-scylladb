// Package scyllastate implements IPFS-Cluster state interface using ScyllaDB
package scyllastate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gocql/gocql"
	"github.com/ipfs-cluster/ipfs-cluster/api"
	"github.com/ipfs-cluster/ipfs-cluster/state"
)

// ScyllaState implements the state.State interface using ScyllaDB as the backend
type ScyllaState struct {
	session          *gocql.Session
	config           *Config
	logger           *log.Logger
	structuredLogger *StructuredLogger
	metrics          *Metrics
	prepared         *PreparedStatements

	// Prepared statement caching for dynamic queries
	preparedCache *PreparedStatementCache

	// Graceful degradation manager for handling node failures and network partitions
	degradationManager *GracefulDegradationManager

	// State tracking
	totalPins int64
	closed    int32 // atomic flag for closed state

	// Synchronization
	mu sync.RWMutex
}

// ScyllaBatchingState implements state.BatchingState for batched operations
type ScyllaBatchingState struct {
	*ScyllaState
	batch *gocql.Batch
	mu    sync.Mutex
}

// PreparedStatements holds all prepared CQL statements for optimal performance
type PreparedStatements struct {
	// Pin operations
	insertPin   *gocql.Query
	selectPin   *gocql.Query
	deletePin   *gocql.Query
	checkExists *gocql.Query
	listAllPins *gocql.Query

	// Placement operations
	insertPlacement *gocql.Query
	selectPlacement *gocql.Query
	deletePlacement *gocql.Query

	// Peer pin operations
	insertPeerPin *gocql.Query
	selectPeerPin *gocql.Query
	deletePeerPin *gocql.Query
	listPeerPins  *gocql.Query

	// TTL operations
	insertTTL *gocql.Query
	selectTTL *gocql.Query
	deleteTTL *gocql.Query

	// Statistics
	countPins *gocql.Query
}

// PreparedStatementCache provides caching for dynamically prepared statements
type PreparedStatementCache struct {
	cache   map[string]*gocql.Query
	maxSize int
	session *gocql.Session
	mu      sync.RWMutex
}

// NewPreparedStatementCache creates a new prepared statement cache
func NewPreparedStatementCache(session *gocql.Session, maxSize int) *PreparedStatementCache {
	return &PreparedStatementCache{
		cache:   make(map[string]*gocql.Query),
		maxSize: maxSize,
		session: session,
	}
}

// Get retrieves or creates a prepared statement
func (psc *PreparedStatementCache) Get(cql string) (*gocql.Query, error) {
	// Try to get from cache first (read lock)
	psc.mu.RLock()
	if query, exists := psc.cache[cql]; exists {
		psc.mu.RUnlock()
		return query, nil
	}
	psc.mu.RUnlock()

	// Not in cache, prepare statement (write lock)
	psc.mu.Lock()
	defer psc.mu.Unlock()

	// Double-check in case another goroutine prepared it
	if query, exists := psc.cache[cql]; exists {
		return query, nil
	}

	// Check cache size limit
	if len(psc.cache) >= psc.maxSize {
		// Simple eviction: clear cache when full
		// In production, could implement LRU eviction
		psc.cache = make(map[string]*gocql.Query)
	}

	// Prepare the statement
	query := psc.session.Query(cql)
	psc.cache[cql] = query

	return query, nil
}

// Clear clears all cached prepared statements
func (psc *PreparedStatementCache) Clear() {
	psc.mu.Lock()
	defer psc.mu.Unlock()
	psc.cache = make(map[string]*gocql.Query)
}

// Size returns the current number of cached prepared statements
func (psc *PreparedStatementCache) Size() int {
	psc.mu.RLock()
	defer psc.mu.RUnlock()
	return len(psc.cache)
}

// Metrics interface for monitoring ScyllaDB operations
type Metrics interface {
	RecordOperation(operation string, duration time.Duration, consistencyLevel string, err error)
	RecordQuery(queryType, keyspace string, duration time.Duration)
	RecordRetry(operation, reason string)
	RecordBatchOperation(batchType string, size int, err error)
	RecordPinOperation(operationType string)
	UpdateConnectionMetrics(activeConns int, poolStatus map[string]bool)
	UpdateStateMetrics(totalPins int64)
	UpdatePreparedStatements(count int)
	UpdateConsistencyLevel(operationType string, level string)
	UpdateDegradationMetrics(active bool, level int, degradationType string)
	UpdateNodeHealth(host, datacenter, rack string, healthy bool)
	UpdatePartitionStatus(partitioned bool)
	RecordConnectionError()
}

// New creates a new ScyllaState instance
func New(ctx context.Context, config *Config) (*ScyllaState, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	// Create logger
	logger := log.New(log.Writer(), "[ScyllaState] ", log.LstdFlags)

	// Create structured logger for detailed operation logging
	structuredLogger := NewStructuredLogger(config)

	// Create metrics if enabled
	var metrics Metrics
	if config.MetricsEnabled {
		metrics = NewPrometheusMetrics()
	}

	// Create ScyllaState instance
	s := &ScyllaState{
		config:           config,
		logger:           logger,
		structuredLogger: structuredLogger,
		metrics:          metrics,
	}

	// Initialize connection to ScyllaDB
	if err := s.connect(ctx); err != nil {
		return nil, fmt.Errorf("failed to connect to ScyllaDB: %w", err)
	}

	// Initialize prepared statement cache
	s.preparedCache = NewPreparedStatementCache(s.session, config.PreparedStatementCacheSize)

	// Initialize graceful degradation manager
	retryPolicy := NewRetryPolicy(config.RetryPolicy)
	s.degradationManager = NewGracefulDegradationManager(s.session, config, logger, retryPolicy, metrics)

	// Start graceful degradation monitoring
	if err := s.degradationManager.Start(ctx); err != nil {
		s.Close()
		return nil, fmt.Errorf("failed to start graceful degradation manager: %w", err)
	}

	// Prepare statements
	if err := s.prepareStatements(); err != nil {
		s.Close()
		return nil, fmt.Errorf("failed to prepare statements: %w", err)
	}

	// Initialize pin count
	if err := s.updatePinCount(ctx); err != nil {
		s.logger.Printf("Failed to initialize pin count: %v", err)
	}

	s.logger.Printf("ScyllaState initialized successfully: keyspace=%s, hosts=%v, total_pins=%d",
		config.Keyspace, config.Hosts, s.totalPins)

	return s, nil
}

// connect establishes connection to ScyllaDB cluster with optimized settings
func (s *ScyllaState) connect(ctx context.Context) error {
	// Create optimized cluster configuration
	cluster, err := s.config.CreateClusterConfig()
	if err != nil {
		return fmt.Errorf("failed to create cluster config: %w", err)
	}

	// Set up retry policy for resilience
	cluster.RetryPolicy = &RetryPolicy{
		numRetries:    s.config.RetryPolicy.NumRetries,
		minDelay:      s.config.RetryPolicy.MinRetryDelay,
		maxDelay:      s.config.RetryPolicy.MaxRetryDelay,
		backoffFactor: 2.0,
	}

	// Enable tracing if configured for debugging
	if s.config.TracingEnabled {
		cluster.QueryObserver = NewQueryObserver(s.structuredLogger)
		cluster.BatchObserver = NewBatchObserver(s.structuredLogger)
		cluster.ConnectObserver = NewConnectObserver(s.structuredLogger)
	}

	// Configure connection event handlers for monitoring
	s.configureConnectionEvents(cluster)

	// Create session with context timeout
	connectCtx, cancel := context.WithTimeout(ctx, s.config.ConnectTimeout)
	defer cancel()

	session, err := cluster.CreateSession()
	if err != nil {
		return fmt.Errorf("failed to create ScyllaDB session: %w", err)
	}

	// Test connection with optimized query
	if err := s.testConnection(connectCtx, session); err != nil {
		session.Close()
		return fmt.Errorf("connection test failed: %w", err)
	}

	s.session = session

	// Update connection metrics
	if s.metrics != nil {
		s.metrics.UpdateConnectionMetrics(s.config.NumConns, make(map[string]bool))
	}

	// Log connection details including optimization settings
	s.logConnectionDetails()

	return nil
}

// configureConnectionEvents sets up connection event handlers for monitoring
func (s *ScyllaState) configureConnectionEvents(cluster *gocql.ClusterConfig) {
	// Configure connection event callbacks for monitoring and debugging
	// Note: ConnectObserverFunc may not be available in all gocql versions
	// This is a placeholder for connection monitoring that can be implemented
	// when the appropriate gocql version is available

	// Configure host state change observer
	cluster.HostFilter = gocql.HostFilterFunc(func(host *gocql.HostInfo) bool {
		// Log host state changes for debugging
		s.logger.Printf("Host state change: host=%s, state=%s, datacenter=%s",
			host.ConnectAddress(), host.State(), host.DataCenter())
		return true // Accept all hosts
	})
}

// logConnectionDetails logs detailed connection information
func (s *ScyllaState) logConnectionDetails() {
	s.logger.Printf("ScyllaDB connection established with optimizations:")
	s.logger.Printf("  - Keyspace: %s", s.config.Keyspace)
	s.logger.Printf("  - Hosts: %v", s.config.Hosts)
	s.logger.Printf("  - Connections per host: %d", s.config.NumConns)
	s.logger.Printf("  - Token-aware routing: %t", s.config.TokenAwareRouting)
	s.logger.Printf("  - DC-aware routing: %t (local DC: %s)", s.config.DCAwareRouting, s.config.LocalDC)
	s.logger.Printf("  - Prepared statement cache size: %d", s.config.PreparedStatementCacheSize)
	s.logger.Printf("  - Page size: %d", s.config.PageSize)
	s.logger.Printf("  - Connection timeout: %v", s.config.ConnectTimeout)
	s.logger.Printf("  - Query timeout: %v", s.config.Timeout)
	s.logger.Printf("  - Total pins: %d", s.totalPins)
}

// testConnection verifies the ScyllaDB connection is working
func (s *ScyllaState) testConnection(ctx context.Context, session *gocql.Session) error {
	query := session.Query("SELECT now() FROM system.local").WithContext(ctx)
	defer query.Release()

	var now time.Time
	if err := query.Scan(&now); err != nil {
		return fmt.Errorf("connection test query failed: %w", err)
	}

	s.logger.Printf("Connection test successful: server_time=%v", now)
	return nil
}

// prepareStatements prepares all CQL statements for optimal performance
func (s *ScyllaState) prepareStatements() error {
	s.prepared = &PreparedStatements{}

	// Pin operations with optimized queries
	insertPinCQL := `INSERT INTO pins_by_cid (mh_prefix, cid_bin, pin_type, rf, owner, tags, ttl, metadata, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	s.prepared.insertPin = s.createOptimizedQuery(insertPinCQL)

	selectPinCQL := `SELECT pin_type, rf, owner, tags, ttl, metadata, created_at, updated_at
		 FROM pins_by_cid WHERE mh_prefix = ? AND cid_bin = ?`
	s.prepared.selectPin = s.createOptimizedQuery(selectPinCQL)

	deletePinCQL := `DELETE FROM pins_by_cid WHERE mh_prefix = ? AND cid_bin = ?`
	s.prepared.deletePin = s.createOptimizedQuery(deletePinCQL)

	checkExistsCQL := `SELECT cid_bin FROM pins_by_cid WHERE mh_prefix = ? AND cid_bin = ?`
	s.prepared.checkExists = s.createOptimizedQuery(checkExistsCQL)

	listAllPinsCQL := `SELECT mh_prefix, cid_bin, pin_type, rf, owner, tags, ttl, metadata, created_at, updated_at
		 FROM pins_by_cid`
	s.prepared.listAllPins = s.createOptimizedQuery(listAllPinsCQL)

	// Placement operations
	insertPlacementCQL := `INSERT INTO placements_by_cid (mh_prefix, cid_bin, desired, actual, updated_at)
		 VALUES (?, ?, ?, ?, ?)`
	s.prepared.insertPlacement = s.createOptimizedQuery(insertPlacementCQL)

	selectPlacementCQL := `SELECT desired, actual, updated_at
		 FROM placements_by_cid WHERE mh_prefix = ? AND cid_bin = ?`
	s.prepared.selectPlacement = s.createOptimizedQuery(selectPlacementCQL)

	deletePlacementCQL := `DELETE FROM placements_by_cid WHERE mh_prefix = ? AND cid_bin = ?`
	s.prepared.deletePlacement = s.createOptimizedQuery(deletePlacementCQL)

	// Peer pin operations
	insertPeerPinCQL := `INSERT INTO pins_by_peer (peer_id, mh_prefix, cid_bin, state, last_seen)
		 VALUES (?, ?, ?, ?, ?)`
	s.prepared.insertPeerPin = s.createOptimizedQuery(insertPeerPinCQL)

	selectPeerPinCQL := `SELECT state, last_seen FROM pins_by_peer
		 WHERE peer_id = ? AND mh_prefix = ? AND cid_bin = ?`
	s.prepared.selectPeerPin = s.createOptimizedQuery(selectPeerPinCQL)

	deletePeerPinCQL := `DELETE FROM pins_by_peer WHERE peer_id = ? AND mh_prefix = ? AND cid_bin = ?`
	s.prepared.deletePeerPin = s.createOptimizedQuery(deletePeerPinCQL)

	listPeerPinsCQL := `SELECT mh_prefix, cid_bin, state, last_seen FROM pins_by_peer
		 WHERE peer_id = ? LIMIT ?`
	s.prepared.listPeerPins = s.createOptimizedQuery(listPeerPinsCQL)

	// TTL operations
	insertTTLCQL := `INSERT INTO pin_ttl_queue (ttl_bucket, cid_bin, owner, ttl)
		 VALUES (?, ?, ?, ?)`
	s.prepared.insertTTL = s.createOptimizedQuery(insertTTLCQL)

	selectTTLCQL := `SELECT cid_bin, owner, ttl FROM pin_ttl_queue
		 WHERE ttl_bucket = ? LIMIT ?`
	s.prepared.selectTTL = s.createOptimizedQuery(selectTTLCQL)

	deleteTTLCQL := `DELETE FROM pin_ttl_queue WHERE ttl_bucket = ? AND ttl = ? AND cid_bin = ?`
	s.prepared.deleteTTL = s.createOptimizedQuery(deleteTTLCQL)

	// Statistics
	countPinsCQL := `SELECT COUNT(*) FROM pins_by_cid`
	s.prepared.countPins = s.createOptimizedQuery(countPinsCQL)

	s.logger.Printf("All CQL statements prepared successfully with optimizations")
	return nil
}

// createOptimizedQuery creates a query with performance optimizations applied
func (s *ScyllaState) createOptimizedQuery(cql string) *gocql.Query {
	query := s.session.Query(cql)

	// Apply performance optimizations
	query.PageSize(s.config.PageSize)

	// Set consistency levels for optimal performance vs consistency trade-off
	query.Consistency(s.config.GetConsistency())
	query.SerialConsistency(s.config.GetSerialConsistency())

	// Enable idempotent flag for safe retries where applicable
	// This allows the driver to retry queries safely
	if s.isIdempotentQuery(cql) {
		query.Idempotent(true)
	}

	return query
}

// isIdempotentQuery determines if a query is safe to retry
func (s *ScyllaState) isIdempotentQuery(cql string) bool {
	cqlUpper := strings.ToUpper(strings.TrimSpace(cql))

	// SELECT queries are always idempotent
	if strings.HasPrefix(cqlUpper, "SELECT") {
		return true
	}

	// DELETE queries with WHERE clauses are idempotent
	if strings.HasPrefix(cqlUpper, "DELETE") && strings.Contains(cqlUpper, "WHERE") {
		return true
	}

	// INSERT queries with IF NOT EXISTS are idempotent
	if strings.HasPrefix(cqlUpper, "INSERT") && strings.Contains(cqlUpper, "IF NOT EXISTS") {
		return true
	}

	// UPDATE queries with WHERE clauses are generally idempotent for our use case
	if strings.HasPrefix(cqlUpper, "UPDATE") && strings.Contains(cqlUpper, "WHERE") {
		return true
	}

	// Conservative approach: assume non-idempotent for other cases
	return false
}

// GetPreparedQuery retrieves a prepared query from cache or creates it
func (s *ScyllaState) GetPreparedQuery(cql string) (*gocql.Query, error) {
	if s.preparedCache == nil {
		return nil, fmt.Errorf("prepared statement cache not initialized")
	}

	query, err := s.preparedCache.Get(cql)
	if err != nil {
		return nil, fmt.Errorf("failed to get prepared query: %w", err)
	}

	// Apply optimizations to cached query
	return s.applyQueryOptimizations(query), nil
}

// applyQueryOptimizations applies performance optimizations to a query
func (s *ScyllaState) applyQueryOptimizations(query *gocql.Query) *gocql.Query {
	// Apply page size optimization
	query.PageSize(s.config.PageSize)

	// Apply consistency settings
	query.Consistency(s.config.GetConsistency())
	query.SerialConsistency(s.config.GetSerialConsistency())

	return query
}

// updatePinCount updates the internal pin count from the database
func (s *ScyllaState) updatePinCount(ctx context.Context) error {
	query := s.prepared.countPins.WithContext(ctx)
	defer query.Release()

	var count int64
	if err := query.Scan(&count); err != nil {
		return fmt.Errorf("failed to count pins: %w", err)
	}

	atomic.StoreInt64(&s.totalPins, count)

	if s.metrics != nil {
		s.metrics.UpdateStateMetrics(count)
	}

	return nil
}

// Close closes the ScyllaDB session and cleans up resources
func (s *ScyllaState) Close() error {
	if !atomic.CompareAndSwapInt32(&s.closed, 0, 1) {
		return nil // Already closed
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Stop graceful degradation manager
	if s.degradationManager != nil {
		s.degradationManager.Stop()
		s.degradationManager = nil
	}

	// Clear prepared statement cache
	if s.preparedCache != nil {
		s.preparedCache.Clear()
		s.preparedCache = nil
	}

	// Close session
	if s.session != nil {
		s.session.Close()
		s.session = nil
	}

	// Reset metrics
	if s.metrics != nil {
		s.metrics.UpdateConnectionMetrics(0, make(map[string]bool))
	}

	s.logger.Printf("ScyllaState closed with all resources cleaned up")
	return nil
}

// isClosed returns true if the state has been closed
func (s *ScyllaState) isClosed() bool {
	return atomic.LoadInt32(&s.closed) == 1
}

// recordOperation records metrics for an operation
func (s *ScyllaState) recordOperation(operation string, duration time.Duration, err error) {
	if s.metrics == nil {
		return
	}

	consistencyLevel := s.config.Consistency
	s.metrics.RecordOperation(operation, duration, consistencyLevel, err)
}

// logOperation logs an operation with context using structured logging
func (s *ScyllaState) logOperation(ctx context.Context, operation string, cid api.Cid, duration time.Duration, err error) {
	opCtx := NewOperationContext(operation).
		WithDuration(duration).
		WithConsistency(s.config.Consistency)

	if cid != nil {
		opCtx = opCtx.WithCID(cid)
	}

	if err != nil {
		opCtx = opCtx.WithError(err)
	}

	s.structuredLogger.LogOperation(ctx, opCtx)
}

// recordQuery records metrics for individual CQL queries
func (s *ScyllaState) recordQuery(queryType string, duration time.Duration) {
	if s.metrics == nil {
		return
	}
	s.metrics.RecordQuery(queryType, s.config.Keyspace, duration)
}

// recordRetry records a retry attempt
func (s *ScyllaState) recordRetry(operation, reason string) {
	if s.metrics == nil {
		return
	}
	s.metrics.RecordRetry(operation, reason)
}

// recordBatchOperation records batch operation metrics
func (s *ScyllaState) recordBatchOperation(batchType string, size int, err error) {
	if s.metrics == nil {
		return
	}
	s.metrics.RecordBatchOperation(batchType, size, err)
}

// recordPinOperation records pin-specific operation metrics
func (s *ScyllaState) recordPinOperation(operationType string) {
	if s.metrics == nil {
		return
	}
	s.metrics.RecordPinOperation(operationType)
}

// updatePreparedStatementsMetric updates prepared statement cache metrics
func (s *ScyllaState) updatePreparedStatementsMetric() {
	if s.metrics == nil || s.preparedCache == nil {
		return
	}
	// Get cache size from prepared cache
	cacheSize := s.preparedCache.Size()
	s.metrics.UpdatePreparedStatements(cacheSize)
}

// recordConnectionError records connection errors
func (s *ScyllaState) recordConnectionError() {
	if s.metrics == nil {
		return
	}
	s.metrics.RecordConnectionError()
}

// isTimeoutError checks if an error is a timeout error
func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	// Check for gocql timeout errors by string matching
	// This is a workaround since gocql.IsTimeoutError may not be available
	errStr := err.Error()
	return strings.Contains(errStr, "timeout") || strings.Contains(errStr, "Timeout")
}

// isUnavailableError checks if an error is an unavailable error
func isUnavailableError(err error) bool {
	if err == nil {
		return false
	}
	// Check for gocql unavailable errors by string matching
	errStr := err.Error()
	return strings.Contains(errStr, "unavailable") || strings.Contains(errStr, "Unavailable")
}

// executeWithGracefulDegradation executes a query with graceful degradation support
// Implements requirements 3.1, 3.3, and 3.4 for handling node failures and network partitions
func (s *ScyllaState) executeWithGracefulDegradation(ctx context.Context, queryFunc func(consistency gocql.Consistency) error) error {
	if s.degradationManager == nil {
		// Fallback to direct execution if degradation manager is not available
		return queryFunc(s.config.GetConsistency())
	}

	return s.degradationManager.ExecuteWithDegradation(ctx, queryFunc)
}

// executeQueryWithDegradation executes a single query with graceful degradation
func (s *ScyllaState) executeQueryWithDegradation(ctx context.Context, query *gocql.Query) error {
	return s.executeWithGracefulDegradation(ctx, func(consistency gocql.Consistency) error {
		query.Consistency(consistency)
		return query.WithContext(ctx).Exec()
	})
}

// scanQueryWithDegradation executes a scan query with graceful degradation
func (s *ScyllaState) scanQueryWithDegradation(ctx context.Context, query *gocql.Query, dest ...interface{}) error {
	return s.executeWithGracefulDegradation(ctx, func(consistency gocql.Consistency) error {
		query.Consistency(consistency)
		return query.WithContext(ctx).Scan(dest...)
	})
}

// iterQueryWithDegradation executes an iter query with graceful degradation
func (s *ScyllaState) iterQueryWithDegradation(ctx context.Context, query *gocql.Query, iterFunc func(*gocql.Iter) error) error {
	return s.executeWithGracefulDegradation(ctx, func(consistency gocql.Consistency) error {
		query.Consistency(consistency)
		iter := query.WithContext(ctx).Iter()
		defer iter.Close()

		err := iterFunc(iter)
		if err != nil {
			return err
		}

		return iter.Close()
	})
}

// GetDegradationMetrics returns current graceful degradation metrics
func (s *ScyllaState) GetDegradationMetrics() *DegradationMetrics {
	if s.degradationManager == nil {
		return &DegradationMetrics{}
	}
	return s.degradationManager.GetMetrics()
}

// GetNodeHealth returns the health status of all ScyllaDB nodes
func (s *ScyllaState) GetNodeHealth() map[string]*NodeHealthStatus {
	if s.degradationManager == nil {
		return make(map[string]*NodeHealthStatus)
	}
	return s.degradationManager.GetNodeHealth()
}

// IsPartitioned returns true if a network partition is currently detected
func (s *ScyllaState) IsPartitioned() bool {
	if s.degradationManager == nil {
		return false
	}
	return s.degradationManager.IsPartitioned()
}

// GetCurrentConsistencyLevel returns the current (possibly degraded) consistency level
func (s *ScyllaState) GetCurrentConsistencyLevel() gocql.Consistency {
	if s.degradationManager == nil {
		return s.config.GetConsistency()
	}
	return s.degradationManager.GetCurrentConsistencyLevel()
}

// Pin serialization version constants for versioning support
const (
	// PinSerializationVersion1 is the initial version using api.Pin.ProtoMarshal()
	PinSerializationVersion1 = 1
	// CurrentPinSerializationVersion is the current version being used
	CurrentPinSerializationVersion = PinSerializationVersion1
)

// SerializedPin represents a versioned serialized pin for storage in ScyllaDB
type SerializedPin struct {
	Version uint8  `json:"version"`
	Data    []byte `json:"data"`
}

// serializePin serializes an api.Pin using the current protobuf format for compatibility
// This implements requirement 4.2 for pin serialization compatibility
func (s *ScyllaState) serializePin(pin api.Pin) ([]byte, error) {
	// Use existing api.Pin.ProtoMarshal() for compatibility with other state stores
	pinData, err := pin.ProtoMarshal()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal pin with protobuf: %w", err)
	}

	// Wrap with version information for future compatibility
	serializedPin := SerializedPin{
		Version: CurrentPinSerializationVersion,
		Data:    pinData,
	}

	// Serialize the versioned wrapper
	versionedData, err := json.Marshal(serializedPin)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal versioned pin data: %w", err)
	}

	return versionedData, nil
}

// deserializePin deserializes a pin from its stored format, handling version compatibility
// This implements requirement 4.2 for pin deserialization with version support
func (s *ScyllaState) deserializePin(cid api.Cid, data []byte) (api.Pin, error) {
	if len(data) == 0 {
		return api.Pin{}, fmt.Errorf("empty pin data for CID %s", cid.String())
	}

	// Try to deserialize as versioned data first
	var serializedPin SerializedPin
	if err := json.Unmarshal(data, &serializedPin); err != nil {
		// Fallback: assume this is raw protobuf data from an older version
		return s.deserializePinV1(cid, data)
	}

	// Handle different versions
	switch serializedPin.Version {
	case PinSerializationVersion1:
		return s.deserializePinV1(cid, serializedPin.Data)
	default:
		return api.Pin{}, fmt.Errorf("unsupported pin serialization version %d for CID %s",
			serializedPin.Version, cid.String())
	}
}

// deserializePinV1 deserializes a pin using version 1 format (protobuf)
func (s *ScyllaState) deserializePinV1(cid api.Cid, data []byte) (api.Pin, error) {
	var pin api.Pin

	// Use existing api.Pin.ProtoUnmarshal() for compatibility
	if err := pin.ProtoUnmarshal(data); err != nil {
		return api.Pin{}, fmt.Errorf("failed to unmarshal pin protobuf data for CID %s: %w",
			cid.String(), err)
	}

	// Ensure the CID is set correctly (protobuf doesn't include it in some cases)
	if !pin.Cid.Defined() || !pin.Cid.Equals(cid) {
		pin.Cid = cid
	}

	return pin, nil
}

// validateSerializedPin validates that a serialized pin can be properly deserialized
func (s *ScyllaState) validateSerializedPin(cid api.Cid, data []byte) error {
	_, err := s.deserializePin(cid, data)
	return err
}

// migrateSerializedPin migrates a pin from an older serialization format to the current format
// This can be used during database migrations or when encountering old format data
func (s *ScyllaState) migrateSerializedPin(cid api.Cid, oldData []byte) ([]byte, error) {
	// Deserialize using the old format
	pin, err := s.deserializePin(cid, oldData)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize pin for migration: %w", err)
	}

	// Re-serialize using the current format
	newData, err := s.serializePin(pin)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize pin with new format: %w", err)
	}

	return newData, nil
}

// Ensure ScyllaState implements the required interfaces
var (
	_ state.State         = (*ScyllaState)(nil)
	_ state.ReadOnly      = (*ScyllaState)(nil)
	_ state.WriteOnly     = (*ScyllaState)(nil)
	_ state.BatchingState = (*ScyllaBatchingState)(nil)
	_ io.Closer           = (*ScyllaState)(nil)
)
