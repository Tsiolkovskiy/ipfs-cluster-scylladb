package scyllastate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ipfs-cluster/ipfs-cluster/api"
	"github.com/ipfs-cluster/ipfs-cluster/state"
)

// MigrationValidator provides comprehensive data integrity validation after migration
type MigrationValidator struct {
	source      state.ReadOnly
	destination *ScyllaState
	config      *ValidationConfig
	logger      Logger

	// Progress tracking
	totalPins     int64
	validatedPins int64
	validPins     int64
	invalidPins   int64
	startTime     time.Time

	// Validation results
	results   []*ValidationIssue
	resultsMu sync.Mutex

	// Statistics
	stats *ValidationStatistics
}

// ValidationConfig holds configuration for migration validation
type ValidationConfig struct {
	// SampleRate controls what percentage of pins to validate (0.0-1.0)
	SampleRate float64 `json:"sample_rate"`

	// WorkerCount controls the number of validation workers
	WorkerCount int `json:"worker_count"`

	// BatchSize controls the batch size for validation operations
	BatchSize int `json:"batch_size"`

	// DeepValidation enables deep content validation (slower but more thorough)
	DeepValidation bool `json:"deep_validation"`

	// ChecksumValidation enables checksum validation of pin data
	ChecksumValidation bool `json:"checksum_validation"`

	// MetadataValidation enables validation of pin metadata
	MetadataValidation bool `json:"metadata_validation"`

	// TimestampTolerance allows for small timestamp differences (in seconds)
	TimestampTolerance int64 `json:"timestamp_tolerance"`

	// MaxIssues limits the number of validation issues to collect
	MaxIssues int `json:"max_issues"`

	// ProgressReportInterval controls progress reporting frequency
	ProgressReportInterval time.Duration `json:"progress_report_interval"`

	// FailFast stops validation on first critical error
	FailFast bool `json:"fail_fast"`
}

// DefaultValidationConfig returns default validation configuration
func DefaultValidationConfig() *ValidationConfig {
	return &ValidationConfig{
		SampleRate:             1.0, // Validate all pins by default
		WorkerCount:            4,
		BatchSize:              1000,
		DeepValidation:         false,
		ChecksumValidation:     true,
		MetadataValidation:     true,
		TimestampTolerance:     5, // 5 seconds tolerance
		MaxIssues:              1000,
		ProgressReportInterval: time.Second * 30,
		FailFast:               false,
	}
}

// ValidationIssue represents a validation problem found during validation
type ValidationIssue struct {
	Type        ValidationIssueType    `json:"type"`
	Severity    ValidationSeverity     `json:"severity"`
	CID         string                 `json:"cid"`
	Description string                 `json:"description"`
	Details     map[string]interface{} `json:"details,omitempty"`
	Timestamp   time.Time              `json:"timestamp"`
}

// ValidationIssueType represents the type of validation issue
type ValidationIssueType string

const (
	ValidationIssueTypeMissing     ValidationIssueType = "missing"
	ValidationIssueTypeMismatch    ValidationIssueType = "mismatch"
	ValidationIssueTypeCorrupted   ValidationIssueType = "corrupted"
	ValidationIssueTypeMetadata    ValidationIssueType = "metadata"
	ValidationIssueTypeChecksum    ValidationIssueType = "checksum"
	ValidationIssueTypeTimestamp   ValidationIssueType = "timestamp"
	ValidationIssueTypePermissions ValidationIssueType = "permissions"
)

// ValidationSeverity represents the severity of a validation issue
type ValidationSeverity string

const (
	ValidationSeverityCritical ValidationSeverity = "critical"
	ValidationSeverityMajor    ValidationSeverity = "major"
	ValidationSeverityMinor    ValidationSeverity = "minor"
	ValidationSeverityWarning  ValidationSeverity = "warning"
)

// ValidationStatistics contains validation statistics
type ValidationStatistics struct {
	TotalPins        int64                         `json:"total_pins"`
	ValidatedPins    int64                         `json:"validated_pins"`
	ValidPins        int64                         `json:"valid_pins"`
	InvalidPins      int64                         `json:"invalid_pins"`
	SkippedPins      int64                         `json:"skipped_pins"`
	IssuesByType     map[ValidationIssueType]int64 `json:"issues_by_type"`
	IssuesBySeverity map[ValidationSeverity]int64  `json:"issues_by_severity"`
	ValidationRate   float64                       `json:"validation_rate"`
	Duration         time.Duration                 `json:"duration"`
	StartTime        time.Time                     `json:"start_time"`
	EndTime          time.Time                     `json:"end_time"`
}

