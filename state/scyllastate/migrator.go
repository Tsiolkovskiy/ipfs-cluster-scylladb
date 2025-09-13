package scyllastate

import (
	"context"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ipfs-cluster/ipfs-cluster/api"
	"github.com/ipfs-cluster/ipfs-cluster/state"
)

// Migrator handles data migration from existing state backends to ScyllaDB
type Migrator struct {
	destination *ScyllaState
	config      *MigratorConfig
	metrics     *MigrationMetrics
	logger      Logger

	// Progress tracking
	totalPins     int64
	processedPins int64
	errorCount    int64
	startTime     time.Time

	// Synchronization
	mu sync.RWMutex
}

// MigratorConfig holds configuration for migration operations
type MigratorConfig struct {
	// BatchSize controls how many pins are processed in each batch
	BatchSize int `json:"batch_size"`

	// WorkerCount controls the number of concurrent migration workers
	WorkerCount int `json:"worker_count"`

	// ValidationEnabled enables data integrity validation after migration
	ValidationEnabled bool `json:"validation_enabled"`

	// ValidationSampleRate controls what percentage of migrated data to validate (0.0-1.0)
	ValidationSampleRate float64 `json:"validation_sample_rate"`

	// ContinueOnError allows migration to continue even if some pins fail
	ContinueOnError bool `json:"continue_on_error"`

	// MaxRetries controls how many times to retry failed operations
	MaxRetries int `json:"max_retries"`

	// RetryDelay controls the delay between retry attempts
	RetryDelay time.Duration `json:"retry_delay"`

	// ProgressReportInterval controls how often to report progress
	ProgressReportInterval time.Duration `json:"progress_report_interval"`

	// DryRun enables dry-run mode (no actual data changes)
	DryRun bool `json:"dry_run"`
}

// DefaultMigratorConfig returns default migrator configuration
func DefaultMigratorConfig() *MigratorConfig {
	return &MigratorConfig{
		BatchSize:              1000,
		WorkerCount:            4,
		ValidationEnabled:      true,
		ValidationSampleRate:   0.1, // Validate 10% of migrated data
		ContinueOnError:        false,
		MaxRetries:             3,
		RetryDelay:             time.Second * 2,
		ProgressReportInterval: time.Second * 30,
		DryRun:                 false,
	}
}

// MigrationMetrics tracks migration progress and performance
type MigrationMetrics struct {
	TotalPins       int64         `json:"total_pins"`
	ProcessedPins   int64         `json:"processed_pins"`
	SuccessfulPins  int64         `json:"successful_pins"`
	FailedPins      int64         `json:"failed_pins"`
	ValidationPins  int64         `json:"validation_pins"`
	ValidationFails int64         `json:"validation_fails"`
	StartTime       time.Time     `json:"start_time"`
	EndTime         time.Time     `json:"end_time,omitempty"`
	Duration        time.Duration `json:"duration"`
	PinsPerSecond   float64       `json:"pins_per_second"`
}

// MigrationResult contains the result of a migration operation
type MigrationResult struct {
	Success          bool               `json:"success"`
	Metrics          *MigrationMetrics  `json:"metrics"`
	ValidationResult *ValidationSummary `json:"validation_result,omitempty"`
	Errors           []string           `json:"errors,omitempty"`
}

// ValidationSummary contains validation results
type ValidationSummary struct {
	TotalValidated   int64    `json:"total_validated"`
	ValidationPassed int64    `json:"validation_passed"`
	ValidationFailed int64    `json:"validation_failed"`
	FailureReasons   []string `json:"failure_reasons,omitempty"`
}

// Logger interface for migration logging
type Logger interface {
	Printf(format string, args ...interface{})
	Errorf(format string, args ...interface{})
	Warnf(format string, args ...interface{})
	Infof(format string, args ...interface{})
	Debugf(format string, args ...interface{})
}

// NewMigrator creates a new migrator instance
func NewMigrator(destination *ScyllaState, config *MigratorConfig, logger Logger) *Migrator {
	if config == nil {
		config = DefaultMigratorConfig()
	}

	return &Migrator{
		destination: destination,
		config:      config,
		metrics:     &MigrationMetrics{},
		logger:      logger,
		startTime:   time.Now(),
	}
}

