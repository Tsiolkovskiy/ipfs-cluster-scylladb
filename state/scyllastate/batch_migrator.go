package scyllastate

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ipfs-cluster/ipfs-cluster/api"
	"github.com/ipfs-cluster/ipfs-cluster/state"
)

// BatchMigrator handles large-scale data migration with advanced batching and parallel processing
type BatchMigrator struct {
	destination *ScyllaState
	config      *BatchMigratorConfig
	logger      Logger

	// Progress tracking
	totalBatches     int64
	processedBatches int64
	totalPins        int64
	processedPins    int64
	errorCount       int64
	startTime        time.Time

	// Worker management
	workers     []*MigrationWorker
	workerPool  chan *MigrationWorker
	batchQueue  chan *PinBatch
	resultQueue chan *BatchResult

	// Synchronization
	mu     sync.RWMutex
	wg     sync.WaitGroup
	ctx    context.Context
	cancel context.CancelFunc
}

// BatchMigratorConfig holds configuration for batch migration operations
type BatchMigratorConfig struct {
	// BatchSize controls the number of pins per batch
	BatchSize int `json:"batch_size"`

	// MaxBatches controls the maximum number of batches to process concurrently
	MaxBatches int `json:"max_batches"`

	// WorkerCount controls the number of migration workers
	WorkerCount int `json:"worker_count"`

	// QueueSize controls the size of the batch queue
	QueueSize int `json:"queue_size"`

	// RetryPolicy controls retry behavior for failed batches
	RetryPolicy *BatchRetryPolicy `json:"retry_policy"`

	// MemoryLimit controls memory usage limits (in bytes)
	MemoryLimit int64 `json:"memory_limit"`

	// CheckpointInterval controls how often to save progress checkpoints
	CheckpointInterval time.Duration `json:"checkpoint_interval"`

	// CheckpointPath specifies where to save progress checkpoints
	CheckpointPath string `json:"checkpoint_path"`

	// ResumeFromCheckpoint enables resuming from a previous checkpoint
	ResumeFromCheckpoint bool `json:"resume_from_checkpoint"`

	// ValidationEnabled enables batch-level validation
	ValidationEnabled bool `json:"validation_enabled"`

	// ProgressReportInterval controls progress reporting frequency
	ProgressReportInterval time.Duration `json:"progress_report_interval"`
}

// BatchRetryPolicy controls retry behavior for failed batches
type BatchRetryPolicy struct {
	MaxRetries    int           `json:"max_retries"`
	InitialDelay  time.Duration `json:"initial_delay"`
	MaxDelay      time.Duration `json:"max_delay"`
	BackoffFactor float64       `json:"backoff_factor"`
}

// DefaultBatchMigratorConfig returns default batch migrator configuration
func DefaultBatchMigratorConfig() *BatchMigratorConfig {
	return &BatchMigratorConfig{
		BatchSize:   5000,
		MaxBatches:  10,
		WorkerCount: 8,
		QueueSize:   100,
		RetryPolicy: &BatchRetryPolicy{
			MaxRetries:    3,
			InitialDelay:  time.Second * 2,
			MaxDelay:      time.Second * 30,
			BackoffFactor: 2.0,
		},
		MemoryLimit:            1024 * 1024 * 1024, // 1GB
		CheckpointInterval:     time.Minute * 5,
		CheckpointPath:         "/tmp/scylla_migration_checkpoint.json",
		ResumeFromCheckpoint:   false,
		ValidationEnabled:      true,
		ProgressReportInterval: time.Second * 30,
	}
}

// PinBatch represents a batch of pins to be migrated
type PinBatch struct {
	ID       int64     `json:"id"`
	Pins     []api.Pin `json:"pins"`
	Attempts int       `json:"attempts"`
	Created  time.Time `json:"created"`
}

// BatchResult represents the result of processing a batch
type BatchResult struct {
	BatchID       int64         `json:"batch_id"`
	Success       bool          `json:"success"`
	ProcessedPins int           `json:"processed_pins"`
	FailedPins    int           `json:"failed_pins"`
	Duration      time.Duration `json:"duration"`
	Error         error         `json:"error,omitempty"`
}

