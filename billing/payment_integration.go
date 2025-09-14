package billing

import (
	"context"
	"fmt"
	"sync"
	"time"

	logging "github.com/ipfs/go-log/v2"
)

var integrationLogger = logging.Logger("billing-integration")

// PaymentIntegrationService manages payment integrations and coordination
type PaymentIntegrationService struct {
	config           *Config
	paymentProcessor PaymentProcessor
	storage          BillingStorage
	
	// Payment monitoring
	pendingPayments  map[string]*PendingPayment
	paymentMutex     sync.RWMutex
	
	// Background services
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// PendingPayment tracks payments that are waiting for confirmation
type PendingPayment struct {
	PaymentID     string
	InvoiceID     string
	Provider      string
	Amount        int64
	Currency      string
	Status        PaymentStatus
	CreatedAt     time.Time
	LastCheckedAt time.Time
	RetryCount    int
	MaxRetries    int
	TimeoutAt     time.Time
}

// PaymentIntegrationConfig holds configuration for payment integrations
type PaymentIntegrationConfig struct {
	// Monitoring settings
	PaymentCheckInterval    time.Duration `json:"payment_check_interval"`
	PaymentTimeout          time.Duration `json:"payment_timeout"`
	MaxRetries              int           `json:"max_retries"`
	
	// Webhook settings
	WebhookEndpoint         string        `json:"webhook_endpoint"`
	WebhookSecret           string        `json:"webhook_secret"`
	
	// Notification settings
	NotificationEnabled     bool          `json:"notification_enabled"`
	NotificationWebhookURL  string        `json:"notification_webhook_url"`
	
	// Auto-processing settings
	AutoProcessPayments     bool          `json:"auto_process_payments"`
	AutoRefundFailedPayments bool         `json:"auto_refund_failed_payments"`
}

// NewPaymentIntegrationService creates a new payment integration service
func NewPaymentIntegrationService(config *Config, paymentProcessor PaymentProcessor, storage BillingStorage) *PaymentIntegrationService {
	ctx, cancel := context.WithCancel(context.Background())
	
	service := &PaymentIntegrationService{
		config:           config,
		paymentProcessor: paymentProcessor,
		storage:          storage,
		pendingPayments:  make(map[string]*PendingPayment),
		ctx:              ctx,
		cancel:           cancel,
	}
	
	// Start background services
	service.startBackgroundServices()
	
	return service
}

// ProcessInvoicePayment processes payment for an invoice
func (s *PaymentIntegrationService) ProcessInvoicePayment(ctx context.Context, invoiceID string, paymentMethod *PaymentMethod) (*PaymentResult, error) {
	// Get invoice details
	invoice, err := s.storage.GetInvoice(ctx, invoiceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get invoice: %w", err)
	}
	
	if invoice.Status == InvoiceStatusPaid {
		return nil, fmt.Errorf("invoice is already paid")
	}
	
	// Create payment request
	paymentRequest := &PaymentRequest{
		InvoiceID:     invoiceID,
		Amount:        invoice.TotalAmount,
		Currency:      invoice.Currency,
		PaymentMethod: paymentMethod,
		Customer:      invoice.Customer,
		Metadata: map[string]string{
			"invoice_id":     invoiceID,
			"invoice_number": invoice.Number,
			"owner_id":       invoice.OwnerID,
			"tenant_id":      invoice.TenantID,
		},
	}
	
	// Process payment
	result, err := s.paymentProcessor.ProcessPayment(ctx, paymentRequest)
	if err != nil {
		return nil, fmt.Errorf("payment processing failed: %w", err)
	}
	
	// Store payment record
	payment := &Payment{
		ID:            result.PaymentID,
		InvoiceID:     invoiceID,
		OwnerID:       invoice.OwnerID,
		Amount:        result.Amount,
		Currency:      result.Currency,
		Status:        result.Status,
		PaymentMethod: paymentMethod,
		TransactionID: result.TransactionID,
		ProcessedAt:   result.ProcessedAt,
		Metadata:      result.Metadata,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	
	if err := s.storage.StorePayment(ctx, payment); err != nil {
		integrationLogger.Errorf("Failed to store payment record: %v", err)
	}
	
	// Track pending payment if not immediately successful
	if result.Status == PaymentStatusPending {
		s.trackPendingPayment(result.PaymentID, invoiceID, paymentMethod.Provider, result.Amount, result.Currency)
	} else if result.Status == PaymentStatusSucceeded {
		// Update invoice status immediately
		if err := s.updateInvoicePaymentStatus(ctx, invoiceID, result.PaymentID); err != nil {
			integrationLogger.Errorf("Failed to update invoice status: %v", err)
		}
	}
	
	integrationLogger.Infof("Payment processed for invoice %s: payment_id=%s, status=%s, amount=%d %s", 
		invoiceID, result.PaymentID, result.Status, result.Amount, result.Currency)
	
	return result, nil
}

// HandlePaymentWebhook handles incoming payment webhooks
func (s *PaymentIntegrationService) HandlePaymentWebhook(ctx context.Context, webhook *PaymentWebhook) error {
	// Verify webhook signature if configured
	if err := s.verifyWebhookSignature(webhook); err != nil {
		return fmt.Errorf("webhook signature verification failed: %w", err)
	}
	
	// Process webhook with the appropriate payment processor
	if err := s.paymentProcessor.HandlePaymentWebhook(ctx, webhook); err != nil {
		return fmt.Errorf("webhook processing failed: %w", err)
	}
	
	// Update payment status based on webhook event
	if err := s.processWebhookEvent(ctx, webhook); err != nil {
		integrationLogger.Errorf("Failed to process webhook event: %v", err)
	}
	
	integrationLogger.Debugf("Processed payment webhook: provider=%s, event=%s", webhook.Provider, webhook.EventType)
	
	return nil
}

// RefundInvoicePayment processes a refund for an invoice payment
func (s *PaymentIntegrationService) RefundInvoicePayment(ctx context.Context, invoiceID string, amount int64, reason string) (*RefundResult, error) {
	// Get payment for the invoice
	payment, err := s.getInvoicePayment(ctx, invoiceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get invoice payment: %w", err)
	}
	
	if payment.Status != PaymentStatusSucceeded {
		return nil, fmt.Errorf("cannot refund payment with status: %s", payment.Status)
	}
	
	// Process refund
	refundResult, err := s.paymentProcessor.RefundPayment(ctx, payment.ID, amount)
	if err != nil {
		return nil, fmt.Errorf("refund processing failed: %w", err)
	}
	
	// Update payment status
	payment.Status = PaymentStatusRefunded
	payment.UpdatedAt = time.Now()
	payment.Metadata["refund_id"] = refundResult.RefundID
	payment.Metadata["refund_reason"] = reason
	
	if err := s.storage.StorePayment(ctx, payment); err != nil {
		integrationLogger.Errorf("Failed to update payment record: %v", err)
	}
	
	// Update invoice status
	invoice, err := s.storage.GetInvoice(ctx, invoiceID)
	if err == nil {
		invoice.Status = InvoiceStatusRefunded
		invoice.UpdatedAt = time.Now()
		if err := s.storage.StoreInvoice(ctx, invoice); err != nil {
			integrationLogger.Errorf("Failed to update invoice status: %v", err)
		}
	}
	
	integrationLogger.Infof("Refund processed for invoice %s: refund_id=%s, amount=%d", 
		invoiceID, refundResult.RefundID, amount)
	
	return refundResult, nil
}

// GetPaymentStatus gets the current status of a payment
func (s *PaymentIntegrationService) GetPaymentStatus(ctx context.Context, paymentID string) (*Payment, error) {
	return s.storage.GetPayment(ctx, paymentID)
}

// Shutdown gracefully shuts down the payment integration service
func (s *PaymentIntegrationService) Shutdown() {
	integrationLogger.Info("Shutting down payment integration service")
	s.cancel()
	s.wg.Wait()
}

// trackPendingPayment adds a payment to the pending payments tracker
func (s *PaymentIntegrationService) trackPendingPayment(paymentID, invoiceID, provider string, amount int64, currency string) {
	s.paymentMutex.Lock()
	defer s.paymentMutex.Unlock()
	
	pendingPayment := &PendingPayment{
		PaymentID:     paymentID,
		InvoiceID:     invoiceID,
		Provider:      provider,
		Amount:        amount,
		Currency:      currency,
		Status:        PaymentStatusPending,
		CreatedAt:     time.Now(),
		LastCheckedAt: time.Now(),
		RetryCount:    0,
		MaxRetries:    3,
		TimeoutAt:     time.Now().Add(30 * time.Minute), // 30 minute timeout
	}
	
	s.pendingPayments[paymentID] = pendingPayment
	
	integrationLogger.Debugf("Tracking pending payment: %s", paymentID)
}

// startBackgroundServices starts background monitoring services
func (s *PaymentIntegrationService) startBackgroundServices() {
	// Start payment monitoring service
	s.wg.Add(1)
	go s.runPaymentMonitoring()
	
	// Start cleanup service
	s.wg.Add(1)
	go s.runCleanupService()
}

// runPaymentMonitoring monitors pending payments for status updates
func (s *PaymentIntegrationService) runPaymentMonitoring() {
	defer s.wg.Done()
	
	ticker := time.NewTicker(30 * time.Second) // Check every 30 seconds
	defer ticker.Stop()
	
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.checkPendingPayments()
		}
	}
}