// ValidationReport contains the complete validation results
type ValidationReport struct {
	Success     bool                  `json:"success"`
	Statistics  *ValidationStatistics `json:"statistics"`
	Issues      []*ValidationIssue    `json:"issues"`
	Summary     string                `json:"summary"`
	GeneratedAt time.Time             `json:"generated_at"`
}

// NewMigrationValidator creates a new migration validator
func NewMigrationValidator(source state.ReadOnly, destination *ScyllaState, config *ValidationConfig, logger Logger) *MigrationValidator {
	if config == nil {
		config = DefaultValidationConfig()
	}

	return &MigrationValidator{
		source:      source,
		destination: destination,
		config:      config,
		logger:      logger,
		results:     make([]*ValidationIssue, 0),
		stats: &ValidationStatistics{
			IssuesByType:     make(map[ValidationIssueType]int64),
			IssuesBySeverity: make(map[ValidationSeverity]int64),
		},
		startTime: time.Now(),
	}
}

// ValidateMigration performs comprehensive validation of the migration
func (mv *MigrationValidator) ValidateMigration(ctx context.Context) (*ValidationReport, error) {
	mv.logger.Infof("Starting migration validation with sample rate %.2f", mv.config.SampleRate)
	mv.startTime = time.Now()
	mv.stats.StartTime = mv.startTime

	// Start progress reporting
	progressCtx, progressCancel := context.WithCancel(ctx)
	defer progressCancel()
	go mv.reportProgress(progressCtx)

	// Create pin channel for streaming validation
	pinChan := make(chan api.Pin, mv.config.BatchSize*2)
	errorChan := make(chan error, mv.config.WorkerCount)

	// Start source reader
	go func() {
		defer close(pinChan)
		if err := mv.source.List(ctx, pinChan); err != nil {
			mv.logger.Errorf("Failed to list pins from source: %v", err)
			errorChan <- fmt.Errorf("source listing failed: %w", err)
		}
	}()

	// Start validation workers
	var wg sync.WaitGroup
	for i := 0; i < mv.config.WorkerCount; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			mv.validationWorker(ctx, workerID, pinChan, errorChan)
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
		if mv.config.FailFast {
			mv.logger.Errorf("Validation failed (fail-fast mode): %v", err)
			return mv.buildReport(false, errors), err
		}
	}

	// Finalize statistics
	mv.stats.EndTime = time.Now()
	mv.stats.Duration = mv.stats.EndTime.Sub(mv.stats.StartTime)
	if mv.stats.Duration > 0 {
		mv.stats.ValidationRate = float64(mv.stats.ValidatedPins) / mv.stats.Duration.Seconds()
	}

	success := len(errors) == 0 && mv.stats.InvalidPins == 0
	report := mv.buildReport(success, errors)

	if success {
		mv.logger.Infof("Validation completed successfully: %d pins validated, %d valid, %d invalid",
			mv.stats.ValidatedPins, mv.stats.ValidPins, mv.stats.InvalidPins)
	} else {
		mv.logger.Errorf("Validation completed with issues: %d pins validated, %d valid, %d invalid, %d errors",
			mv.stats.ValidatedPins, mv.stats.ValidPins, mv.stats.InvalidPins, len(errors))
	}

	return report, nil
}

// validationWorker performs validation work
func (mv *MigrationValidator) validationWorker(ctx context.Context, workerID int, pinChan <-chan api.Pin, errorChan chan<- error) {
	for pin := range pinChan {
		atomic.AddInt64(&mv.totalPins, 1)
		atomic.AddInt64(&mv.stats.TotalPins, 1)

		// Apply sampling
		if !mv.shouldValidatePin() {
			atomic.AddInt64(&mv.stats.SkippedPins, 1)
			continue
		}

		if err := mv.validatePin(ctx, pin); err != nil {
			errorChan <- fmt.Errorf("worker %d validation failed for pin %s: %w", workerID, pin.Cid, err)
			if mv.config.FailFast {
				return
			}
		}

		atomic.AddInt64(&mv.validatedPins, 1)
		atomic.AddInt64(&mv.stats.ValidatedPins, 1)

		// Check if we've reached the maximum number of issues
		mv.resultsMu.Lock()
		issueCount := len(mv.results)
		mv.resultsMu.Unlock()

		if issueCount >= mv.config.MaxIssues {
			mv.logger.Warnf("Reached maximum number of validation issues (%d), stopping validation", mv.config.MaxIssues)
			return
		}
	}
}