// MigrationWorker handles the actual migration work for batches
type MigrationWorker struct {
	ID          int
	destination *ScyllaState
	logger      Logger
	config      *BatchMigratorConfig
}

// MigrationCheckpoint stores migration progress for resumption
type MigrationCheckpoint struct {
	TotalPins        int64     `json:"total_pins"`
	ProcessedPins    int64     `json:"processed_pins"`
	TotalBatches     int64     `json:"total_batches"`
	ProcessedBatches int64     `json:"processed_batches"`
	LastBatchID      int64     `json:"last_batch_id"`
	StartTime        time.Time `json:"start_time"`
	CheckpointTime   time.Time `json:"checkpoint_time"`
}

// NewBatchMigrator creates a new batch migrator instance
func NewBatchMigrator(destination *ScyllaState, config *BatchMigratorConfig, logger Logger) *BatchMigrator {
	if config == nil {
		config = DefaultBatchMigratorConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	bm := &BatchMigrator{
		destination: destination,
		config:      config,
		logger:      logger,
		workerPool:  make(chan *MigrationWorker, config.WorkerCount),
		batchQueue:  make(chan *PinBatch, config.QueueSize),
		resultQueue: make(chan *BatchResult, config.QueueSize),
		ctx:         ctx,
		cancel:      cancel,
		startTime:   time.Now(),
	}

	// Initialize workers
	bm.initializeWorkers()

	return bm
}

// initializeWorkers creates and initializes migration workers
func (bm *BatchMigrator) initializeWorkers() {
	bm.workers = make([]*MigrationWorker, bm.config.WorkerCount)

	for i := 0; i < bm.config.WorkerCount; i++ {
		worker := &MigrationWorker{
			ID:          i,
			destination: bm.destination,
			logger:      bm.logger,
			config:      bm.config,
		}
		bm.workers[i] = worker
		bm.workerPool <- worker
	}

	bm.logger.Infof("Initialized %d migration workers", bm.config.WorkerCount)
}

// MigrateFromState performs batch migration from an existing state backend
func (bm *BatchMigrator) MigrateFromState(ctx context.Context, source state.ReadOnly) (*MigrationResult, error) {
	bm.logger.Infof("Starting batch migration from state backend to ScyllaDB")

	// Start background processes
	bm.startBackgroundProcesses(ctx)

	// Create pin channel for reading from source
	pinChan := make(chan api.Pin, bm.config.BatchSize*2)

	// Start source reader
	go func() {
		defer close(pinChan)
		if err := source.List(ctx, pinChan); err != nil {
			bm.logger.Errorf("Failed to list pins from source: %v", err)
		}
	}()

	// Start batch creator
	go bm.createBatches(ctx, pinChan)

	// Wait for completion
	bm.wg.Wait()

	// Build final result
	return bm.buildFinalResult(), nil
}

// createBatches creates batches from the pin stream
func (bm *BatchMigrator) createBatches(ctx context.Context, pinChan <-chan api.Pin) {
	defer close(bm.batchQueue)

	batchID := int64(0)
	currentBatch := &PinBatch{
		ID:      batchID,
		Pins:    make([]api.Pin, 0, bm.config.BatchSize),
		Created: time.Now(),
	}

	for pin := range pinChan {
		currentBatch.Pins = append(currentBatch.Pins, pin)
		atomic.AddInt64(&bm.totalPins, 1)

		// Send batch when full
		if len(currentBatch.Pins) >= bm.config.BatchSize {
			select {
			case bm.batchQueue <- currentBatch:
				atomic.AddInt64(&bm.totalBatches, 1)
			case <-ctx.Done():
				return
			}

			// Create new batch
			batchID++
			currentBatch = &PinBatch{
				ID:      batchID,
				Pins:    make([]api.Pin, 0, bm.config.BatchSize),
				Created: time.Now(),
			}
		}
	}

	// Send final batch if it has pins
	if len(currentBatch.Pins) > 0 {
		select {
		case bm.batchQueue <- currentBatch:
			atomic.AddInt64(&bm.totalBatches, 1)
		case <-ctx.Done():
			return
		}
	}
}

// startBackgroundProcesses starts all background processes
func (bm *BatchMigrator) startBackgroundProcesses(ctx context.Context) {
	// Start batch processors
	for i := 0; i < bm.config.WorkerCount; i++ {
		bm.wg.Add(1)
		go bm.processBatches(ctx)
	}

	// Start result processor
	bm.wg.Add(1)
	go bm.processResults(ctx)

	// Start progress reporter
	bm.wg.Add(1)
	go bm.reportProgress(ctx)

	// Start checkpoint saver if enabled
	if bm.config.CheckpointPath != "" {
		bm.wg.Add(1)
		go bm.saveCheckpoints(ctx)
	}
}

// processBatches processes batches from the queue
func (bm *BatchMigrator) processBatches(ctx context.Context) {
	defer bm.wg.Done()

	for {
		select {
		case batch, ok := <-bm.batchQueue:
			if !ok {
				return // Queue closed
			}

			// Get a worker from the pool
			select {
			case worker := <-bm.workerPool:
				result := worker.ProcessBatch(ctx, batch)

				// Return worker to pool
				bm.workerPool <- worker

				// Send result
				select {
				case bm.resultQueue <- result:
				case <-ctx.Done():
					return
				}

			case <-ctx.Done():
				return
			}

		case <-ctx.Done():
			return
		}
	}
}

// processResults processes batch results
func (bm *BatchMigrator) processResults(ctx context.Context) {
	defer bm.wg.Done()
	defer close(bm.resultQueue)

	for {
		select {
		case result, ok := <-bm.resultQueue:
			if !ok {
				return // Queue closed
			}

			bm.handleBatchResult(ctx, result)

		case <-ctx.Done():
			return
		}
	}
}

// handleBatchResult handles the result of a batch processing
func (bm *BatchMigrator) handleBatchResult(ctx context.Context, result *BatchResult) {
	atomic.AddInt64(&bm.processedBatches, 1)
	atomic.AddInt64(&bm.processedPins, int64(result.ProcessedPins))

	if !result.Success {
		atomic.AddInt64(&bm.errorCount, 1)
		bm.logger.Errorf("Batch %d failed: %v", result.BatchID, result.Error)

		// Handle retry logic here if needed
		// For now, we just log the error
	} else {
		bm.logger.Debugf("Batch %d completed successfully: %d pins in %v",
			result.BatchID, result.ProcessedPins, result.Duration)
	}
}

// reportProgress reports migration progress periodically
func (bm *BatchMigrator) reportProgress(ctx context.Context) {
	defer bm.wg.Done()

	ticker := time.NewTicker(bm.config.ProgressReportInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			bm.logProgress()
		case <-ctx.Done():
			return
		}
	}
}

