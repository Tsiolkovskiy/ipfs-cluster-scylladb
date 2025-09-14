package billing

import (
	"context"
	"fmt"
	"time"
)

// UsageCollectorImpl implements the UsageCollector interface
type UsageCollectorImpl struct {
	storage BillingStorage
	config  *Config
}

// NewUsageCollector creates a new usage collector
func NewUsageCollector(storage BillingStorage, config *Config) UsageCollector {
	return &UsageCollectorImpl{
		storage: storage,
		config:  config,
	}
}

// CollectPinUsage collects usage data from pin operations
func (u *UsageCollectorImpl) CollectPinUsage(ctx context.Context, pin Pin, operation UsageOperation) error {
	// Calculate pin size
	size := int64(1024 * 1024) // 1MB placeholder
	
	// Create usage event
	event := &UsageEvent{
		ID:         generateEventID(),
		OwnerID:    getOwnerFromPin(pin),
		TenantID:   getTenantFromPin(pin),
		Operation:  operation,
		ResourceID: pin.CID,
		Quantity:   size,
		Unit:       "bytes",
		Timestamp:  time.Now(),
		Metadata:   extractPinMetadata(pin),
	}
	
	// Store usage event
	if err := u.storage.StoreUsageEvent(ctx, event); err != nil {
		return fmt.Errorf("failed to store usage event: %w", err)
	}
	
	logger.Debugf("Collected pin usage: owner=%s, operation=%s, cid=%s, size=%d", 
		event.OwnerID, operation, pin.CID, size)
	
	return nil
}