// MigrateFromState migrates data from an existing state.State implementation
func (m *Migrator) MigrateFromState(ctx context.Context, source state.ReadOnly) (*MigrationResult, error) {
	m.logger.Infof("Starting migration from state backend to ScyllaDB")
	m.startTime = time.Now()
	m.metrics.StartTime = m.startTime

	// Create pin channel for streaming data
	pinChan := make(chan api.Pin, m.config.BatchSize*2)
	errorChan := make(chan error, m.config.WorkerCount)

	// Start progress reporting
	progressCtx, progressCancel := context.WithCancel(ctx)
	defer progressCancel()
	go m.reportProgress(progressCtx)

	// Start source reader
	go func() {
		defer close(pinChan)
		if err := source.List(ctx, pinChan); err != nil {
			m.logger.Errorf("Failed to list pins from source: %v", err)
			errorChan <- fmt.Errorf("source listing failed: %w", err)
		}
	}()

	// Start migration workers
	var wg sync.WaitGroup
	for i := 0; i < m.config.WorkerCount; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			m.migrationWorker(ctx, workerID, pinChan, errorChan)
		}(i)
	}

	// Wait for all workers to complete
	go func() {
		wg.Wait()
		close(errorChan)
	}()

	// Collect errors
	var errors []string
	for err := range errorChan {
		errors = append(errors, err.Error())
		if !m.config.ContinueOnError {
			m.logger.Errorf("Migration failed: %v", err)
			return m.buildResult(false, errors), err
		}
	}

	// Finalize metrics
	m.metrics.EndTime = time.Now()
	m.metrics.Duration = m.metrics.EndTime.Sub(m.metrics.StartTime)
	if m.metrics.Duration > 0 {
		m.metrics.PinsPerSecond = float64(m.metrics.ProcessedPins) / m.metrics.Duration.Seconds()
	}

	// Perform validation if enabled
	var validationResult *ValidationSummary
	if m.config.ValidationEnabled && !m.config.DryRun {
		m.logger.Infof("Starting post-migration validation")
		validationResult = m.validateMigration(ctx, source)
	}

	success := len(errors) == 0 || (m.config.ContinueOnError && m.metrics.SuccessfulPins > 0)
	result := m.buildResult(success, errors)
	result.ValidationResult = validationResult

	if success {
		m.logger.Infof("Migration completed successfully: %d pins migrated in %v (%.2f pins/sec)",
			m.metrics.ProcessedPins, m.metrics.Duration, m.metrics.PinsPerSecond)
	} else {
		m.logger.Errorf("Migration completed with errors: %d successful, %d failed",
			m.metrics.SuccessfulPins, m.metrics.FailedPins)
	}

	return result, nil
}

// MigrateFromReader migrates data from a serialized state reader
func (m *Migrator) MigrateFromReader(ctx context.Context, reader io.Reader) (*MigrationResult, error) {
	m.logger.Infof("Starting migration from serialized state reader to ScyllaDB")
	m.startTime = time.Now()
	m.metrics.StartTime = m.startTime

	if m.config.DryRun {
		m.logger.Infof("DRY RUN MODE: No actual data will be migrated")
		return m.simulateMigrationFromReader(ctx, reader)
	}

	// Use the destination's Unmarshal method for efficient bulk import
	if err := m.destination.Unmarshal(reader); err != nil {
		return m.buildResult(false, []string{err.Error()}), err
	}

	// Update metrics
	m.metrics.EndTime = time.Now()
	m.metrics.Duration = m.metrics.EndTime.Sub(m.metrics.StartTime)
	m.metrics.ProcessedPins = m.destination.GetPinCount()
	m.metrics.SuccessfulPins = m.metrics.ProcessedPins

	if m.metrics.Duration > 0 {
		m.metrics.PinsPerSecond = float64(m.metrics.ProcessedPins) / m.metrics.Duration.Seconds()
	}

	m.logger.Infof("Migration from reader completed: %d pins migrated in %v (%.2f pins/sec)",
		m.metrics.ProcessedPins, m.metrics.Duration, m.metrics.PinsPerSecond)

	return m.buildResult(true, nil), nil
}

// migrationWorker processes pins from the channel and migrates them
func (m *Migrator) migrationWorker(ctx context.Context, workerID int, pinChan <-chan api.Pin, errorChan chan<- error) {
	batch := make([]api.Pin, 0, m.config.BatchSize)

	for pin := range pinChan {
		batch = append(batch, pin)

		// Process batch when full
		if len(batch) >= m.config.BatchSize {
			if err := m.processBatch(ctx, workerID, batch); err != nil {
				errorChan <- fmt.Errorf("worker %d batch processing failed: %w", workerID, err)
				if !m.config.ContinueOnError {
					return
				}
			}
			batch = batch[:0] // Reset batch
		}
	}

	// Process remaining pins in batch
	if len(batch) > 0 {
		if err := m.processBatch(ctx, workerID, batch); err != nil {
			errorChan <- fmt.Errorf("worker %d final batch processing failed: %w", workerID, err)
		}
	}
}

