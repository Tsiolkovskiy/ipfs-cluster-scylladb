package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// BenchmarkConfig holds configuration for the benchmark
type BenchmarkConfig struct {
	ClusterAPI   string
	Concurrency  int
	Operations   int
	Scenario     string
	Duration     time.Duration
	OutputFile   string
}

// BenchmarkResult holds the results of a benchmark run
type BenchmarkResult struct {
	Scenario        string            `json:"scenario"`
	Timestamp       time.Time         `json:"timestamp"`
	Duration        time.Duration     `json:"duration"`
	TotalOperations int64             `json:"total_operations"`
	SuccessfulOps   int64             `json:"successful_operations"`
	FailedOps       int64             `json:"failed_operations"`
	OperationsPerSec float64          `json:"operations_per_second"`
	Latencies       LatencyStats      `json:"latencies"`
	ErrorBreakdown  map[string]int64  `json:"error_breakdown"`
	Config          BenchmarkConfig   `json:"config"`
}

// LatencyStats holds latency statistics
type LatencyStats struct {
	Min    time.Duration `json:"min"`
	Max    time.Duration `json:"max"`
	Mean   time.Duration `json:"mean"`
	P50    time.Duration `json:"p50"`
	P95    time.Duration `json:"p95"`
	P99    time.Duration `json:"p99"`
}

// Operation represents a single benchmark operation
type Operation struct {
	Type      string
	CID       string
	StartTime time.Time
	EndTime   time.Time
	Success   bool
	Error     string
}

// PinRequest represents a pin request to IPFS-Cluster
type PinRequest struct {
	CID              string            `json:"cid"`
	Name             string            `json:"name,omitempty"`
	ReplicationMin   int               `json:"replication_factor_min,omitempty"`
	ReplicationMax   int               `json:"replication_factor_max,omitempty"`
	UserAllocations  []string          `json:"user_allocations,omitempty"`
	ExpireAt         *time.Time        `json:"expire_at,omitempty"`
	Metadata         map[string]string `json:"metadata,omitempty"`
}

// ClusterClient wraps HTTP client for IPFS-Cluster API
type ClusterClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewClusterClient creates a new cluster client
func NewClusterClient(baseURL string) *ClusterClient {
	return &ClusterClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Pin adds a pin to the cluster
func (c *ClusterClient) Pin(ctx context.Context, req PinRequest) error {
	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal pin request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v0/pins", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("pin failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// Unpin removes a pin from the cluster
func (c *ClusterClient) Unpin(ctx context.Context, cid string) error {
	httpReq, err := http.NewRequestWithContext(ctx, "DELETE", c.baseURL+"/api/v0/pins/"+cid, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unpin failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// List retrieves all pins from the cluster
func (c *ClusterClient) List(ctx context.Context) error {
	httpReq, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/v0/pins", nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("list failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Read and discard response body
	_, err = io.Copy(io.Discard, resp.Body)
	return err
}

// generateRandomCID generates a random CID for testing
func generateRandomCID() string {
	// Generate a random 32-byte hash
	hash := make([]byte, 32)
	rand.Read(hash)
	
	// Create a simple CIDv1 with SHA256 hash
	return fmt.Sprintf("bafkreig%x", hash[:28]) // Truncate to fit base32 encoding
}

// runSinglePinScenario runs the single pin scenario
func runSinglePinScenario(ctx context.Context, client *ClusterClient, config BenchmarkConfig, results chan<- Operation) {
	for i := 0; i < config.Operations; i++ {
		select {
		case <-ctx.Done():
			return
		default:
		}

		op := Operation{
			Type:      "pin",
			CID:       generateRandomCID(),
			StartTime: time.Now(),
		}

		req := PinRequest{
			CID:            op.CID,
			Name:           fmt.Sprintf("benchmark-pin-%d", i),
			ReplicationMin: 1,
			ReplicationMax: 3,
		}

		err := client.Pin(ctx, req)
		op.EndTime = time.Now()
		op.Success = err == nil
		if err != nil {
			op.Error = err.Error()
		}

		results <- op
	}
}

// runBatchPinScenario runs the batch pin scenario
func runBatchPinScenario(ctx context.Context, client *ClusterClient, config BenchmarkConfig, results chan<- Operation) {
	batchSize := config.Concurrency
	batches := config.Operations / batchSize

	for batch := 0; batch < batches; batch++ {
		var wg sync.WaitGroup
		
		for i := 0; i < batchSize; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				
				select {
				case <-ctx.Done():
					return
				default:
				}

				op := Operation{
					Type:      "pin",
					CID:       generateRandomCID(),
					StartTime: time.Now(),
				}

				req := PinRequest{
					CID:            op.CID,
					Name:           fmt.Sprintf("batch-pin-%d-%d", batch, idx),
					ReplicationMin: 1,
					ReplicationMax: 3,
				}

				err := client.Pin(ctx, req)
				op.EndTime = time.Now()
				op.Success = err == nil
				if err != nil {
					op.Error = err.Error()
				}

				results <- op
			}(i)
		}
		
		wg.Wait()
	}
}

// runConcurrentPinScenario runs the concurrent pin scenario
func runConcurrentPinScenario(ctx context.Context, client *ClusterClient, config BenchmarkConfig, results chan<- Operation) {
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, config.Concurrency)

	for i := 0; i < config.Operations; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			
			select {
			case <-ctx.Done():
				return
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			}

			op := Operation{
				Type:      "pin",
				CID:       generateRandomCID(),
				StartTime: time.Now(),
			}

			req := PinRequest{
				CID:            op.CID,
				Name:           fmt.Sprintf("concurrent-pin-%d", idx),
				ReplicationMin: 1,
				ReplicationMax: 3,
			}

			err := client.Pin(ctx, req)
			op.EndTime = time.Now()
			op.Success = err == nil
			if err != nil {
				op.Error = err.Error()
			}

			results <- op
		}(i)
	}
	
	wg.Wait()
}