// AggregateUsage aggregates usage data for a billing period
func (u *UsageCollectorImpl) AggregateUsage(ctx context.Context, ownerID string, period *TimePeriod) (*UsageMetrics, error) {
	// Get usage events for the period
	events, err := u.storage.GetUsageEvents(ctx, ownerID, period)
	if err != nil {
		return nil, fmt.Errorf("failed to get usage events: %w", err)
	}
	
	// Initialize metrics
	metrics := &UsageMetrics{
		OwnerID:         ownerID,
		Period:          period,
		StorageBytes:    0,
		StorageHours:    0,
		BandwidthBytes:  0,
		APICalls:        0,
		PinOperations:   0,
		UniqueObjects:   0,
		Metadata:        make(map[string]string),
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	
	// Track unique objects
	uniqueObjects := make(map[string]bool)
	
	// Aggregate events
	for _, event := range events {
		// Set tenant ID from first event
		if metrics.TenantID == "" {
			metrics.TenantID = event.TenantID
		}
		
		switch event.Operation {
		case UsageOperationPinAdd:
			metrics.PinOperations++
			metrics.StorageBytes += event.Quantity
			uniqueObjects[event.ResourceID] = true
			
		case UsageOperationPinRemove:
			metrics.PinOperations++
			// Note: We don't subtract storage bytes here as we track cumulative usage
			
		case UsageOperationPinUpdate:
			metrics.PinOperations++
			
		case UsageOperationStorage:
			metrics.StorageBytes += event.Quantity
			
		case UsageOperationBandwidth:
			metrics.BandwidthBytes += event.Quantity
			
		case UsageOperationAPI:
			metrics.APICalls += event.Quantity
		}
	}
	
	metrics.UniqueObjects = int64(len(uniqueObjects))
	
	// Calculate storage hours
	// This is a simplified calculation - in reality you'd track storage over time
	periodHours := int64(period.EndTime.Sub(period.StartTime).Hours())
	metrics.StorageHours = metrics.StorageBytes * periodHours / (1024 * 1024 * 1024) // GB-hours
	
	return metrics, nil
}

// PerformDailyRollup performs daily aggregation of usage data
func (u *UsageCollectorImpl) PerformDailyRollup(ctx context.Context, date time.Time) error {
	logger.Infof("Starting daily rollup for date: %s", date.Format("2006-01-02"))
	
	// Define the daily period
	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	endOfDay := startOfDay.Add(24 * time.Hour).Add(-time.Nanosecond)
	
	period := &TimePeriod{
		StartTime: startOfDay,
		EndTime:   endOfDay,
		Type:      "daily",
	}
	
	// Get all unique owner IDs for the day
	// In a real implementation, you would have a more efficient way to get this
	ownerIDs, err := u.getUniqueOwnerIDsForPeriod(ctx, period)
	if err != nil {
		return fmt.Errorf("failed to get unique owner IDs: %w", err)
	}
	
	// Process each owner
	for _, ownerID := range ownerIDs {
		if err := u.performOwnerDailyRollup(ctx, ownerID, period); err != nil {
			logger.Errorf("Failed to perform daily rollup for owner %s: %v", ownerID, err)
			continue
		}
	}
	
	logger.Infof("Completed daily rollup for date: %s, processed %d owners", 
		date.Format("2006-01-02"), len(ownerIDs))
	
	return nil
}

// performOwnerDailyRollup performs daily rollup for a specific owner
func (u *UsageCollectorImpl) performOwnerDailyRollup(ctx context.Context, ownerID string, period *TimePeriod) error {
	// Check if rollup already exists
	existingMetrics, err := u.storage.GetUsageMetrics(ctx, ownerID, period)
	if err == nil && existingMetrics != nil {
		logger.Debugf("Daily rollup already exists for owner %s on %s", 
			ownerID, period.StartTime.Format("2006-01-02"))
		return nil
	}
	
	// Aggregate usage for the day
	metrics, err := u.AggregateUsage(ctx, ownerID, period)
	if err != nil {
		return fmt.Errorf("failed to aggregate usage: %w", err)
	}
	
	// Store aggregated metrics
	if err := u.storage.StoreUsageMetrics(ctx, metrics); err != nil {
		return fmt.Errorf("failed to store usage metrics: %w", err)
	}
	
	logger.Debugf("Completed daily rollup for owner %s: storage=%d bytes, operations=%d", 
		ownerID, metrics.StorageBytes, metrics.PinOperations)
	
	return nil
}

// getUniqueOwnerIDsForPeriod gets unique owner IDs that have usage events in a period
func (u *UsageCollectorImpl) getUniqueOwnerIDsForPeriod(ctx context.Context, period *TimePeriod) ([]string, error) {
	// This is a placeholder implementation
	// In a real system, you would have an efficient way to get unique owner IDs
	// For example, by maintaining a separate index or using ScyllaDB's token() function
	
	// For now, return a hardcoded list for testing
	return []string{"owner1", "owner2", "owner3"}, nil
}

// getOwnerFromPin extracts owner ID from pin metadata
func getOwnerFromPin(pin Pin) string {
	// Check pin metadata for owner information
	if pin.Metadata != nil {
		if owner, exists := pin.Metadata["owner"]; exists {
			return owner
		}
		if owner, exists := pin.Metadata["user_id"]; exists {
			return owner
		}
	}
	
	// Default owner if not specified
	return "default"
}

// getTenantFromPin extracts tenant ID from pin metadata
func getTenantFromPin(pin Pin) string {
	// Check pin metadata for tenant information
	if pin.Metadata != nil {
		if tenant, exists := pin.Metadata["tenant"]; exists {
			return tenant
		}
		if tenant, exists := pin.Metadata["tenant_id"]; exists {
			return tenant
		}
	}
	
	// Default tenant if not specified
	return "default"
}

// extractPinMetadata extracts relevant metadata from pin for billing
func extractPinMetadata(pin Pin) map[string]string {
	metadata := make(map[string]string)
	
	// Copy relevant metadata
	if pin.Metadata != nil {
		for key, value := range pin.Metadata {
			switch key {
			case "project", "department", "cost_center", "environment":
				metadata[key] = value
			}
		}
	}
	
	// Add pin-specific metadata
	metadata["pin_type"] = pin.Type
	
	return metadata
}

// MemoryUsageCollector is an in-memory implementation for testing
type MemoryUsageCollector struct {
	events  []*UsageEvent
	metrics map[string]*UsageMetrics
}

// NewMemoryUsageCollector creates a new in-memory usage collector
func NewMemoryUsageCollector() *MemoryUsageCollector {
	return &MemoryUsageCollector{
		events:  make([]*UsageEvent, 0),
		metrics: make(map[string]*UsageMetrics),
	}
}

// CollectPinUsage implements UsageCollector interface
func (m *MemoryUsageCollector) CollectPinUsage(ctx context.Context, pin Pin, operation UsageOperation) error {
	event := &UsageEvent{
		ID:         generateEventID(),
		OwnerID:    getOwnerFromPin(pin),
		TenantID:   getTenantFromPin(pin),
		Operation:  operation,
		ResourceID: pin.CID,
		Quantity:   1024 * 1024, // 1MB placeholder
		Unit:       "bytes",
		Timestamp:  time.Now(),
		Metadata:   extractPinMetadata(pin),
	}
	
	m.events = append(m.events, event)
	return nil
}

// AggregateUsage implements UsageCollector interface
func (m *MemoryUsageCollector) AggregateUsage(ctx context.Context, ownerID string, period *TimePeriod) (*UsageMetrics, error) {
	// Filter events for owner and period
	var filteredEvents []*UsageEvent
	for _, event := range m.events {
		if event.OwnerID == ownerID && 
		   event.Timestamp.After(period.StartTime) && 
		   event.Timestamp.Before(period.EndTime) {
			filteredEvents = append(filteredEvents, event)
		}
	}
	
	// Aggregate metrics
	metrics := &UsageMetrics{
		OwnerID:       ownerID,
		Period:        period,
		StorageBytes:  0,
		PinOperations: int64(len(filteredEvents)),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	
	uniqueObjects := make(map[string]bool)
	for _, event := range filteredEvents {
		if metrics.TenantID == "" {
			metrics.TenantID = event.TenantID
		}
		
		metrics.StorageBytes += event.Quantity
		uniqueObjects[event.ResourceID] = true
	}
	
	metrics.UniqueObjects = int64(len(uniqueObjects))
	
	return metrics, nil
}

// PerformDailyRollup implements UsageCollector interface
func (m *MemoryUsageCollector) PerformDailyRollup(ctx context.Context, date time.Time) error {
	// Store aggregated metrics for the day
	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	endOfDay := startOfDay.Add(24 * time.Hour).Add(-time.Nanosecond)
	
	period := &TimePeriod{
		StartTime: startOfDay,
		EndTime:   endOfDay,
		Type:      "daily",
	}
	
	// Get unique owner IDs
	ownerIDs := make(map[string]bool)
	for _, event := range m.events {
		if event.Timestamp.After(period.StartTime) && event.Timestamp.Before(period.EndTime) {
			ownerIDs[event.OwnerID] = true
		}
	}
	
	// Aggregate for each owner
	for ownerID := range ownerIDs {
		metrics, err := m.AggregateUsage(ctx, ownerID, period)
		if err != nil {
			return err
		}
		
		key := fmt.Sprintf("%s:%s", ownerID, period.StartTime.Format("2006-01-02"))
		m.metrics[key] = metrics
	}
	
	return nil
}

// GetEvents returns all stored events (for testing)
func (m *MemoryUsageCollector) GetEvents() []*UsageEvent {
	return m.events
}

// GetMetrics returns all stored metrics (for testing)
func (m *MemoryUsageCollector) GetMetrics() map[string]*UsageMetrics {
	return m.metrics
}