// processBatch processes a batch of pins
func (m *Migrator) processBatch(ctx context.Context, workerID int, batch []api.Pin) error {
	if m.config.DryRun {
		// In dry run mode, just count the pins
		atomic.AddInt64(&m.processedPins, int64(len(batch)))
		atomic.AddInt64(&m.metrics.ProcessedPins, int64(len(batch)))
		atomic.AddInt64(&m.metrics.SuccessfulPins, int64(len(batch)))
		return nil
	}

	// Create batching state for efficient bulk operations
	batchingState := m.destination.NewBatching()
	defer batchingState.Close()

	successCount := 0
	for _, pin := range batch {
		if err := m.addPinWithRetry(ctx, batchingState, pin); err != nil {
			atomic.AddInt64(&m.errorCount, 1)
			atomic.AddInt64(&m.metrics.FailedPins, 1)
			m.logger.Errorf("Worker %d failed to add pin %s: %v", workerID, pin.Cid, err)

			if !m.config.ContinueOnError {
				return err
			}
		} else {
			successCount++
		}
	}

	// Commit the batch
	if err := batchingState.Commit(ctx); err != nil {
		atomic.AddInt64(&m.errorCount, int64(successCount))
		atomic.AddInt64(&m.metrics.FailedPins, int64(successCount))
		return fmt.Errorf("failed to commit batch: %w", err)
	}

	// Update metrics
	atomic.AddInt64(&m.processedPins, int64(len(batch)))
	atomic.AddInt64(&m.metrics.ProcessedPins, int64(len(batch)))
	atomic.AddInt64(&m.metrics.SuccessfulPins, int64(successCount))

	return nil
}

// addPinWithRetry adds a pin with retry logic
func (m *Migrator) addPinWithRetry(ctx context.Context, batchingState *ScyllaBatchingState, pin api.Pin) error {
	var lastErr error

	for attempt := 0; attempt <= m.config.MaxRetries; attempt++ {
		if attempt > 0 {
			// Wait before retry
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(m.config.RetryDelay):
			}
		}

		if err := batchingState.Add(ctx, pin); err != nil {
			lastErr = err
			continue
		}

		return nil // Success
	}

	return fmt.Errorf("failed after %d attempts: %w", m.config.MaxRetries, lastErr)
}

// validateMigration performs post-migration validation
func (m *Migrator) validateMigration(ctx context.Context, source state.ReadOnly) *ValidationSummary {
	summary := &ValidationSummary{}

	// Create pin channel for validation
	pinChan := make(chan api.Pin, m.config.BatchSize)

	// Start source reader
	go func() {
		defer close(pinChan)
		if err := source.List(ctx, pinChan); err != nil {
			m.logger.Errorf("Failed to list pins for validation: %v", err)
		}
	}()

	// Validate pins
	for pin := range pinChan {
		// Sample validation based on configured rate
		if m.shouldValidatePin() {
			summary.TotalValidated++

			// Check if pin exists in destination
			exists, err := m.destination.Has(ctx, pin.Cid)
			if err != nil {
				summary.ValidationFailed++
				summary.FailureReasons = append(summary.FailureReasons,
					fmt.Sprintf("Failed to check pin %s: %v", pin.Cid, err))
				continue
			}

			if !exists {
				summary.ValidationFailed++
				summary.FailureReasons = append(summary.FailureReasons,
					fmt.Sprintf("Pin %s missing in destination", pin.Cid))
				continue
			}

			// Validate pin data integrity
			destPin, err := m.destination.Get(ctx, pin.Cid)
			if err != nil {
				summary.ValidationFailed++
				summary.FailureReasons = append(summary.FailureReasons,
					fmt.Sprintf("Failed to get pin %s from destination: %v", pin.Cid, err))
				continue
			}

			if !m.pinsEqual(pin, destPin) {
				summary.ValidationFailed++
				summary.FailureReasons = append(summary.FailureReasons,
					fmt.Sprintf("Pin %s data mismatch", pin.Cid))
				continue
			}

			summary.ValidationPassed++
		}
	}

	m.metrics.ValidationPins = summary.TotalValidated
	m.metrics.ValidationFails = summary.ValidationFailed

	m.logger.Infof("Validation completed: %d validated, %d passed, %d failed",
		summary.TotalValidated, summary.ValidationPassed, summary.ValidationFailed)

	return summary
}