// validatePin validates a single pin
func (mv *MigrationValidator) validatePin(ctx context.Context, sourcePin api.Pin) error {
	// Check if pin exists in destination
	exists, err := mv.destination.Has(ctx, sourcePin.Cid)
	if err != nil {
		return fmt.Errorf("failed to check pin existence: %w", err)
	}

	if !exists {
		mv.addValidationIssue(&ValidationIssue{
			Type:        ValidationIssueTypeMissing,
			Severity:    ValidationSeverityCritical,
			CID:         sourcePin.Cid.String(),
			Description: "Pin missing in destination",
			Timestamp:   time.Now(),
		})
		atomic.AddInt64(&mv.invalidPins, 1)
		atomic.AddInt64(&mv.stats.InvalidPins, 1)
		return nil
	}

	// Get pin from destination
	destPin, err := mv.destination.Get(ctx, sourcePin.Cid)
	if err != nil {
		mv.addValidationIssue(&ValidationIssue{
			Type:        ValidationIssueTypeCorrupted,
			Severity:    ValidationSeverityCritical,
			CID:         sourcePin.Cid.String(),
			Description: fmt.Sprintf("Failed to retrieve pin from destination: %v", err),
			Timestamp:   time.Now(),
		})
		atomic.AddInt64(&mv.invalidPins, 1)
		atomic.AddInt64(&mv.stats.InvalidPins, 1)
		return nil
	}

	// Validate pin data
	if err := mv.validatePinData(sourcePin, destPin); err != nil {
		atomic.AddInt64(&mv.invalidPins, 1)
		atomic.AddInt64(&mv.stats.InvalidPins, 1)
		return nil
	}

	atomic.AddInt64(&mv.validPins, 1)
	atomic.AddInt64(&mv.stats.ValidPins, 1)
	return nil
}

// validatePinData validates the data integrity of a pin
func (mv *MigrationValidator) validatePinData(sourcePin, destPin api.Pin) error {
	// Basic field validation
	if !sourcePin.Cid.Equals(destPin.Cid) {
		mv.addValidationIssue(&ValidationIssue{
			Type:        ValidationIssueTypeMismatch,
			Severity:    ValidationSeverityCritical,
			CID:         sourcePin.Cid.String(),
			Description: "CID mismatch between source and destination",
			Details: map[string]interface{}{
				"source_cid":      sourcePin.Cid.String(),
				"destination_cid": destPin.Cid.String(),
			},
			Timestamp: time.Now(),
		})
		return fmt.Errorf("CID mismatch")
	}

	if sourcePin.Type != destPin.Type {
		mv.addValidationIssue(&ValidationIssue{
			Type:        ValidationIssueTypeMismatch,
			Severity:    ValidationSeverityMajor,
			CID:         sourcePin.Cid.String(),
			Description: "Pin type mismatch",
			Details: map[string]interface{}{
				"source_type":      sourcePin.Type,
				"destination_type": destPin.Type,
			},
			Timestamp: time.Now(),
		})
	}

	if sourcePin.ReplicationFactorMin != destPin.ReplicationFactorMin {
		mv.addValidationIssue(&ValidationIssue{
			Type:        ValidationIssueTypeMismatch,
			Severity:    ValidationSeverityMinor,
			CID:         sourcePin.Cid.String(),
			Description: "Replication factor mismatch",
			Details: map[string]interface{}{
				"source_rf":      sourcePin.ReplicationFactorMin,
				"destination_rf": destPin.ReplicationFactorMin,
			},
			Timestamp: time.Now(),
		})
	}

	// Metadata validation
	if mv.config.MetadataValidation {
		if err := mv.validatePinMetadata(sourcePin, destPin); err != nil {
			return err
		}
	}

	// Timestamp validation
	if err := mv.validatePinTimestamp(sourcePin, destPin); err != nil {
		return err
	}

	// Checksum validation
	if mv.config.ChecksumValidation {
		if err := mv.validatePinChecksum(sourcePin, destPin); err != nil {
			return err
		}
	}

	// Deep validation
	if mv.config.DeepValidation {
		if err := mv.validatePinDeep(sourcePin, destPin); err != nil {
			return err
		}
	}

	return nil
}

