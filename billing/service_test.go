package billing

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()
	
	assert.False(t, config.Enabled)
	assert.Equal(t, "USD", config.DefaultCurrency)
	assert.Equal(t, 30, config.InvoiceDueDays)
	assert.True(t, config.UsageCollectionEnabled)
	assert.Equal(t, time.Hour, config.UsageAggregationPeriod)
	assert.Equal(t, "02:00", config.DailyRollupTime)
	assert.Equal(t, "standard", config.DefaultPricingTierID)
	assert.False(t, config.TaxCalculationEnabled)
	assert.True(t, config.InvoiceGenerationEnabled)
	assert.Equal(t, "INV-", config.InvoiceNumberPrefix)
	assert.False(t, config.AutoSendInvoices)
	assert.False(t, config.PaymentProcessingEnabled)
	assert.Contains(t, config.SupportedPaymentMethods, "stripe")
	assert.Contains(t, config.SupportedPaymentMethods, "filecoin")
}

func TestNewService_Disabled(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = false
	
	storage := NewMemoryBillingStorage()
	service, err := NewService(context.Background(), config, storage)
	require.NoError(t, err)
	assert.NotNil(t, service)
	assert.False(t, service.IsEnabled())
}

func TestNewService_Enabled(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	config.NATSEnabled = false // Disable NATS for testing
	
	storage := NewMemoryBillingStorage()
	service, err := NewService(context.Background(), config, storage)
	require.NoError(t, err)
	assert.NotNil(t, service)
	assert.True(t, service.IsEnabled())
}

func TestRecordPinUsage(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	config.NATSEnabled = false
	
	storage := NewMemoryBillingStorage()
	service, err := NewService(context.Background(), config, storage)
	require.NoError(t, err)
	
	// Create pin usage event
	event := &PinUsageEvent{
		EventID:   "test-event-1",
		OwnerID:   "user-123",
		TenantID:  "tenant-456",
		CID:       "QmTest123",
		Operation: UsageOperationPinAdd,
		Size:      1024 * 1024, // 1MB
		Timestamp: time.Now(),
		Metadata: map[string]string{
			"project": "test-project",
		},
	}
	
	// Record usage
	err = service.RecordPinUsage(context.Background(), event)
	assert.NoError(t, err)
	
	// Verify usage was stored
	events := storage.GetAllUsageEvents()
	assert.Len(t, events, 1)
	assert.Equal(t, event.OwnerID, events[0].OwnerID)
	assert.Equal(t, event.Operation, events[0].Operation)
}

func TestGetUsageMetrics(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	config.NATSEnabled = false
	
	storage := NewMemoryBillingStorage()
	service, err := NewService(context.Background(), config, storage)
	require.NoError(t, err)
	
	// Store some usage events
	ownerID := "user-123"
	period := &TimePeriod{
		StartTime: time.Now().Add(-24 * time.Hour),
		EndTime:   time.Now(),
		Type:      "daily",
	}
	
	events := []*UsageEvent{
		{
			ID:         "event-1",
			OwnerID:    ownerID,
			TenantID:   "tenant-456",
			Operation:  UsageOperationPinAdd,
			ResourceID: "QmTest1",
			Quantity:   1024 * 1024,
			Unit:       "bytes",
			Timestamp:  time.Now().Add(-12 * time.Hour),
		},
		{
			ID:         "event-2",
			OwnerID:    ownerID,
			TenantID:   "tenant-456",
			Operation:  UsageOperationPinAdd,
			ResourceID: "QmTest2",
			Quantity:   2 * 1024 * 1024,
			Unit:       "bytes",
			Timestamp:  time.Now().Add(-6 * time.Hour),
		},
	}
	
	for _, event := range events {
		err := storage.StoreUsageEvent(context.Background(), event)
		require.NoError(t, err)
	}
	
	// Get usage metrics
	req := &UsageMetricsRequest{
		OwnerID: ownerID,
		Period:  period,
	}
	
	metrics, err := service.GetUsageMetrics(context.Background(), req)
	require.NoError(t, err)
	assert.NotNil(t, metrics)
	assert.Equal(t, ownerID, metrics.OwnerID)
	assert.Equal(t, int64(2), metrics.PinOperations)
	assert.Equal(t, int64(3*1024*1024), metrics.StorageBytes)
}

