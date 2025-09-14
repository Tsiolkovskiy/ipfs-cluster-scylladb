// Package examples demonstrates monitoring and observability with ScyllaDB state store
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/ipfs-cluster/ipfs-cluster/api"
	"github.com/ipfs-cluster/ipfs-cluster/state/scyllastate"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// MonitoringExample demonstrates monitoring and metrics collection
func MonitoringExample() {
	fmt.Println("=== Monitoring Example ===")

	// Example 1: Basic metrics collection
	fmt.Println("1. Basic metrics collection...")
	basicMetricsExample()

	// Example 2: Custom metrics
	fmt.Println("2. Custom metrics...")
	customMetricsExample()

	// Example 3: Health monitoring
	fmt.Println("3. Health monitoring...")
	healthMonitoringExample()

	// Example 4: Performance monitoring
	fmt.Println("4. Performance monitoring...")
	performanceMonitoringExample()

	fmt.Println("=== Monitoring Example Complete ===\n")
}

// basicMetricsExample demonstrates basic Prometheus metrics
func basicMetricsExample() {
	// Create configuration with metrics enabled
	config := &scyllastate.Config{
		Hosts:          []string{"127.0.0.1"},
		Port:           9042,
		Keyspace:       "ipfs_pins_monitoring",
		Consistency:    "QUORUM",
		Timeout:        30 * time.Second,
		MetricsEnabled: true, // Enable metrics collection
	}
	config.Default()

	ctx := context.Background()
	state, err := scyllastate.New(ctx, config)
	if err != nil {
		log.Fatal("Failed to create state store:", err)
	}
	defer state.Close()

	// Perform operations to generate metrics
	testPins := createMonitoringTestPins(10)
	
	fmt.Printf("  Performing operations to generate metrics...\n")
	for i, pin := range testPins {
		// Add pin
		err := state.Add(ctx, pin)
		if err != nil {
			log.Printf("Failed to add pin %d: %v", i, err)
			continue
		}

		// Get pin
		_, err = state.Get(ctx, pin.Cid)
		if err != nil {
			log.Printf("Failed to get pin %d: %v", i, err)
		}

		// Check existence
		_, err = state.Has(ctx, pin.Cid)
		if err != nil {
			log.Printf("Failed to check pin %d: %v", i, err)
		}
	}

	// List all pins
	pinChan := make(chan api.Pin, 20)
	go func() {
		defer close(pinChan)
		state.List(ctx, pinChan)
	}()
	
	count := 0
	for range pinChan {
		count++
	}

	fmt.Printf("✓ Generated metrics from %d operations\n", len(testPins)*3+count)
	fmt.Printf("  Metrics are available at /metrics endpoint\n")

	// Cleanup
	for _, pin := range testPins {
		state.Rm(ctx, pin.Cid)
	}
}

// customMetricsExample demonstrates custom application metrics
func customMetricsExample() {
	// Define custom metrics
	var (
		pinOperationDuration = prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name: "ipfs_cluster_pin_operation_duration_seconds",
				Help: "Duration of pin operations",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"operation", "status"},
		)

		pinsByType = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "ipfs_cluster_pins_by_type_total",
				Help: "Total number of pins by type",
			},
			[]string{"pin_type"},
		)

		activeConnections = prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "ipfs_cluster_scylla_connections_active",
				Help: "Number of active ScyllaDB connections",
			},
		)
	)

	// Register custom metrics
	prometheus.MustRegister(pinOperationDuration)
	prometheus.MustRegister(pinsByType)
	prometheus.MustRegister(activeConnections)

	// Create state store
	config := &scyllastate.Config{
		Hosts:          []string{"127.0.0.1"},
		Port:           9042,
		Keyspace:       "ipfs_pins_custom_metrics",
		Consistency:    "QUORUM",
		Timeout:        30 * time.Second,
		MetricsEnabled: true,
	}
	config.Default()

	ctx := context.Background()
	state, err := scyllastate.New(ctx, config)
	if err != nil {
		log.Fatal("Failed to create state store:", err)
	}
	defer state.Close()

	// Simulate operations with custom metrics
	testPins := createDiverseMonitoringPins()
	
	fmt.Printf("  Recording custom metrics for %d operations...\n", len(testPins))
	
	for _, pin := range testPins {
		// Record operation duration
		start := time.Now()
		
		err := state.Add(ctx, pin)
		
		duration := time.Since(start)
		status := "success"
		if err != nil {
			status = "error"
			log.Printf("Failed to add pin: %v", err)
		}
		
		// Record metrics
		pinOperationDuration.WithLabelValues("add", status).Observe(duration.Seconds())
		pinsByType.WithLabelValues(string(pin.Type)).Inc()
	}

	// Update connection gauge
	activeConnections.Set(float64(config.NumConns))

	fmt.Printf("✓ Custom metrics recorded\n")
	fmt.Printf("  - Pin operation durations\n")
	fmt.Printf("  - Pins by type counters\n")
	fmt.Printf("  - Active connections gauge\n")

	// Cleanup
	for _, pin := range testPins {
		state.Rm(ctx, pin.Cid)
	}
}