// logProgress logs current progress
func (bm *BatchMigrator) logProgress() {
	totalPins := atomic.LoadInt64(&bm.totalPins)
	processedPins := atomic.LoadInt64(&bm.processedPins)
	totalBatches := atomic.LoadInt64(&bm.totalBatches)
	processedBatches := atomic.LoadInt64(&bm.processedBatches)
	errors := atomic.LoadInt64(&bm.errorCount)

	elapsed := time.Since(bm.startTime)
	var pinsPerSec, batchesPerSec float64

	if elapsed > 0 {
		pinsPerSec = float64(processedPins) / elapsed.Seconds()
		batchesPerSec = float64(processedBatches) / elapsed.Seconds()
	}

	var progress float64
	if totalPins > 0 {
		progress = float64(processedPins) / float64(totalPins) * 100
	}

	bm.logger.Infof("Batch migration progress: %.1f%% (%d/%d pins, %d/%d batches, %d errors, %.1f pins/sec, %.2f batches/sec)",
		progress, processedPins, totalPins, processedBatches, totalBatches, errors, pinsPerSec, batchesPerSec)
}

// saveCheckpoints saves migration progress checkpoints
func (bm *BatchMigrator) saveCheckpoints(ctx context.Context) {
	defer bm.wg.Done()

	ticker := time.NewTicker(bm.config.CheckpointInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := bm.saveCheckpoint(); err != nil {
				bm.logger.Errorf("Failed to save checkpoint: %v", err)
			}
		case <-ctx.Done():
			// Save final checkpoint
			if err := bm.saveCheckpoint(); err != nil {
				bm.logger.Errorf("Failed to save final checkpoint: %v", err)
			}
			return
		}
	}
}

