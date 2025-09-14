package billing

import (
	"context"
	"fmt"
	"time"
)

// InvoiceEmitterImpl implements the InvoiceEmitter interface
type InvoiceEmitterImpl struct {
	storage BillingStorage
	config  *Config
}

// NewInvoiceEmitter creates a new invoice emitter
func NewInvoiceEmitter(storage BillingStorage, config *Config) InvoiceEmitter {
	return &InvoiceEmitterImpl{
		storage: storage,
		config:  config,
	}
}

// EmitInvoice generates an invoice from cost breakdown and customer information
func (i *InvoiceEmitterImpl) EmitInvoice(ctx context.Context, costs *CostBreakdown, customer *Customer) (*Invoice, error) {
	// Generate invoice ID
	invoiceID := generateInvoiceID()
	
	// Create invoice
	invoice := &Invoice{
		ID:            invoiceID,
		OwnerID:       customer.OwnerID,
		TenantID:      customer.TenantID,
		Customer:      customer,
		CostBreakdown: costs,
		TotalAmount:   costs.Total,
		Currency:      costs.Currency,
		Status:        InvoiceStatusDraft,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Metadata:      make(map[string]string),
	}
	
	// Set payment method if customer has one
	if customer.PaymentMethod != nil {
		invoice.PaymentMethod = customer.PaymentMethod
	}
	
	return invoice, nil
}

// SendInvoiceNotification sends invoice notification to customer
func (i *InvoiceEmitterImpl) SendInvoiceNotification(ctx context.Context, invoice *Invoice) error {
	// In a real implementation, this would send email, SMS, or other notifications
	logger.Infof("Sending invoice notification for invoice %s to customer %s (%s)", 
		invoice.Number, invoice.Customer.Name, invoice.Customer.Email)
	
	// Placeholder implementation
	// You would integrate with email service, notification service, etc.
	
	return nil
}

// UpdateInvoiceStatus updates the status of an invoice
func (i *InvoiceEmitterImpl) UpdateInvoiceStatus(ctx context.Context, invoiceID string, status InvoiceStatus) error {
	// Get existing invoice
	invoice, err := i.storage.GetInvoice(ctx, invoiceID)
	if err != nil {
		return fmt.Errorf("failed to get invoice: %w", err)
	}
	
	// Update status and timestamp
	invoice.Status = status
	invoice.UpdatedAt = time.Now()
	
	// Set paid timestamp if status is paid
	if status == InvoiceStatusPaid && invoice.PaidAt == nil {
		now := time.Now()
		invoice.PaidAt = &now
	}
	
	// Store updated invoice
	if err := i.storage.StoreInvoice(ctx, invoice); err != nil {
		return fmt.Errorf("failed to store updated invoice: %w", err)
	}
	
	logger.Infof("Updated invoice %s status to %s", invoiceID, status)
	
	return nil
}

// generateInvoiceID generates a unique invoice ID
func generateInvoiceID() string {
	return fmt.Sprintf("inv_%d", time.Now().UnixNano())
}

// MemoryInvoiceEmitter is an in-memory implementation for testing
type MemoryInvoiceEmitter struct {
	invoices map[string]*Invoice
	config   *Config
}

// NewMemoryInvoiceEmitter creates a new in-memory invoice emitter
func NewMemoryInvoiceEmitter(config *Config) *MemoryInvoiceEmitter {
	return &MemoryInvoiceEmitter{
		invoices: make(map[string]*Invoice),
		config:   config,
	}
}

// EmitInvoice implements InvoiceEmitter interface
func (m *MemoryInvoiceEmitter) EmitInvoice(ctx context.Context, costs *CostBreakdown, customer *Customer) (*Invoice, error) {
	invoiceID := generateInvoiceID()
	
	invoice := &Invoice{
		ID:            invoiceID,
		OwnerID:       customer.OwnerID,
		TenantID:      customer.TenantID,
		Customer:      customer,
		CostBreakdown: costs,
		TotalAmount:   costs.Total,
		Currency:      costs.Currency,
		Status:        InvoiceStatusDraft,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Metadata:      make(map[string]string),
	}
	
	m.invoices[invoiceID] = invoice
	return invoice, nil
}

// SendInvoiceNotification implements InvoiceEmitter interface
func (m *MemoryInvoiceEmitter) SendInvoiceNotification(ctx context.Context, invoice *Invoice) error {
	// Simulate sending notification
	logger.Infof("Mock: Sent invoice notification for %s", invoice.ID)
	return nil
}

// UpdateInvoiceStatus implements InvoiceEmitter interface
func (m *MemoryInvoiceEmitter) UpdateInvoiceStatus(ctx context.Context, invoiceID string, status InvoiceStatus) error {
	invoice, exists := m.invoices[invoiceID]
	if !exists {
		return fmt.Errorf("invoice not found: %s", invoiceID)
	}
	
	invoice.Status = status
	invoice.UpdatedAt = time.Now()
	
	if status == InvoiceStatusPaid && invoice.PaidAt == nil {
		now := time.Now()
		invoice.PaidAt = &now
	}
	
	return nil
}

// GetInvoices returns all stored invoices (for testing)
func (m *MemoryInvoiceEmitter) GetInvoices() map[string]*Invoice {
	return m.invoices
}