func TestCalculateCosts(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	config.NATSEnabled = false
	
	storage := NewMemoryBillingStorage()
	service, err := NewService(context.Background(), config, storage)
	require.NoError(t, err)
	
	// Create usage metrics
	usage := &UsageMetrics{
		OwnerID:         "user-123",
		TenantID:        "tenant-456",
		StorageBytes:    1024 * 1024 * 1024, // 1GB
		BandwidthBytes:  512 * 1024 * 1024,  // 512MB
		APICalls:        1000,
		PinOperations:   50,
		UniqueObjects:   25,
	}
	
	// Create pricing tier
	pricingTier := &PricingTier{
		ID:       "test-tier",
		Currency: "USD",
		StoragePricing: &StoragePricing{
			PricePerGBMonth: 100, // $1.00 per GB-month
			FreeQuotaGB:     0,
		},
		BandwidthPricing: &BandwidthPricing{
			PricePerGB:  50, // $0.50 per GB
			FreeQuotaGB: 0,
		},
		APIPricing: &APIPricing{
			PricePerThousand: 100, // $1.00 per 1000 calls
			FreeQuotaCalls:   0,
		},
	}
	
	// Calculate costs
	req := &CostCalculationRequest{
		OwnerID:     usage.OwnerID,
		TenantID:    usage.TenantID,
		Usage:       usage,
		PricingTier: pricingTier,
	}
	
	calculation, err := service.CalculateCosts(context.Background(), req)
	require.NoError(t, err)
	assert.NotNil(t, calculation)
	assert.Equal(t, "USD", calculation.Currency)
	assert.Greater(t, calculation.TotalAmount, int64(0))
}

func TestGenerateInvoice(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	config.NATSEnabled = false
	
	storage := NewMemoryBillingStorage()
	service, err := NewService(context.Background(), config, storage)
	require.NoError(t, err)
	
	// Create usage metrics
	usage := &UsageMetrics{
		OwnerID:         "user-123",
		TenantID:        "tenant-456",
		StorageBytes:    1024 * 1024 * 1024,
		BandwidthBytes:  512 * 1024 * 1024,
		APICalls:        1000,
		PinOperations:   50,
	}
	
	// Create invoice request
	req := &InvoiceRequest{
		OwnerID:  usage.OwnerID,
		TenantID: usage.TenantID,
		Period: &TimePeriod{
			StartTime: time.Now().Add(-30 * 24 * time.Hour),
			EndTime:   time.Now(),
			Type:      "monthly",
		},
		Usage:   usage,
		DueDate: time.Now().Add(30 * 24 * time.Hour),
		Notes:   "Test invoice",
	}
	
	// Generate invoice
	invoice, err := service.GenerateInvoice(context.Background(), req)
	require.NoError(t, err)
	assert.NotNil(t, invoice)
	assert.Equal(t, usage.OwnerID, invoice.OwnerID)
	assert.Equal(t, InvoiceStatusDraft, invoice.Status)
	assert.Greater(t, invoice.TotalAmount, int64(0))
	assert.Contains(t, invoice.Number, config.InvoiceNumberPrefix)
}

