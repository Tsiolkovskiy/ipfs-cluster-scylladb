package billing

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// MemoryBillingStorage implements BillingStorage using in-memory storage
type MemoryBillingStorage struct {
	usageEvents   []*UsageEvent
	usageMetrics  map[string]*UsageMetrics
	invoices      map[string]*Invoice
	payments      map[string]*Payment
	pricingTiers  map[string]*PricingTier
	mu            sync.RWMutex
}

// NewMemoryBillingStorage creates a new in-memory billing storage
func NewMemoryBillingStorage() *MemoryBillingStorage {
	storage := &MemoryBillingStorage{
		usageEvents:  make([]*UsageEvent, 0),
		usageMetrics: make(map[string]*UsageMetrics),
		invoices:     make(map[string]*Invoice),
		payments:     make(map[string]*Payment),
		pricingTiers: make(map[string]*PricingTier),
	}
	
	// Initialize with default pricing tier
	defaultTier := &PricingTier{
		ID:          "standard",
		Name:        "Standard",
		Description: "Standard pricing tier",
		Currency:    "USD",
		StoragePricing: &StoragePricing{
			PricePerGBMonth: 100, // $1.00 per GB-month
			MinimumCharge:   0,
			FreeQuotaGB:     1,   // 1GB free
			Model:           "per_gb_month",
		},
		BandwidthPricing: &BandwidthPricing{
			PricePerGB:  50, // $0.50 per GB
			FreeQuotaGB: 1,  // 1GB free
			Model:       "per_gb",
		},
		APIPricing: &APIPricing{
			PricePerThousand: 100, // $1.00 per 1000 calls
			FreeQuotaCalls:   1000, // 1000 calls free
			Model:            "per_thousand",
		},
		IsActive:  true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	
	storage.pricingTiers[defaultTier.ID] = defaultTier
	
	return storage
}

// StoreUsageEvent stores a usage event
func (m *MemoryBillingStorage) StoreUsageEvent(ctx context.Context, event *UsageEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	// Create a copy to avoid external modification
	eventCopy := *event
	m.usageEvents = append(m.usageEvents, &eventCopy)
	
	return nil
}

// GetUsageEvents retrieves usage events for a time period
func (m *MemoryBillingStorage) GetUsageEvents(ctx context.Context, ownerID string, period *TimePeriod) ([]*UsageEvent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	var events []*UsageEvent
	
	for _, event := range m.usageEvents {
		if event.OwnerID == ownerID &&
			event.Timestamp.After(period.StartTime) &&
			event.Timestamp.Before(period.EndTime) {
			
			// Create a copy to avoid external modification
			eventCopy := *event
			events = append(events, &eventCopy)
		}
	}
	
	return events, nil
}

// StoreUsageMetrics stores aggregated usage metrics
func (m *MemoryBillingStorage) StoreUsageMetrics(ctx context.Context, metrics *UsageMetrics) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	key := fmt.Sprintf("%s:%s:%s", metrics.OwnerID, metrics.Period.Type, 
		metrics.Period.StartTime.Format("2006-01-02"))
	
	// Create a copy to avoid external modification
	metricsCopy := *metrics
	m.usageMetrics[key] = &metricsCopy
	
	return nil
}

// GetUsageMetrics retrieves aggregated usage metrics
func (m *MemoryBillingStorage) GetUsageMetrics(ctx context.Context, ownerID string, period *TimePeriod) (*UsageMetrics, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	key := fmt.Sprintf("%s:%s:%s", ownerID, period.Type, 
		period.StartTime.Format("2006-01-02"))
	
	metrics, exists := m.usageMetrics[key]
	if !exists {
		return nil, nil // No metrics found
	}
	
	// Create a copy to avoid external modification
	metricsCopy := *metrics
	return &metricsCopy, nil
}

// StoreInvoice stores an invoice
func (m *MemoryBillingStorage) StoreInvoice(ctx context.Context, invoice *Invoice) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	// Create a copy to avoid external modification
	invoiceCopy := *invoice
	if invoice.Customer != nil {
		customerCopy := *invoice.Customer
		invoiceCopy.Customer = &customerCopy
	}
	if invoice.CostBreakdown != nil {
		breakdownCopy := *invoice.CostBreakdown
		invoiceCopy.CostBreakdown = &breakdownCopy
	}
	
	m.invoices[invoice.ID] = &invoiceCopy
	
	return nil
}