// runMixedOperationsScenario runs mixed pin/unpin/list operations
func runMixedOperationsScenario(ctx context.Context, client *ClusterClient, config BenchmarkConfig, results chan<- Operation) {
	pinnedCIDs := make([]string, 0, config.Operations/4)
	var cidsMutex sync.Mutex
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, config.Concurrency)

	for i := 0; i < config.Operations; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			
			select {
			case <-ctx.Done():
				return
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			}

			// Determine operation type (70% pin, 20% unpin, 10% list)
			opType := "pin"
			if rand.Float32() < 0.1 {
				opType = "list"
			} else if rand.Float32() < 0.3 {
				cidsMutex.Lock()
				if len(pinnedCIDs) > 0 {
					opType = "unpin"
				}
				cidsMutex.Unlock()
			}

			op := Operation{
				Type:      opType,
				StartTime: time.Now(),
			}

			var err error
			switch opType {
			case "pin":
				op.CID = generateRandomCID()
				req := PinRequest{
					CID:            op.CID,
					Name:           fmt.Sprintf("mixed-pin-%d", idx),
					ReplicationMin: 1,
					ReplicationMax: 3,
				}
				err = client.Pin(ctx, req)
				if err == nil {
					cidsMutex.Lock()
					pinnedCIDs = append(pinnedCIDs, op.CID)
					cidsMutex.Unlock()
				}

			case "unpin":
				cidsMutex.Lock()
				if len(pinnedCIDs) > 0 {
					// Remove random CID from the list
					cidIdx := rand.Intn(len(pinnedCIDs))
					op.CID = pinnedCIDs[cidIdx]
					pinnedCIDs = append(pinnedCIDs[:cidIdx], pinnedCIDs[cidIdx+1:]...)
				}
				cidsMutex.Unlock()
				
				if op.CID != "" {
					err = client.Unpin(ctx, op.CID)
				}

			case "list":
				err = client.List(ctx)
			}

			op.EndTime = time.Now()
			op.Success = err == nil
			if err != nil {
				op.Error = err.Error()
			}

			results <- op
		}(i)
	}
	
	wg.Wait()
}

// calculateLatencyStats calculates latency statistics from operations
func calculateLatencyStats(operations []Operation) LatencyStats {
	if len(operations) == 0 {
		return LatencyStats{}
	}

	latencies := make([]time.Duration, 0, len(operations))
	var totalLatency time.Duration

	for _, op := range operations {
		if op.Success {
			latency := op.EndTime.Sub(op.StartTime)
			latencies = append(latencies, latency)
			totalLatency += latency
		}
	}

	if len(latencies) == 0 {
		return LatencyStats{}
	}

	// Sort latencies for percentile calculations
	for i := 0; i < len(latencies); i++ {
		for j := i + 1; j < len(latencies); j++ {
			if latencies[i] > latencies[j] {
				latencies[i], latencies[j] = latencies[j], latencies[i]
			}
		}
	}

	stats := LatencyStats{
		Min:  latencies[0],
		Max:  latencies[len(latencies)-1],
		Mean: totalLatency / time.Duration(len(latencies)),
	}

	// Calculate percentiles
	if len(latencies) > 0 {
		stats.P50 = latencies[len(latencies)*50/100]
		stats.P95 = latencies[len(latencies)*95/100]
		stats.P99 = latencies[len(latencies)*99/100]
	}

	return stats
}