// healthMonitoringExample demonstrates health check monitoring
func healthMonitoringExample() {
	config := &scyllastate.Config{
		Hosts:       []string{"127.0.0.1"},
		Port:        9042,
		Keyspace:    "ipfs_pins_health",
		Consistency: "QUORUM",
		Timeout:     10 * time.Second,
	}
	config.Default()

	ctx := context.Background()
	state, err := scyllastate.New(ctx, config)
	if err != nil {
		log.Fatal("Failed to create state store:", err)
	}
	defer state.Close()

	// Define health check function
	healthCheck := func() (bool, string, time.Duration) {
		start := time.Now()
		
		// Create test pin for health check
		testPin := createHealthCheckPin()
		
		// Test write operation
		err := state.Add(ctx, testPin)
		if err != nil {
			return false, fmt.Sprintf("Write failed: %v", err), time.Since(start)
		}

		// Test read operation
		_, err = state.Get(ctx, testPin.Cid)
		if err != nil {
			return false, fmt.Sprintf("Read failed: %v", err), time.Since(start)
		}

		// Test delete operation
		err = state.Rm(ctx, testPin.Cid)
		if err != nil {
			return false, fmt.Sprintf("Delete failed: %v", err), time.Since(start)
		}

		return true, "All operations successful", time.Since(start)
	}

	// Perform health checks
	fmt.Printf("  Performing health checks...\n")
	
	for i := 0; i < 5; i++ {
		healthy, message, duration := healthCheck()
		
		status := "✓"
		if !healthy {
			status = "✗"
		}
		
		fmt.Printf("    Health check %d: %s %s (took %v)\n", 
			i+1, status, message, duration)
		
		time.Sleep(1 * time.Second)
	}

	fmt.Printf("✓ Health monitoring example completed\n")
}

// performanceMonitoringExample demonstrates performance metrics collection
func performanceMonitoringExample() {
	config := &scyllastate.Config{
		Hosts:          []string{"127.0.0.1"},
		Port:           9042,
		Keyspace:       "ipfs_pins_performance",
		Consistency:    "QUORUM",
		Timeout:        30 * time.Second,
		MetricsEnabled: true,
	}
	config.Default()

	ctx := context.Background()
	state, err := scyllastate.New(ctx, config)
	if err != nil {
		log.Fatal("Failed to create state store:", err)
	}
	defer state.Close()

	// Performance metrics
	type PerformanceMetrics struct {
		Operation     string
		Count         int
		TotalDuration time.Duration
		MinDuration   time.Duration
		MaxDuration   time.Duration
		Errors        int
	}

	metrics := make(map[string]*PerformanceMetrics)
	
	initMetric := func(operation string) {
		metrics[operation] = &PerformanceMetrics{
			Operation:   operation,
			MinDuration: time.Hour, // Initialize to high value
		}
	}

	recordMetric := func(operation string, duration time.Duration, err error) {
		if metrics[operation] == nil {
			initMetric(operation)
		}
		
		m := metrics[operation]
		m.Count++
		m.TotalDuration += duration
		
		if duration < m.MinDuration {
			m.MinDuration = duration
		}
		if duration > m.MaxDuration {
			m.MaxDuration = duration
		}
		
		if err != nil {
			m.Errors++
		}
	}

	// Generate test data
	testPins := createMonitoringTestPins(100)
	
	fmt.Printf("  Collecting performance metrics for %d operations...\n", len(testPins))

	// Test Add operations
	for _, pin := range testPins {
		start := time.Now()
		err := state.Add(ctx, pin)
		duration := time.Since(start)
		recordMetric("add", duration, err)
	}

	// Test Get operations
	for _, pin := range testPins {
		start := time.Now()
		_, err := state.Get(ctx, pin.Cid)
		duration := time.Since(start)
		recordMetric("get", duration, err)
	}

	// Test Has operations
	for _, pin := range testPins {
		start := time.Now()
		_, err := state.Has(ctx, pin.Cid)
		duration := time.Since(start)
		recordMetric("has", duration, err)
	}

	// Test List operation
	start := time.Now()
	pinChan := make(chan api.Pin, len(testPins)+10)
	go func() {
		defer close(pinChan)
		err := state.List(ctx, pinChan)
		recordMetric("list", time.Since(start), err)
	}()
	
	count := 0
	for range pinChan {
		count++
	}

	// Test Remove operations
	for _, pin := range testPins {
		start := time.Now()
		err := state.Rm(ctx, pin.Cid)
		duration := time.Since(start)
		recordMetric("remove", duration, err)
	}

	// Display performance metrics
	fmt.Printf("✓ Performance metrics collected:\n")
	fmt.Printf("    %-10s %-8s %-12s %-12s %-12s %-8s %-10s\n", 
		"Operation", "Count", "Total", "Avg", "Min", "Max", "Errors")
	fmt.Printf("    %s\n", strings.Repeat("-", 80))
	
	for _, m := range metrics {
		if m.Count == 0 {
			continue
		}
		
		avgDuration := m.TotalDuration / time.Duration(m.Count)
		
		fmt.Printf("    %-10s %-8d %-12v %-12v %-12v %-8v %-10d\n",
			m.Operation,
			m.Count,
			m.TotalDuration.Truncate(time.Millisecond),
			avgDuration.Truncate(time.Millisecond),
			m.MinDuration.Truncate(time.Microsecond),
			m.MaxDuration.Truncate(time.Millisecond),
			m.Errors)
	}
}