// GetInvoice retrieves an invoice by ID
func (m *MemoryBillingStorage) GetInvoice(ctx context.Context, invoiceID string) (*Invoice, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	invoice, exists := m.invoices[invoiceID]
	if !exists {
		return nil, fmt.Errorf("invoice not found: %s", invoiceID)
	}
	
	// Create a copy to avoid external modification
	invoiceCopy := *invoice
	if invoice.Customer != nil {
		customerCopy := *invoice.Customer
		invoiceCopy.Customer = &customerCopy
	}
	if invoice.CostBreakdown != nil {
		breakdownCopy := *invoice.CostBreakdown
		invoiceCopy.CostBreakdown = &breakdownCopy
	}
	
	return &invoiceCopy, nil
}

// GetInvoicesByOwner retrieves invoices for an owner
func (m *MemoryBillingStorage) GetInvoicesByOwner(ctx context.Context, ownerID string, period *TimePeriod) ([]*Invoice, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	var invoices []*Invoice
	
	for _, invoice := range m.invoices {
		if invoice.OwnerID == ownerID &&
			invoice.Period != nil &&
			invoice.Period.StartTime.After(period.StartTime) &&
			invoice.Period.EndTime.Before(period.EndTime) {
			
			// Create a copy to avoid external modification
			invoiceCopy := *invoice
			if invoice.Customer != nil {
				customerCopy := *invoice.Customer
				invoiceCopy.Customer = &customerCopy
			}
			if invoice.CostBreakdown != nil {
				breakdownCopy := *invoice.CostBreakdown
				invoiceCopy.CostBreakdown = &breakdownCopy
			}
			
			invoices = append(invoices, &invoiceCopy)
		}
	}
	
	return invoices, nil
}

// StorePayment stores a payment
func (m *MemoryBillingStorage) StorePayment(ctx context.Context, payment *Payment) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	// Create a copy to avoid external modification
	paymentCopy := *payment
	if payment.PaymentMethod != nil {
		methodCopy := *payment.PaymentMethod
		paymentCopy.PaymentMethod = &methodCopy
	}
	
	m.payments[payment.ID] = &paymentCopy
	
	return nil
}

// GetPayment retrieves a payment by ID
func (m *MemoryBillingStorage) GetPayment(ctx context.Context, paymentID string) (*Payment, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	payment, exists := m.payments[paymentID]
	if !exists {
		return nil, fmt.Errorf("payment not found: %s", paymentID)
	}
	
	// Create a copy to avoid external modification
	paymentCopy := *payment
	if payment.PaymentMethod != nil {
		methodCopy := *payment.PaymentMethod
		paymentCopy.PaymentMethod = &methodCopy
	}
	
	return &paymentCopy, nil
}

// StorePricingTier stores a pricing tier
func (m *MemoryBillingStorage) StorePricingTier(ctx context.Context, tier *PricingTier) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	// Create a copy to avoid external modification
	tierCopy := *tier
	if tier.StoragePricing != nil {
		storageCopy := *tier.StoragePricing
		tierCopy.StoragePricing = &storageCopy
	}
	if tier.BandwidthPricing != nil {
		bandwidthCopy := *tier.BandwidthPricing
		tierCopy.BandwidthPricing = &bandwidthCopy
	}
	if tier.APIPricing != nil {
		apiCopy := *tier.APIPricing
		tierCopy.APIPricing = &apiCopy
	}
	
	m.pricingTiers[tier.ID] = &tierCopy
	
	return nil
}

// GetPricingTier retrieves a pricing tier by ID
func (m *MemoryBillingStorage) GetPricingTier(ctx context.Context, tierID string) (*PricingTier, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	tier, exists := m.pricingTiers[tierID]
	if !exists {
		return nil, fmt.Errorf("pricing tier not found: %s", tierID)
	}
	
	// Create a copy to avoid external modification
	tierCopy := *tier
	if tier.StoragePricing != nil {
		storageCopy := *tier.StoragePricing
		tierCopy.StoragePricing = &storageCopy
	}
	if tier.BandwidthPricing != nil {
		bandwidthCopy := *tier.BandwidthPricing
		tierCopy.BandwidthPricing = &bandwidthCopy
	}
	if tier.APIPricing != nil {
		apiCopy := *tier.APIPricing
		tierCopy.APIPricing = &apiCopy
	}
	
	return &tierCopy, nil
}