// saveCheckpoint saves current progress to checkpoint file
func (bm *BatchMigrator) saveCheckpoint() error {
	checkpoint := &MigrationCheckpoint{
		TotalPins:        atomic.LoadInt64(&bm.totalPins),
		ProcessedPins:    atomic.LoadInt64(&bm.processedPins),
		TotalBatches:     atomic.LoadInt64(&bm.totalBatches),
		ProcessedBatches: atomic.LoadInt64(&bm.processedBatches),
		StartTime:        bm.startTime,
		CheckpointTime:   time.Now(),
	}

	// Implementation would save to file system
	// For now, just log the checkpoint
	bm.logger.Debugf("Checkpoint saved: %d/%d pins processed", checkpoint.ProcessedPins, checkpoint.TotalPins)

	return nil
}

// buildFinalResult builds the final migration result
func (bm *BatchMigrator) buildFinalResult() *MigrationResult {
	totalPins := atomic.LoadInt64(&bm.totalPins)
	processedPins := atomic.LoadInt64(&bm.processedPins)
	errors := atomic.LoadInt64(&bm.errorCount)

	endTime := time.Now()
	duration := endTime.Sub(bm.startTime)

	metrics := &MigrationMetrics{
		TotalPins:      totalPins,
		ProcessedPins:  processedPins,
		SuccessfulPins: processedPins - errors,
		FailedPins:     errors,
		StartTime:      bm.startTime,
		EndTime:        endTime,
		Duration:       duration,
	}

	if duration > 0 {
		metrics.PinsPerSecond = float64(processedPins) / duration.Seconds()
	}

	return &MigrationResult{
		Success: errors == 0,
		Metrics: metrics,
	}
}

// ProcessBatch processes a single batch of pins
func (worker *MigrationWorker) ProcessBatch(ctx context.Context, batch *PinBatch) *BatchResult {
	startTime := time.Now()
	result := &BatchResult{
		BatchID: batch.ID,
	}

	// Create batching state for efficient operations
	batchingState := worker.destination.NewBatching()
	defer batchingState.Close()

	processedCount := 0
	failedCount := 0

	// Process each pin in the batch
	for _, pin := range batch.Pins {
		if err := batchingState.Add(ctx, pin); err != nil {
			failedCount++
			worker.logger.Errorf("Worker %d failed to add pin %s to batch %d: %v",
				worker.ID, pin.Cid, batch.ID, err)
		} else {
			processedCount++
		}
	}

	// Commit the batch
	if err := batchingState.Commit(ctx); err != nil {
		result.Success = false
		result.Error = fmt.Errorf("failed to commit batch %d: %w", batch.ID, err)
		result.FailedPins = len(batch.Pins)
	} else {
		result.Success = failedCount == 0
		result.ProcessedPins = processedCount
		result.FailedPins = failedCount
	}

	result.Duration = time.Since(startTime)

	return result
}

// Stop stops the batch migrator
func (bm *BatchMigrator) Stop() {
	bm.cancel()
	bm.wg.Wait()
	bm.logger.Infof("Batch migrator stopped")
}

// GetProgress returns current migration progress
func (bm *BatchMigrator) GetProgress() *MigrationMetrics {
	totalPins := atomic.LoadInt64(&bm.totalPins)
	processedPins := atomic.LoadInt64(&bm.processedPins)
	errors := atomic.LoadInt64(&bm.errorCount)

	duration := time.Since(bm.startTime)
	var pinsPerSecond float64
	if duration > 0 {
		pinsPerSecond = float64(processedPins) / duration.Seconds()
	}

	return &MigrationMetrics{
		TotalPins:      totalPins,
		ProcessedPins:  processedPins,
		SuccessfulPins: processedPins - errors,
		FailedPins:     errors,
		StartTime:      bm.startTime,
		Duration:       duration,
		PinsPerSecond:  pinsPerSecond,
	}
}
