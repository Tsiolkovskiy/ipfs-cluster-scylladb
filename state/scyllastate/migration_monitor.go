package scyllastate

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// MigrationMonitor provides real-time monitoring and progress tracking for migration operations
type MigrationMonitor struct {
	config    *MonitorConfig
	logger    Logger
	startTime time.Time

	// Progress tracking
	currentOperation string
	totalOperations  int64
	completedOps     int64
	failedOps        int64

	// Real-time metrics
	metrics     *MigrationMetrics
	metricsLock sync.RWMutex

	// Event tracking
	events     []MigrationEvent
	eventsLock sync.Mutex
	maxEvents  int

	// Status tracking
	status     MigrationStatus
	statusLock sync.RWMutex

	// Reporting
	reportFile   *os.File
	reportTicker *time.Ticker

	// Control
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// MonitorConfig holds configuration for migration monitoring
type MonitorConfig struct {
	// ReportInterval controls how often to generate progress reports
	ReportInterval time.Duration `json:"report_interval"`

	// ReportFile specifies where to write progress reports (optional)
	ReportFile string `json:"report_file"`

	// MaxEvents controls the maximum number of events to keep in memory
	MaxEvents int `json:"max_events"`

	// EnableRealTimeMetrics enables real-time metrics collection
	EnableRealTimeMetrics bool `json:"enable_real_time_metrics"`

	// MetricsInterval controls metrics collection frequency
	MetricsInterval time.Duration `json:"metrics_interval"`

	// EnableEventLogging enables detailed event logging
	EnableEventLogging bool `json:"enable_event_logging"`

	// AlertThresholds defines thresholds for generating alerts
	AlertThresholds *AlertThresholds `json:"alert_thresholds"`
}

// AlertThresholds defines thresholds for generating alerts during migration
type AlertThresholds struct {
	// ErrorRateThreshold triggers alert when error rate exceeds this percentage
	ErrorRateThreshold float64 `json:"error_rate_threshold"`

	// SlowOperationThreshold triggers alert when operations take longer than this
	SlowOperationThreshold time.Duration `json:"slow_operation_threshold"`

	// MemoryUsageThreshold triggers alert when memory usage exceeds this (in bytes)
	MemoryUsageThreshold int64 `json:"memory_usage_threshold"`

	// QueueDepthThreshold triggers alert when queue depth exceeds this
	QueueDepthThreshold int64 `json:"queue_depth_threshold"`
}

// MigrationEvent represents a significant event during migration
type MigrationEvent struct {
	Timestamp time.Time              `json:"timestamp"`
	Type      MigrationEventType     `json:"type"`
	Severity  EventSeverity          `json:"severity"`
	Operation string                 `json:"operation"`
	Message   string                 `json:"message"`
	Details   map[string]interface{} `json:"details,omitempty"`
	Duration  time.Duration          `json:"duration,omitempty"`
}

// MigrationEventType represents the type of migration event
type MigrationEventType string

const (
	EventTypeStart       MigrationEventType = "start"
	EventTypeProgress    MigrationEventType = "progress"
	EventTypeComplete    MigrationEventType = "complete"
	EventTypeError       MigrationEventType = "error"
	EventTypeWarning     MigrationEventType = "warning"
	EventTypeAlert       MigrationEventType = "alert"
	EventTypeMilestone   MigrationEventType = "milestone"
	EventTypePerformance MigrationEventType = "performance"
)

// EventSeverity represents the severity of an event
type EventSeverity string

const (
	SeverityInfo     EventSeverity = "info"
	SeverityWarning  EventSeverity = "warning"
	SeverityError    EventSeverity = "error"
	SeverityCritical EventSeverity = "critical"
)

// MigrationStatus represents the current status of migration
type MigrationStatus string

const (
	StatusIdle       MigrationStatus = "idle"
	StatusStarting   MigrationStatus = "starting"
	StatusRunning    MigrationStatus = "running"
	StatusCompleting MigrationStatus = "completing"
	StatusCompleted  MigrationStatus = "completed"
	StatusFailed     MigrationStatus = "failed"
	StatusCancelled  MigrationStatus = "cancelled"
)

// ProgressReport contains a comprehensive progress report
type ProgressReport struct {
	Timestamp        time.Time           `json:"timestamp"`
	Status           MigrationStatus     `json:"status"`
	CurrentOperation string              `json:"current_operation"`
	Progress         *ProgressMetrics    `json:"progress"`
	Performance      *PerformanceMetrics `json:"performance"`
	RecentEvents     []MigrationEvent    `json:"recent_events"`
	Alerts           []Alert             `json:"alerts,omitempty"`
	EstimatedETA     time.Duration       `json:"estimated_eta,omitempty"`
}

// ProgressMetrics contains progress-related metrics
type ProgressMetrics struct {
	TotalOperations     int64   `json:"total_operations"`
	CompletedOperations int64   `json:"completed_operations"`
	FailedOperations    int64   `json:"failed_operations"`
	SuccessRate         float64 `json:"success_rate"`
	CompletionRate      float64 `json:"completion_rate"`
}

// PerformanceMetrics contains performance-related metrics
type PerformanceMetrics struct {
	OperationsPerSecond float64       `json:"operations_per_second"`
	AverageLatency      time.Duration `json:"average_latency"`
	P95Latency          time.Duration `json:"p95_latency"`
	P99Latency          time.Duration `json:"p99_latency"`
	MemoryUsage         int64         `json:"memory_usage"`
	QueueDepth          int64         `json:"queue_depth"`
}

// Alert represents an alert generated during migration
type Alert struct {
	Timestamp time.Time              `json:"timestamp"`
	Type      AlertType              `json:"type"`
	Severity  EventSeverity          `json:"severity"`
	Message   string                 `json:"message"`
	Details   map[string]interface{} `json:"details,omitempty"`
}

// AlertType represents the type of alert
type AlertType string

const (
	AlertTypeHighErrorRate   AlertType = "high_error_rate"
	AlertTypeSlowPerformance AlertType = "slow_performance"
	AlertTypeHighMemoryUsage AlertType = "high_memory_usage"
	AlertTypeHighQueueDepth  AlertType = "high_queue_depth"
	AlertTypeConnectionIssue AlertType = "connection_issue"
	AlertTypeDataIntegrity   AlertType = "data_integrity"
)

// DefaultMonitorConfig returns default monitoring configuration
func DefaultMonitorConfig() *MonitorConfig {
	return &MonitorConfig{
		ReportInterval:        time.Second * 30,
		MaxEvents:             1000,
		EnableRealTimeMetrics: true,
		MetricsInterval:       time.Second * 5,
		EnableEventLogging:    true,
		AlertThresholds: &AlertThresholds{
			ErrorRateThreshold:     5.0, // 5% error rate
			SlowOperationThreshold: time.Second * 30,
			MemoryUsageThreshold:   1024 * 1024 * 1024, // 1GB
			QueueDepthThreshold:    10000,
		},
	}
}

// NewMigrationMonitor creates a new migration monitor
func NewMigrationMonitor(config *MonitorConfig, logger Logger) *MigrationMonitor {
	if config == nil {
		config = DefaultMonitorConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	monitor := &MigrationMonitor{
		config:    config,
		logger:    logger,
		startTime: time.Now(),
		metrics:   &MigrationMetrics{},
		events:    make([]MigrationEvent, 0, config.MaxEvents),
		maxEvents: config.MaxEvents,
		status:    StatusIdle,
		ctx:       ctx,
		cancel:    cancel,
	}

	return monitor
}

// Start starts the migration monitor
func (mm *MigrationMonitor) Start() error {
	mm.statusLock.Lock()
	mm.status = StatusStarting
	mm.statusLock.Unlock()

	// Open report file if specified
	if mm.config.ReportFile != "" {
		file, err := os.Create(mm.config.ReportFile)
		if err != nil {
			return fmt.Errorf("failed to create report file: %w", err)
		}
		mm.reportFile = file
	}

	// Start background processes
	if mm.config.EnableRealTimeMetrics {
		mm.wg.Add(1)
		go mm.metricsCollector()
	}

	mm.wg.Add(1)
	go mm.reportGenerator()

	mm.wg.Add(1)
	go mm.alertMonitor()

	mm.statusLock.Lock()
	mm.status = StatusRunning
	mm.statusLock.Unlock()

	mm.logEvent(MigrationEvent{
		Timestamp: time.Now(),
		Type:      EventTypeStart,
		Severity:  SeverityInfo,
		Operation: "monitor",
		Message:   "Migration monitor started",
	})

	return nil
}

// Stop stops the migration monitor
func (mm *MigrationMonitor) Stop() {
	mm.statusLock.Lock()
	mm.status = StatusCompleting
	mm.statusLock.Unlock()

	mm.cancel()
	mm.wg.Wait()

	if mm.reportTicker != nil {
		mm.reportTicker.Stop()
	}

	if mm.reportFile != nil {
		mm.reportFile.Close()
	}

	mm.statusLock.Lock()
	mm.status = StatusCompleted
	mm.statusLock.Unlock()

	mm.logEvent(MigrationEvent{
		Timestamp: time.Now(),
		Type:      EventTypeComplete,
		Severity:  SeverityInfo,
		Operation: "monitor",
		Message:   "Migration monitor stopped",
	})
}

// SetCurrentOperation sets the current operation being monitored
func (mm *MigrationMonitor) SetCurrentOperation(operation string) {
	mm.currentOperation = operation
	mm.logEvent(MigrationEvent{
		Timestamp: time.Now(),
		Type:      EventTypeProgress,
		Severity:  SeverityInfo,
		Operation: operation,
		Message:   fmt.Sprintf("Started operation: %s", operation),
	})
}

// RecordOperationStart records the start of an operation
func (mm *MigrationMonitor) RecordOperationStart(operation string) {
	mm.totalOperations++
	mm.SetCurrentOperation(operation)
}

// RecordOperationComplete records the completion of an operation
func (mm *MigrationMonitor) RecordOperationComplete(operation string, duration time.Duration, err error) {
	mm.completedOps++

	if err != nil {
		mm.failedOps++
		mm.logEvent(MigrationEvent{
			Timestamp: time.Now(),
			Type:      EventTypeError,
			Severity:  SeverityError,
			Operation: operation,
			Message:   fmt.Sprintf("Operation failed: %v", err),
			Duration:  duration,
			Details: map[string]interface{}{
				"error": err.Error(),
			},
		})
	} else {
		mm.logEvent(MigrationEvent{
			Timestamp: time.Now(),
			Type:      EventTypeProgress,
			Severity:  SeverityInfo,
			Operation: operation,
			Message:   "Operation completed successfully",
			Duration:  duration,
		})
	}

	// Update metrics
	mm.metricsLock.Lock()
	mm.metrics.ProcessedPins = mm.completedOps
	mm.metrics.FailedPins = mm.failedOps
	mm.metrics.SuccessfulPins = mm.completedOps - mm.failedOps
	mm.metricsLock.Unlock()
}

// RecordMilestone records a significant milestone
func (mm *MigrationMonitor) RecordMilestone(message string, details map[string]interface{}) {
	mm.logEvent(MigrationEvent{
		Timestamp: time.Now(),
		Type:      EventTypeMilestone,
		Severity:  SeverityInfo,
		Operation: mm.currentOperation,
		Message:   message,
		Details:   details,
	})
}

// logEvent logs an event to the event history
func (mm *MigrationMonitor) logEvent(event MigrationEvent) {
	mm.eventsLock.Lock()
	defer mm.eventsLock.Unlock()

	// Add event to history
	mm.events = append(mm.events, event)

	// Trim events if we exceed the maximum
	if len(mm.events) > mm.maxEvents {
		// Keep the most recent events
		copy(mm.events, mm.events[len(mm.events)-mm.maxEvents:])
		mm.events = mm.events[:mm.maxEvents]
	}

	// Log to logger if enabled
	if mm.config.EnableEventLogging {
		switch event.Severity {
		case SeverityInfo:
			mm.logger.Infof("[%s] %s: %s", event.Type, event.Operation, event.Message)
		case SeverityWarning:
			mm.logger.Warnf("[%s] %s: %s", event.Type, event.Operation, event.Message)
		case SeverityError, SeverityCritical:
			mm.logger.Errorf("[%s] %s: %s", event.Type, event.Operation, event.Message)
		}
	}
}

// metricsCollector collects real-time metrics
func (mm *MigrationMonitor) metricsCollector() {
	defer mm.wg.Done()

	ticker := time.NewTicker(mm.config.MetricsInterval)
	defer ticker.Stop()

	for {
		select {
		case <-mm.ctx.Done():
			return
		case <-ticker.C:
			mm.collectMetrics()
		}
	}
}

// collectMetrics collects current metrics
func (mm *MigrationMonitor) collectMetrics() {
	mm.metricsLock.Lock()
	defer mm.metricsLock.Unlock()

	// Update timing metrics
	elapsed := time.Since(mm.startTime)
	mm.metrics.Duration = elapsed

	if elapsed > 0 && mm.completedOps > 0 {
		mm.metrics.PinsPerSecond = float64(mm.completedOps) / elapsed.Seconds()
	}

	// Calculate success rate
	if mm.totalOperations > 0 {
		successRate := float64(mm.completedOps-mm.failedOps) / float64(mm.totalOperations) * 100
		mm.metrics.SuccessfulPins = mm.completedOps - mm.failedOps
	}
}

// reportGenerator generates periodic progress reports
func (mm *MigrationMonitor) reportGenerator() {
	defer mm.wg.Done()

	mm.reportTicker = time.NewTicker(mm.config.ReportInterval)
	defer mm.reportTicker.Stop()

	for {
		select {
		case <-mm.ctx.Done():
			return
		case <-mm.reportTicker.C:
			mm.generateReport()
		}
	}
}

// generateReport generates and outputs a progress report
func (mm *MigrationMonitor) generateReport() {
	report := mm.GetProgressReport()

	// Output to logger
	mm.logger.Infof("Migration Progress: %.1f%% complete (%d/%d operations), %.2f ops/sec, %d errors",
		report.Progress.CompletionRate,
		report.Progress.CompletedOperations,
		report.Progress.TotalOperations,
		report.Performance.OperationsPerSecond,
		report.Progress.FailedOperations)

	// Write to report file if configured
	if mm.reportFile != nil {
		encoder := json.NewEncoder(mm.reportFile)
		if err := encoder.Encode(report); err != nil {
			mm.logger.Errorf("Failed to write progress report: %v", err)
		}
	}
}

// alertMonitor monitors for alert conditions
func (mm *MigrationMonitor) alertMonitor() {
	defer mm.wg.Done()

	ticker := time.NewTicker(time.Second * 10) // Check for alerts every 10 seconds
	defer ticker.Stop()

	for {
		select {
		case <-mm.ctx.Done():
			return
		case <-ticker.C:
			mm.checkAlerts()
		}
	}
}

// checkAlerts checks for alert conditions
func (mm *MigrationMonitor) checkAlerts() {
	if mm.config.AlertThresholds == nil {
		return
	}

	// Check error rate
	if mm.totalOperations > 0 {
		errorRate := float64(mm.failedOps) / float64(mm.totalOperations) * 100
		if errorRate > mm.config.AlertThresholds.ErrorRateThreshold {
			mm.generateAlert(AlertTypeHighErrorRate, SeverityWarning,
				fmt.Sprintf("High error rate detected: %.2f%%", errorRate),
				map[string]interface{}{
					"error_rate":        errorRate,
					"threshold":         mm.config.AlertThresholds.ErrorRateThreshold,
					"failed_operations": mm.failedOps,
					"total_operations":  mm.totalOperations,
				})
		}
	}

	// Additional alert checks would go here...
}

// generateAlert generates an alert
func (mm *MigrationMonitor) generateAlert(alertType AlertType, severity EventSeverity, message string, details map[string]interface{}) {
	alert := Alert{
		Timestamp: time.Now(),
		Type:      alertType,
		Severity:  severity,
		Message:   message,
		Details:   details,
	}

	// Log as event
	mm.logEvent(MigrationEvent{
		Timestamp: alert.Timestamp,
		Type:      EventTypeAlert,
		Severity:  severity,
		Operation: mm.currentOperation,
		Message:   fmt.Sprintf("ALERT [%s]: %s", alertType, message),
		Details:   details,
	})
}

// GetProgressReport returns a comprehensive progress report
func (mm *MigrationMonitor) GetProgressReport() *ProgressReport {
	mm.metricsLock.RLock()
	metrics := *mm.metrics
	mm.metricsLock.RUnlock()

	mm.statusLock.RLock()
	status := mm.status
	mm.statusLock.RUnlock()

	// Calculate progress metrics
	var completionRate, successRate float64
	if mm.totalOperations > 0 {
		completionRate = float64(mm.completedOps) / float64(mm.totalOperations) * 100
		successRate = float64(mm.completedOps-mm.failedOps) / float64(mm.totalOperations) * 100
	}

	progress := &ProgressMetrics{
		TotalOperations:     mm.totalOperations,
		CompletedOperations: mm.completedOps,
		FailedOperations:    mm.failedOps,
		SuccessRate:         successRate,
		CompletionRate:      completionRate,
	}

	// Calculate performance metrics
	performance := &PerformanceMetrics{
		OperationsPerSecond: metrics.PinsPerSecond,
		// Other performance metrics would be calculated here
	}

	// Get recent events
	mm.eventsLock.Lock()
	recentEvents := make([]MigrationEvent, 0)
	if len(mm.events) > 0 {
		// Get last 10 events
		start := len(mm.events) - 10
		if start < 0 {
			start = 0
		}
		recentEvents = append(recentEvents, mm.events[start:]...)
	}
	mm.eventsLock.Unlock()

	// Calculate ETA
	var eta time.Duration
	if mm.completedOps > 0 && mm.totalOperations > mm.completedOps {
		remaining := mm.totalOperations - mm.completedOps
		if metrics.PinsPerSecond > 0 {
			eta = time.Duration(float64(remaining)/metrics.PinsPerSecond) * time.Second
		}
	}

	return &ProgressReport{
		Timestamp:        time.Now(),
		Status:           status,
		CurrentOperation: mm.currentOperation,
		Progress:         progress,
		Performance:      performance,
		RecentEvents:     recentEvents,
		EstimatedETA:     eta,
	}
}

// GetCurrentMetrics returns current migration metrics
func (mm *MigrationMonitor) GetCurrentMetrics() *MigrationMetrics {
	mm.metricsLock.RLock()
	defer mm.metricsLock.RUnlock()

	metrics := *mm.metrics
	return &metrics
}

// GetRecentEvents returns recent migration events
func (mm *MigrationMonitor) GetRecentEvents(count int) []MigrationEvent {
	mm.eventsLock.Lock()
	defer mm.eventsLock.Unlock()

	if count <= 0 || count > len(mm.events) {
		count = len(mm.events)
	}

	if count == 0 {
		return []MigrationEvent{}
	}

	start := len(mm.events) - count
	events := make([]MigrationEvent, count)
	copy(events, mm.events[start:])

	return events
}

// GetStatus returns the current migration status
func (mm *MigrationMonitor) GetStatus() MigrationStatus {
	mm.statusLock.RLock()
	defer mm.statusLock.RUnlock()
	return mm.status
}