// GetPricingTiers retrieves all pricing tiers
func (m *MemoryBillingStorage) GetPricingTiers(ctx context.Context) ([]*PricingTier, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	var tiers []*PricingTier
	
	for _, tier := range m.pricingTiers {
		// Create a copy to avoid external modification
		tierCopy := *tier
		if tier.StoragePricing != nil {
			storageCopy := *tier.StoragePricing
			tierCopy.StoragePricing = &storageCopy
		}
		if tier.BandwidthPricing != nil {
			bandwidthCopy := *tier.BandwidthPricing
			tierCopy.BandwidthPricing = &bandwidthCopy
		}
		if tier.APIPricing != nil {
			apiCopy := *tier.APIPricing
			tierCopy.APIPricing = &apiCopy
		}
		
		tiers = append(tiers, &tierCopy)
	}
	
	return tiers, nil
}

// GetAllUsageEvents returns all stored usage events (for testing)
func (m *MemoryBillingStorage) GetAllUsageEvents() []*UsageEvent {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	events := make([]*UsageEvent, len(m.usageEvents))
	for i, event := range m.usageEvents {
		eventCopy := *event
		events[i] = &eventCopy
	}
	
	return events
}

// GetAllUsageMetrics returns all stored usage metrics (for testing)
func (m *MemoryBillingStorage) GetAllUsageMetrics() map[string]*UsageMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	metrics := make(map[string]*UsageMetrics)
	for key, metric := range m.usageMetrics {
		metricCopy := *metric
		metrics[key] = &metricCopy
	}
	
	return metrics
}

// GetAllInvoices returns all stored invoices (for testing)
func (m *MemoryBillingStorage) GetAllInvoices() map[string]*Invoice {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	invoices := make(map[string]*Invoice)
	for id, invoice := range m.invoices {
		invoiceCopy := *invoice
		if invoice.Customer != nil {
			customerCopy := *invoice.Customer
			invoiceCopy.Customer = &customerCopy
		}
		if invoice.CostBreakdown != nil {
			breakdownCopy := *invoice.CostBreakdown
			invoiceCopy.CostBreakdown = &breakdownCopy
		}
		invoices[id] = &invoiceCopy
	}
	
	return invoices
}

// GetAllPayments returns all stored payments (for testing)
func (m *MemoryBillingStorage) GetAllPayments() map[string]*Payment {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	payments := make(map[string]*Payment)
	for id, payment := range m.payments {
		paymentCopy := *payment
		if payment.PaymentMethod != nil {
			methodCopy := *payment.PaymentMethod
			paymentCopy.PaymentMethod = &methodCopy
		}
		payments[id] = &paymentCopy
	}
	
	return payments
}

// GetAllPricingTiers returns all stored pricing tiers (for testing)
func (m *MemoryBillingStorage) GetAllPricingTiers() map[string]*PricingTier {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	tiers := make(map[string]*PricingTier)
	for id, tier := range m.pricingTiers {
		tierCopy := *tier
		if tier.StoragePricing != nil {
			storageCopy := *tier.StoragePricing
			tierCopy.StoragePricing = &storageCopy
		}
		if tier.BandwidthPricing != nil {
			bandwidthCopy := *tier.BandwidthPricing
			tierCopy.BandwidthPricing = &bandwidthCopy
		}
		if tier.APIPricing != nil {
			apiCopy := *tier.APIPricing
			tierCopy.APIPricing = &apiCopy
		}
		tiers[id] = &tierCopy
	}
	
	return tiers
}

// Clear clears all stored data (for testing)
func (m *MemoryBillingStorage) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.usageEvents = m.usageEvents[:0]
	m.usageMetrics = make(map[string]*UsageMetrics)
	m.invoices = make(map[string]*Invoice)
	m.payments = make(map[string]*Payment)
	m.pricingTiers = make(map[string]*PricingTier)
}