func TestGetInvoices(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	config.NATSEnabled = false
	
	storage := NewMemoryBillingStorage()
	service, err := NewService(context.Background(), config, storage)
	require.NoError(t, err)
	
	ownerID := "user-123"
	
	// Store some invoices
	invoices := []*Invoice{
		{
			ID:          "inv-1",
			OwnerID:     ownerID,
			TenantID:    "tenant-456",
			Number:      "INV-001",
			TotalAmount: 1000,
			Currency:    "USD",
			Status:      InvoiceStatusSent,
			Period: &TimePeriod{
				StartTime: time.Now().Add(-60 * 24 * time.Hour),
				EndTime:   time.Now().Add(-30 * 24 * time.Hour),
			},
			CreatedAt: time.Now().Add(-30 * 24 * time.Hour),
		},
		{
			ID:          "inv-2",
			OwnerID:     ownerID,
			TenantID:    "tenant-456",
			Number:      "INV-002",
			TotalAmount: 1500,
			Currency:    "USD",
			Status:      InvoiceStatusPaid,
			Period: &TimePeriod{
				StartTime: time.Now().Add(-30 * 24 * time.Hour),
				EndTime:   time.Now(),
			},
			CreatedAt: time.Now().Add(-15 * 24 * time.Hour),
		},
	}
	
	for _, invoice := range invoices {
		err := storage.StoreInvoice(context.Background(), invoice)
		require.NoError(t, err)
	}
	
	// Get invoices
	period := &TimePeriod{
		StartTime: time.Now().Add(-90 * 24 * time.Hour),
		EndTime:   time.Now(),
	}
	
	retrievedInvoices, err := service.GetInvoices(context.Background(), ownerID, period)
	require.NoError(t, err)
	assert.Len(t, retrievedInvoices, 2)
}

func TestProcessBillingEvent(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	config.NATSEnabled = false
	
	storage := NewMemoryBillingStorage()
	service, err := NewService(context.Background(), config, storage)
	require.NoError(t, err)
	
	// Create billing event
	event := &BillingEvent{
		ID:       "event-123",
		Type:     "usage",
		OwnerID:  "user-123",
		TenantID: "tenant-456",
		Data: map[string]interface{}{
			"usage_event": &UsageEvent{
				ID:         "usage-1",
				OwnerID:    "user-123",
				Operation:  UsageOperationPinAdd,
				ResourceID: "QmTest",
				Quantity:   1024,
			},
		},
		Timestamp: time.Now(),
	}
	
	// Process event
	err = service.ProcessBillingEvent(context.Background(), event)
	assert.NoError(t, err)
}

func TestUpdatePricingTier(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	config.NATSEnabled = false
	
	storage := NewMemoryBillingStorage()
	service, err := NewService(context.Background(), config, storage)
	require.NoError(t, err)
	
	// Create pricing tier
	tier := &PricingTier{
		ID:          "test-tier",
		Name:        "Test Tier",
		Description: "Test pricing tier",
		Currency:    "USD",
		StoragePricing: &StoragePricing{
			PricePerGBMonth: 100,
			FreeQuotaGB:     1,
		},
		IsActive:  true,
		CreatedAt: time.Now(),
	}
	
	// Update pricing tier
	err = service.UpdatePricingTier(context.Background(), tier)
	assert.NoError(t, err)
	
	// Verify tier was stored
	tiers, err := service.GetPricingTiers(context.Background())
	require.NoError(t, err)
	assert.Len(t, tiers, 1)
	assert.Equal(t, tier.ID, tiers[0].ID)
	assert.Equal(t, tier.Name, tiers[0].Name)
}

func TestServiceDisabled(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = false
	
	storage := NewMemoryBillingStorage()
	service, err := NewService(context.Background(), config, storage)
	require.NoError(t, err)
	
	// Test that operations return appropriate errors when disabled
	event := &PinUsageEvent{
		OwnerID:   "user-123",
		Operation: UsageOperationPinAdd,
	}
	
	err = service.RecordPinUsage(context.Background(), event)
	assert.NoError(t, err) // Should not error, just do nothing
	
	_, err = service.GetUsageMetrics(context.Background(), &UsageMetricsRequest{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "billing service is disabled")
	
	_, err = service.CalculateCosts(context.Background(), &CostCalculationRequest{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "billing service is disabled")
}