// validatePinMetadata validates pin metadata
func (mv *MigrationValidator) validatePinMetadata(sourcePin, destPin api.Pin) error {
	if sourcePin.Name != destPin.Name {
		mv.addValidationIssue(&ValidationIssue{
			Type:        ValidationIssueTypeMetadata,
			Severity:    ValidationSeverityMinor,
			CID:         sourcePin.Cid.String(),
			Description: "Pin name mismatch",
			Details: map[string]interface{}{
				"source_name":      sourcePin.Name,
				"destination_name": destPin.Name,
			},
			Timestamp: time.Now(),
		})
	}

	// Validate expiration
	if !sourcePin.ExpireAt.IsZero() || !destPin.ExpireAt.IsZero() {
		diff := sourcePin.ExpireAt.Sub(destPin.ExpireAt)
		if diff < 0 {
			diff = -diff
		}
		if diff > time.Duration(mv.config.TimestampTolerance)*time.Second {
			mv.addValidationIssue(&ValidationIssue{
				Type:        ValidationIssueTypeTimestamp,
				Severity:    ValidationSeverityMinor,
				CID:         sourcePin.Cid.String(),
				Description: "Expiration time mismatch",
				Details: map[string]interface{}{
					"source_expire":      sourcePin.ExpireAt,
					"destination_expire": destPin.ExpireAt,
					"difference_seconds": diff.Seconds(),
				},
				Timestamp: time.Now(),
			})
		}
	}

	return nil
}

// validatePinTimestamp validates pin timestamps
func (mv *MigrationValidator) validatePinTimestamp(sourcePin, destPin api.Pin) error {
	if !sourcePin.Timestamp.IsZero() && !destPin.Timestamp.IsZero() {
		diff := sourcePin.Timestamp.Sub(destPin.Timestamp)
		if diff < 0 {
			diff = -diff
		}
		if diff > time.Duration(mv.config.TimestampTolerance)*time.Second {
			mv.addValidationIssue(&ValidationIssue{
				Type:        ValidationIssueTypeTimestamp,
				Severity:    ValidationSeverityWarning,
				CID:         sourcePin.Cid.String(),
				Description: "Timestamp mismatch within tolerance",
				Details: map[string]interface{}{
					"source_timestamp":      sourcePin.Timestamp,
					"destination_timestamp": destPin.Timestamp,
					"difference_seconds":    diff.Seconds(),
				},
				Timestamp: time.Now(),
			})
		}
	}

	return nil
}

// validatePinChecksum validates pin data checksums
func (mv *MigrationValidator) validatePinChecksum(sourcePin, destPin api.Pin) error {
	// Generate checksums for both pins
	sourceChecksum := mv.generatePinChecksum(sourcePin)
	destChecksum := mv.generatePinChecksum(destPin)

	if sourceChecksum != destChecksum {
		mv.addValidationIssue(&ValidationIssue{
			Type:        ValidationIssueTypeChecksum,
			Severity:    ValidationSeverityCritical,
			CID:         sourcePin.Cid.String(),
			Description: "Pin data checksum mismatch",
			Details: map[string]interface{}{
				"source_checksum":      sourceChecksum,
				"destination_checksum": destChecksum,
			},
			Timestamp: time.Now(),
		})
		return fmt.Errorf("checksum mismatch")
	}

	return nil
}

// generatePinChecksum generates a checksum for pin data
func (mv *MigrationValidator) generatePinChecksum(pin api.Pin) string {
	hasher := sha256.New()

	// Hash essential pin data
	hasher.Write([]byte(pin.Cid.String()))
	hasher.Write([]byte(fmt.Sprintf("%d", pin.Type)))
	hasher.Write([]byte(fmt.Sprintf("%d", pin.ReplicationFactorMin)))
	hasher.Write([]byte(pin.Name))

	if !pin.ExpireAt.IsZero() {
		hasher.Write([]byte(pin.ExpireAt.Format(time.RFC3339)))
	}

	return hex.EncodeToString(hasher.Sum(nil))
}