// runBenchmark executes the benchmark based on the scenario
func runBenchmark(config BenchmarkConfig) (*BenchmarkResult, error) {
	client := NewClusterClient(config.ClusterAPI)
	
	ctx, cancel := context.WithTimeout(context.Background(), config.Duration)
	defer cancel()

	results := make(chan Operation, config.Operations)
	startTime := time.Now()

	// Run the appropriate scenario
	switch config.Scenario {
	case "single_pin":
		go runSinglePinScenario(ctx, client, config, results)
	case "batch_pin":
		go runBatchPinScenario(ctx, client, config, results)
	case "concurrent_pin":
		go runConcurrentPinScenario(ctx, client, config, results)
	case "mixed_operations":
		go runMixedOperationsScenario(ctx, client, config, results)
	default:
		return nil, fmt.Errorf("unknown scenario: %s", config.Scenario)
	}

	// Collect results
	var operations []Operation
	var successfulOps, failedOps int64
	errorBreakdown := make(map[string]int64)

	// Wait for all operations to complete or timeout
	go func() {
		time.Sleep(config.Duration)
		cancel()
	}()

	for {
		select {
		case op := <-results:
			operations = append(operations, op)
			if op.Success {
				atomic.AddInt64(&successfulOps, 1)
			} else {
				atomic.AddInt64(&failedOps, 1)
				errorBreakdown[op.Error]++
			}
			
			if len(operations) >= config.Operations {
				goto done
			}
		case <-ctx.Done():
			goto done
		}
	}

done:
	endTime := time.Now()
	duration := endTime.Sub(startTime)

	// Calculate statistics
	latencies := calculateLatencyStats(operations)
	opsPerSec := float64(len(operations)) / duration.Seconds()

	result := &BenchmarkResult{
		Scenario:        config.Scenario,
		Timestamp:       startTime,
		Duration:        duration,
		TotalOperations: int64(len(operations)),
		SuccessfulOps:   successfulOps,
		FailedOps:       failedOps,
		OperationsPerSec: opsPerSec,
		Latencies:       latencies,
		ErrorBreakdown:  errorBreakdown,
		Config:          config,
	}

	return result, nil
}

func main() {
	var config BenchmarkConfig

	flag.StringVar(&config.ClusterAPI, "cluster-api", "http://localhost:9094", "IPFS-Cluster API endpoint")
	flag.IntVar(&config.Concurrency, "concurrency", 10, "Number of concurrent operations")
	flag.IntVar(&config.Operations, "operations", 1000, "Total number of operations")
	flag.StringVar(&config.Scenario, "scenario", "single_pin", "Benchmark scenario (single_pin, batch_pin, concurrent_pin, mixed_operations)")
	flag.DurationVar(&config.Duration, "duration", 5*time.Minute, "Maximum benchmark duration")
	flag.StringVar(&config.OutputFile, "output", "", "Output file for results (JSON format)")
	flag.Parse()

	log.Printf("Starting benchmark: %s", config.Scenario)
	log.Printf("Configuration: %+v", config)

	result, err := runBenchmark(config)
	if err != nil {
		log.Fatalf("Benchmark failed: %v", err)
	}

	// Print results to stdout
	fmt.Printf("\n=== Benchmark Results ===\n")
	fmt.Printf("Scenario: %s\n", result.Scenario)
	fmt.Printf("Duration: %v\n", result.Duration)
	fmt.Printf("Total Operations: %d\n", result.TotalOperations)
	fmt.Printf("Successful Operations: %d\n", result.SuccessfulOps)
	fmt.Printf("Failed Operations: %d\n", result.FailedOps)
	fmt.Printf("Operations/sec: %.2f\n", result.OperationsPerSec)
	fmt.Printf("Latency Stats:\n")
	fmt.Printf("  Min: %v\n", result.Latencies.Min)
	fmt.Printf("  Max: %v\n", result.Latencies.Max)
	fmt.Printf("  Mean: %v\n", result.Latencies.Mean)
	fmt.Printf("  P50: %v\n", result.Latencies.P50)
	fmt.Printf("  P95: %v\n", result.Latencies.P95)
	fmt.Printf("  P99: %v\n", result.Latencies.P99)

	if len(result.ErrorBreakdown) > 0 {
		fmt.Printf("Error Breakdown:\n")
		for err, count := range result.ErrorBreakdown {
			fmt.Printf("  %s: %d\n", err, count)
		}
	}

	// Save results to file if specified
	if config.OutputFile != "" {
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			log.Fatalf("Failed to marshal results: %v", err)
		}

		err = os.WriteFile(config.OutputFile, data, 0644)
		if err != nil {
			log.Fatalf("Failed to write results file: %v", err)
		}

		log.Printf("Results saved to: %s", config.OutputFile)
	}
}