// runCleanupService cleans up old pending payments and expired data
func (s *PaymentIntegrationService) runCleanupService() {
	defer s.wg.Done()
	
	ticker := time.NewTicker(5 * time.Minute) // Cleanup every 5 minutes
	defer ticker.Stop()
	
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.cleanupExpiredPayments()
		}
	}
}

// checkPendingPayments checks the status of all pending payments
func (s *PaymentIntegrationService) checkPendingPayments() {
	s.paymentMutex.RLock()
	pendingList := make([]*PendingPayment, 0, len(s.pendingPayments))
	for _, payment := range s.pendingPayments {
		pendingList = append(pendingList, payment)
	}
	s.paymentMutex.RUnlock()
	
	for _, pending := range pendingList {
		if time.Now().After(pending.TimeoutAt) {
			s.handlePaymentTimeout(pending)
			continue
		}
		
		// Check payment status
		payment, err := s.storage.GetPayment(s.ctx, pending.PaymentID)
		if err != nil {
			integrationLogger.Errorf("Failed to get payment status for %s: %v", pending.PaymentID, err)
			continue
		}
		
		if payment.Status != PaymentStatusPending {
			s.handlePaymentStatusChange(pending, payment.Status)
		}
	}
}

// cleanupExpiredPayments removes expired pending payments from tracking
func (s *PaymentIntegrationService) cleanupExpiredPayments() {
	s.paymentMutex.Lock()
	defer s.paymentMutex.Unlock()
	
	now := time.Now()
	for paymentID, pending := range s.pendingPayments {
		if now.After(pending.TimeoutAt.Add(1 * time.Hour)) { // Keep for 1 hour after timeout
			delete(s.pendingPayments, paymentID)
			integrationLogger.Debugf("Cleaned up expired pending payment: %s", paymentID)
		}
	}
}