// validatePinDeep performs deep validation of pin data
func (mv *MigrationValidator) validatePinDeep(sourcePin, destPin api.Pin) error {
	// Deep validation would involve more comprehensive checks
	// For now, this is a placeholder for future enhancements

	// Could include:
	// - Validation of pin origins
	// - Validation of pin allocations
	// - Cross-reference with IPFS node
	// - Validation of pin dependencies

	return nil
}

// shouldValidatePin determines if a pin should be validated based on sample rate
func (mv *MigrationValidator) shouldValidatePin() bool {
	if mv.config.SampleRate >= 1.0 {
		return true
	}
	if mv.config.SampleRate <= 0.0 {
		return false
	}

	// Simple random sampling based on total pins processed
	return (atomic.LoadInt64(&mv.totalPins) % int64(1.0/mv.config.SampleRate)) == 0
}

// addValidationIssue adds a validation issue to the results
func (mv *MigrationValidator) addValidationIssue(issue *ValidationIssue) {
	mv.resultsMu.Lock()
	defer mv.resultsMu.Unlock()

	if len(mv.results) < mv.config.MaxIssues {
		mv.results = append(mv.results, issue)
	}

	// Update statistics
	mv.stats.IssuesByType[issue.Type]++
	mv.stats.IssuesBySeverity[issue.Severity]++
}

// reportProgress reports validation progress periodically
func (mv *MigrationValidator) reportProgress(ctx context.Context) {
	ticker := time.NewTicker(mv.config.ProgressReportInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			totalPins := atomic.LoadInt64(&mv.totalPins)
			validatedPins := atomic.LoadInt64(&mv.validatedPins)
			validPins := atomic.LoadInt64(&mv.validPins)
			invalidPins := atomic.LoadInt64(&mv.invalidPins)

			elapsed := time.Since(mv.startTime)
			var rate float64
			if elapsed > 0 {
				rate = float64(validatedPins) / elapsed.Seconds()
			}

			mv.resultsMu.Lock()
			issueCount := len(mv.results)
			mv.resultsMu.Unlock()

			mv.logger.Infof("Validation progress: %d total, %d validated, %d valid, %d invalid, %d issues, %.2f pins/sec",
				totalPins, validatedPins, validPins, invalidPins, issueCount, rate)
		}
	}
}

// buildReport builds the final validation report
func (mv *MigrationValidator) buildReport(success bool, errors []string) *ValidationReport {
	mv.resultsMu.Lock()
	issues := make([]*ValidationIssue, len(mv.results))
	copy(issues, mv.results)
	mv.resultsMu.Unlock()

	summary := mv.generateSummary(success, errors)

	return &ValidationReport{
		Success:     success,
		Statistics:  mv.stats,
		Issues:      issues,
		Summary:     summary,
		GeneratedAt: time.Now(),
	}
}

// generateSummary generates a human-readable summary of validation results
func (mv *MigrationValidator) generateSummary(success bool, errors []string) string {
	if success {
		return fmt.Sprintf("Validation completed successfully. %d pins validated, %d valid, %d invalid.",
			mv.stats.ValidatedPins, mv.stats.ValidPins, mv.stats.InvalidPins)
	}

	summary := fmt.Sprintf("Validation completed with issues. %d pins validated, %d valid, %d invalid.",
		mv.stats.ValidatedPins, mv.stats.ValidPins, mv.stats.InvalidPins)

	if len(errors) > 0 {
		summary += fmt.Sprintf(" %d errors encountered.", len(errors))
	}

	mv.resultsMu.Lock()
	issueCount := len(mv.results)
	mv.resultsMu.Unlock()

	if issueCount > 0 {
		summary += fmt.Sprintf(" %d validation issues found.", issueCount)
	}

	return summary
}

// GetProgress returns current validation progress
func (mv *MigrationValidator) GetProgress() *ValidationStatistics {
	stats := *mv.stats
	stats.TotalPins = atomic.LoadInt64(&mv.totalPins)
	stats.ValidatedPins = atomic.LoadInt64(&mv.validatedPins)
	stats.ValidPins = atomic.LoadInt64(&mv.validPins)
	stats.InvalidPins = atomic.LoadInt64(&mv.invalidPins)

	if !stats.EndTime.IsZero() {
		stats.Duration = stats.EndTime.Sub(stats.StartTime)
	} else {
		stats.Duration = time.Since(stats.StartTime)
	}

	if stats.Duration > 0 {
		stats.ValidationRate = float64(stats.ValidatedPins) / stats.Duration.Seconds()
	}

	return &stats
}