// PrometheusServerExample demonstrates setting up Prometheus metrics server
func PrometheusServerExample() {
	fmt.Println("=== Prometheus Server Example ===")

	// Create state store with metrics
	config := &scyllastate.Config{
		Hosts:          []string{"127.0.0.1"},
		Port:           9042,
		Keyspace:       "ipfs_pins_prometheus",
		Consistency:    "QUORUM",
		Timeout:        30 * time.Second,
		MetricsEnabled: true,
	}
	config.Default()

	ctx := context.Background()
	state, err := scyllastate.New(ctx, config)
	if err != nil {
		log.Fatal("Failed to create state store:", err)
	}
	defer state.Close()

	// Set up Prometheus HTTP server
	http.Handle("/metrics", promhttp.Handler())
	
	// Add health check endpoint
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		// Perform health check
		testPin := createHealthCheckPin()
		
		err := state.Add(ctx, testPin)
		if err != nil {
			http.Error(w, fmt.Sprintf("Health check failed: %v", err), http.StatusServiceUnavailable)
			return
		}
		
		_, err = state.Get(ctx, testPin.Cid)
		if err != nil {
			http.Error(w, fmt.Sprintf("Health check failed: %v", err), http.StatusServiceUnavailable)
			return
		}
		
		state.Rm(ctx, testPin.Cid)
		
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Add status endpoint
	http.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		// Get basic status information
		status := map[string]interface{}{
			"service":     "ipfs-cluster-scylladb",
			"version":     "1.0.0",
			"uptime":      time.Since(time.Now()).String(), // Placeholder
			"keyspace":    config.Keyspace,
			"consistency": config.Consistency,
			"hosts":       config.Hosts,
		}
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(status)
	})

	fmt.Printf("Starting Prometheus metrics server on :8080\n")
	fmt.Printf("Endpoints:\n")
	fmt.Printf("  - http://localhost:8080/metrics (Prometheus metrics)\n")
	fmt.Printf("  - http://localhost:8080/health (Health check)\n")
	fmt.Printf("  - http://localhost:8080/status (Status information)\n")

	// Generate some metrics data
	go func() {
		testPins := createMonitoringTestPins(50)
		
		for {
			// Perform operations to generate metrics
			for _, pin := range testPins {
				state.Add(ctx, pin)
				state.Get(ctx, pin.Cid)
				state.Has(ctx, pin.Cid)
			}
			
			// Clean up
			for _, pin := range testPins {
				state.Rm(ctx, pin.Cid)
			}
			
			time.Sleep(30 * time.Second)
		}
	}()

	// Start server (in real application, this would run indefinitely)
	fmt.Printf("✓ Prometheus server example setup complete\n")
	fmt.Printf("  In production, start server with: http.ListenAndServe(\":8080\", nil)\n")

	fmt.Println("=== Prometheus Server Example Complete ===\n")
}