// handlePaymentTimeout handles payment timeouts
func (s *PaymentIntegrationService) handlePaymentTimeout(pending *PendingPayment) {
	integrationLogger.Warnf("Payment timeout: %s (invoice: %s)", pending.PaymentID, pending.InvoiceID)
	
	// Update payment status to failed
	payment, err := s.storage.GetPayment(s.ctx, pending.PaymentID)
	if err == nil {
		payment.Status = PaymentStatusFailed
		payment.UpdatedAt = time.Now()
		payment.Metadata["timeout_reason"] = "payment_timeout"
		
		if err := s.storage.StorePayment(s.ctx, payment); err != nil {
			integrationLogger.Errorf("Failed to update timed out payment: %v", err)
		}
	}
	
	// Remove from pending tracking
	s.paymentMutex.Lock()
	delete(s.pendingPayments, pending.PaymentID)
	s.paymentMutex.Unlock()
}

// handlePaymentStatusChange handles changes in payment status
func (s *PaymentIntegrationService) handlePaymentStatusChange(pending *PendingPayment, newStatus PaymentStatus) {
	integrationLogger.Infof("Payment status changed: %s -> %s (invoice: %s)", 
		pending.PaymentID, newStatus, pending.InvoiceID)
	
	if newStatus == PaymentStatusSucceeded {
		// Update invoice status
		if err := s.updateInvoicePaymentStatus(s.ctx, pending.InvoiceID, pending.PaymentID); err != nil {
			integrationLogger.Errorf("Failed to update invoice status: %v", err)
		}
	}
	
	// Remove from pending tracking
	s.paymentMutex.Lock()
	delete(s.pendingPayments, pending.PaymentID)
	s.paymentMutex.Unlock()
}

// updateInvoicePaymentStatus updates invoice status when payment is successful
func (s *PaymentIntegrationService) updateInvoicePaymentStatus(ctx context.Context, invoiceID, paymentID string) error {
	invoice, err := s.storage.GetInvoice(ctx, invoiceID)
	if err != nil {
		return fmt.Errorf("failed to get invoice: %w", err)
	}
	
	invoice.Status = InvoiceStatusPaid
	now := time.Now()
	invoice.PaidAt = &now
	invoice.UpdatedAt = now
	
	if invoice.PaymentMethod == nil {
		// Get payment method from payment record
		payment, err := s.storage.GetPayment(ctx, paymentID)
		if err == nil {
			invoice.PaymentMethod = payment.PaymentMethod
		}
	}
	
	return s.storage.StoreInvoice(ctx, invoice)
}

// getInvoicePayment gets the payment record for an invoice
func (s *PaymentIntegrationService) getInvoicePayment(ctx context.Context, invoiceID string) (*Payment, error) {
	// In a real implementation, you would query payments by invoice_id
	// For now, this is a placeholder that would need proper indexing
	return nil, fmt.Errorf("payment not found for invoice: %s", invoiceID)
}

// verifyWebhookSignature verifies the webhook signature
func (s *PaymentIntegrationService) verifyWebhookSignature(webhook *PaymentWebhook) error {
	// Implement webhook signature verification based on provider
	// This would typically use HMAC with a shared secret
	return nil
}

// processWebhookEvent processes webhook events to update payment status
func (s *PaymentIntegrationService) processWebhookEvent(ctx context.Context, webhook *PaymentWebhook) error {
	// Extract payment information from webhook data
	// Update payment status based on event type
	// This would be provider-specific implementation
	return nil
}