// shouldValidatePin determines if a pin should be validated based on sample rate
func (m *Migrator) shouldValidatePin() bool {
	if m.config.ValidationSampleRate >= 1.0 {
		return true
	}
	if m.config.ValidationSampleRate <= 0.0 {
		return false
	}

	// Simple random sampling based on processed count
	return (atomic.LoadInt64(&m.processedPins) % int64(1.0/m.config.ValidationSampleRate)) == 0
}

// pinsEqual compares two pins for equality
func (m *Migrator) pinsEqual(pin1, pin2 api.Pin) bool {
	// Compare essential fields
	if !pin1.Cid.Equals(pin2.Cid) {
		return false
	}

	if pin1.Type != pin2.Type {
		return false
	}

	if pin1.ReplicationFactorMin != pin2.ReplicationFactorMin {
		return false
	}

	if pin1.ReplicationFactorMax != pin2.ReplicationFactorMax {
		return false
	}

	if pin1.Name != pin2.Name {
		return false
	}

	// Compare expiration (allow small time differences due to serialization)
	if !pin1.ExpireAt.IsZero() || !pin2.ExpireAt.IsZero() {
		diff := pin1.ExpireAt.Sub(pin2.ExpireAt)
		if diff < 0 {
			diff = -diff
		}
		if diff > time.Second {
			return false
		}
	}

	return true
}

// reportProgress reports migration progress periodically
func (m *Migrator) reportProgress(ctx context.Context) {
	ticker := time.NewTicker(m.config.ProgressReportInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			processed := atomic.LoadInt64(&m.processedPins)
			errors := atomic.LoadInt64(&m.errorCount)
			elapsed := time.Since(m.startTime)

			var rate float64
			if elapsed > 0 {
				rate = float64(processed) / elapsed.Seconds()
			}

			m.logger.Infof("Migration progress: %d processed, %d errors, %.2f pins/sec, elapsed: %v",
				processed, errors, rate, elapsed)
		}
	}
}

// simulateMigrationFromReader simulates migration from reader for dry run
func (m *Migrator) simulateMigrationFromReader(ctx context.Context, reader io.Reader) (*MigrationResult, error) {
	// Count pins in the reader without actually migrating
	pinCount := int64(0)

	// Simple pin counting by parsing the format
	// This is a simplified version - in practice you'd parse the actual format
	buffer := make([]byte, 4096)
	for {
		n, err := reader.Read(buffer)
		if err == io.EOF {
			break
		}
		if err != nil {
			return m.buildResult(false, []string{err.Error()}), err
		}

		// Estimate pin count based on data size (rough approximation)
		// In practice, you'd parse the actual format to count pins accurately
		pinCount += int64(n / 100) // Rough estimate: 100 bytes per pin
	}

	m.metrics.EndTime = time.Now()
	m.metrics.Duration = m.metrics.EndTime.Sub(m.metrics.StartTime)
	m.metrics.ProcessedPins = pinCount
	m.metrics.SuccessfulPins = pinCount

	m.logger.Infof("DRY RUN: Would migrate approximately %d pins", pinCount)

	return m.buildResult(true, nil), nil
}

// buildResult builds the final migration result
func (m *Migrator) buildResult(success bool, errors []string) *MigrationResult {
	return &MigrationResult{
		Success: success,
		Metrics: m.metrics,
		Errors:  errors,
	}
}

// GetProgress returns current migration progress
func (m *Migrator) GetProgress() *MigrationMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Create a copy of current metrics
	metrics := *m.metrics
	metrics.ProcessedPins = atomic.LoadInt64(&m.processedPins)

	if !metrics.EndTime.IsZero() {
		metrics.Duration = metrics.EndTime.Sub(metrics.StartTime)
	} else {
		metrics.Duration = time.Since(metrics.StartTime)
	}

	if metrics.Duration > 0 {
		metrics.PinsPerSecond = float64(metrics.ProcessedPins) / metrics.Duration.Seconds()
	}

	return &metrics
}

// Cancel cancels the migration operation
func (m *Migrator) Cancel() {
	// Implementation would depend on how cancellation is handled
	// For now, this is a placeholder
	m.logger.Infof("Migration cancellation requested")
}