// AlertingExample demonstrates alerting configuration
func AlertingExample() {
	fmt.Println("=== Alerting Example ===")

	fmt.Printf("Example Prometheus alerting rules:\n")
	fmt.Printf(`
# alerts.yml
groups:
  - name: scylladb_state_store
    rules:
      - alert: ScyllaDBStateStoreDown
        expr: up{job="ipfs-cluster-scylladb"} == 0
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "ScyllaDB state store is down"
          description: "ScyllaDB state store has been down for more than 5 minutes"

      - alert: HighErrorRate
        expr: rate(scylla_state_errors_total[5m]) > 0.1
        for: 2m
        labels:
          severity: warning
        annotations:
          summary: "High error rate in ScyllaDB state store"
          description: "Error rate is {{ $value }} errors per second"

      - alert: HighLatency
        expr: histogram_quantile(0.95, rate(scylla_state_operation_duration_seconds_bucket[5m])) > 1.0
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High latency in ScyllaDB operations"
          description: "95th percentile latency is {{ $value }} seconds"

      - alert: LowThroughput
        expr: rate(scylla_state_operations_total[5m]) < 10
        for: 10m
        labels:
          severity: info
        annotations:
          summary: "Low throughput in ScyllaDB state store"
          description: "Throughput is {{ $value }} operations per second"
`)

	fmt.Printf("Example Grafana dashboard queries:\n")
	fmt.Printf(`
# Operations per second
rate(scylla_state_operations_total[5m])

# Average latency
rate(scylla_state_operation_duration_seconds_sum[5m]) / 
rate(scylla_state_operation_duration_seconds_count[5m])

# Error rate
rate(scylla_state_errors_total[5m]) / 
rate(scylla_state_operations_total[5m])

# 95th percentile latency
histogram_quantile(0.95, rate(scylla_state_operation_duration_seconds_bucket[5m]))
`)

	fmt.Println("✓ Alerting configuration examples provided")
	fmt.Println("=== Alerting Example Complete ===\n")
}

// Helper functions

func createMonitoringTestPins(count int) []api.Pin {
	pins := make([]api.Pin, count)
	for i := 0; i < count; i++ {
		cidStr := fmt.Sprintf("QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPb%04x", i)
		cid, _ := api.DecodeCid(cidStr)

		pins[i] = api.Pin{
			Cid:  cid,
			Type: api.DataType,
			Allocations: []api.PeerID{
				api.PeerID("12D3KooWBhMbKvZso7VbJGqJNjKgJkGGN8yKWBGTGaHFdMzqk1mC"),
			},
			ReplicationFactorMin: 1,
			ReplicationFactorMax: 2,
			MaxDepth:             -1,
			PinOptions: api.PinOptions{
				Name: fmt.Sprintf("monitoring-pin-%d", i),
				UserDefined: map[string]string{
					"monitoring": "true",
					"index":      fmt.Sprintf("%d", i),
				},
			},
		}
	}
	return pins
}

func createDiverseMonitoringPins() []api.Pin {
	pinTypes := []api.PinType{api.DataType, api.MetaType, api.ClusterDAGType, api.ShardType}
	pins := make([]api.Pin, 0)

	for i, pinType := range pinTypes {
		for j := 0; j < 3; j++ {
			cidStr := fmt.Sprintf("QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPb%04x", i*10+j)
			cid, _ := api.DecodeCid(cidStr)

			pin := api.Pin{
				Cid:                  cid,
				Type:                 pinType,
				ReplicationFactorMin: 1,
				ReplicationFactorMax: 2,
				MaxDepth:             -1,
				PinOptions: api.PinOptions{
					Name: fmt.Sprintf("diverse-pin-%s-%d", string(pinType), j),
					UserDefined: map[string]string{
						"type": string(pinType),
					},
				},
			}

			pins = append(pins, pin)
		}
	}

	return pins
}

func createHealthCheckPin() api.Pin {
	cidStr := fmt.Sprintf("QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbHC%02x", 
		time.Now().Unix()%256)
	cid, _ := api.DecodeCid(cidStr)

	return api.Pin{
		Cid:  cid,
		Type: api.DataType,
		Allocations: []api.PeerID{
			api.PeerID("12D3KooWBhMbKvZso7VbJGqJNjKgJkGGN8yKWBGTGaHFdMzqk1mC"),
		},
		ReplicationFactorMin: 1,
		ReplicationFactorMax: 1,
		MaxDepth:             -1,
		PinOptions: api.PinOptions{
			Name: "health-check-pin",
			UserDefined: map[string]string{
				"health_check": "true",
				"timestamp":    fmt.Sprintf("%d", time.Now().Unix()),
			},
		},
	}
}

func main() {
	fmt.Println("ScyllaDB State Store - Monitoring Examples")
	fmt.Println("=========================================")

	// Run examples
	MonitoringExample()
	PrometheusServerExample()
	AlertingExample()

	fmt.Println("All monitoring examples completed successfully!")
}