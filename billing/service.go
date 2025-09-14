package billing

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	logging "github.com/ipfs/go-log/v2"
)

var logger = logging.Logger("billing")

// Service implements the main billing service
type Service struct {
	config          *Config
	storage         BillingStorage
	usageCollector  UsageCollector
	costCalculator  CostCalculator
	invoiceEmitter  InvoiceEmitter
	paymentProcessor PaymentProcessor
	natsConn        *nats.Conn
	mu              sync.RWMutex
}

// Config holds configuration for the billing service
type Config struct {
	// Service settings
	Enabled         bool          `json:"enabled"`
	DefaultCurrency string        `json:"default_currency"`
	InvoiceDueDays  int           `json:"invoice_due_days"`
	
	// Usage collection settings
	UsageCollectionEnabled bool          `json:"usage_collection_enabled"`
	UsageAggregationPeriod time.Duration `json:"usage_aggregation_period"`
	DailyRollupTime        string        `json:"daily_rollup_time"` // "02:00" format
	
	// Cost calculation settings
	DefaultPricingTierID string `json:"default_pricing_tier_id"`
	TaxCalculationEnabled bool  `json:"tax_calculation_enabled"`
	
	// Invoice settings
	InvoiceGenerationEnabled bool   `json:"invoice_generation_enabled"`
	InvoiceNumberPrefix      string `json:"invoice_number_prefix"`
	AutoSendInvoices         bool   `json:"auto_send_invoices"`
	
	// Payment settings
	PaymentProcessingEnabled bool     `json:"payment_processing_enabled"`
	SupportedPaymentMethods  []string `json:"supported_payment_methods"`
	
	// NATS settings
	NATSEnabled     bool   `json:"nats_enabled"`
	NATSURL         string `json:"nats_url"`
	BillingTopic    string `json:"billing_topic"`
	
	// Storage settings
	StorageType     string `json:"storage_type"` // "scylladb", "memory"
	RetentionPeriod time.Duration `json:"retention_period"`
}

// DefaultConfig returns a default billing service configuration
func DefaultConfig() *Config {
	return &Config{
		Enabled:                  false,
		DefaultCurrency:          "USD",
		InvoiceDueDays:          30,
		UsageCollectionEnabled:   true,
		UsageAggregationPeriod:  time.Hour,
		DailyRollupTime:         "02:00",
		DefaultPricingTierID:    "standard",
		TaxCalculationEnabled:   false,
		InvoiceGenerationEnabled: true,
		InvoiceNumberPrefix:     "INV-",
		AutoSendInvoices:        false,
		PaymentProcessingEnabled: false,
		SupportedPaymentMethods: []string{"stripe", "filecoin"},
		NATSEnabled:             true,
		NATSURL:                 "nats://localhost:4222",
		BillingTopic:            "billing.events",
		StorageType:             "scylladb",
		RetentionPeriod:         time.Hour * 24 * 365 * 2, // 2 years
	}
}

// NewService creates a new billing service
func NewService(ctx context.Context, config *Config, storage BillingStorage) (*Service, error) {
	if !config.Enabled {
		return &Service{config: config}, nil
	}
	
	service := &Service{
		config:  config,
		storage: storage,
	}
	
	// Initialize components
	if err := service.initializeComponents(ctx); err != nil {
		return nil, fmt.Errorf("failed to initialize billing components: %w", err)
	}
	
	// Connect to NATS if enabled
	if config.NATSEnabled {
		if err := service.connectNATS(ctx); err != nil {
			return nil, fmt.Errorf("failed to connect to NATS: %w", err)
		}
	}
	
	// Start background services
	if err := service.startBackgroundServices(ctx); err != nil {
		return nil, fmt.Errorf("failed to start background services: %w", err)
	}
	
	logger.Info("Billing service initialized successfully")
	return service, nil
}

// RecordPinUsage records a pin usage event
func (s *Service) RecordPinUsage(ctx context.Context, event *PinUsageEvent) error {
	if !s.config.Enabled || !s.config.UsageCollectionEnabled {
		return nil
	}
	
	// Convert to usage event
	usageEvent := &UsageEvent{
		ID:         generateEventID(),
		OwnerID:    event.OwnerID,
		TenantID:   event.TenantID,
		Operation:  event.Operation,
		ResourceID: event.CID,
		Quantity:   event.Size,
		Unit:       "bytes",
		Timestamp:  event.Timestamp,
		Metadata:   event.Metadata,
	}
	
	// Store usage event
	if err := s.storage.StoreUsageEvent(ctx, usageEvent); err != nil {
		return fmt.Errorf("failed to store usage event: %w", err)
	}
	
	// Publish billing event to NATS
	if s.config.NATSEnabled && s.natsConn != nil {
		billingEvent := &BillingEvent{
			ID:        generateEventID(),
			Type:      "usage",
			OwnerID:   event.OwnerID,
			TenantID:  event.TenantID,
			Data:      map[string]interface{}{"usage_event": usageEvent},
			Timestamp: time.Now(),
		}
		
		if err := s.publishBillingEvent(ctx, billingEvent); err != nil {
			logger.Warnf("Failed to publish billing event: %v", err)
		}
	}
	
	logger.Debugf("Recorded pin usage: owner=%s, operation=%s, size=%d", 
		event.OwnerID, event.Operation, event.Size)
	
	return nil
}

// GetUsageMetrics retrieves usage metrics for a customer
func (s *Service) GetUsageMetrics(ctx context.Context, req *UsageMetricsRequest) (*UsageMetrics, error) {
	if !s.config.Enabled {
		return nil, fmt.Errorf("billing service is disabled")
	}
	
	// Try to get aggregated metrics first
	metrics, err := s.storage.GetUsageMetrics(ctx, req.OwnerID, req.Period)
	if err == nil && metrics != nil {
		return metrics, nil
	}
	
	// If no aggregated metrics, calculate from raw events
	if s.usageCollector != nil {
		return s.usageCollector.AggregateUsage(ctx, req.OwnerID, req.Period)
	}
	
	return nil, fmt.Errorf("no usage metrics available for owner %s", req.OwnerID)
}

// CalculateCosts calculates costs for usage metrics
func (s *Service) CalculateCosts(ctx context.Context, req *CostCalculationRequest) (*CostCalculation, error) {
	if !s.config.Enabled {
		return nil, fmt.Errorf("billing service is disabled")
	}
	
	if s.costCalculator == nil {
		return nil, fmt.Errorf("cost calculator not initialized")
	}
	
	// Get pricing tier if not provided
	pricingTier := req.PricingTier
	if pricingTier == nil {
		tier, err := s.storage.GetPricingTier(ctx, s.config.DefaultPricingTierID)
		if err != nil {
			return nil, fmt.Errorf("failed to get default pricing tier: %w", err)
		}
		pricingTier = tier
	}
	
	// Calculate base costs
	breakdown, err := s.costCalculator.CalculateUsageCosts(ctx, req.Usage, pricingTier)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate usage costs: %w", err)
	}
	
	// Apply discounts
	if len(req.Discounts) > 0 {
		breakdown, err = s.costCalculator.ApplyDiscounts(ctx, breakdown, req.Discounts)
		if err != nil {
			return nil, fmt.Errorf("failed to apply discounts: %w", err)
		}
	}
	
	// Calculate taxes
	if s.config.TaxCalculationEnabled && req.TaxConfig != nil {
		breakdown, err = s.costCalculator.CalculateTaxes(ctx, breakdown, req.TaxConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to calculate taxes: %w", err)
		}
	}
	
	calculation := &CostCalculation{
		OwnerID:      req.OwnerID,
		TenantID:     req.TenantID,
		Period:       req.Usage.Period,
		Usage:        req.Usage,
		Breakdown:    breakdown,
		TotalAmount:  breakdown.Total,
		Currency:     breakdown.Currency,
		CalculatedAt: time.Now(),
	}
	
	return calculation, nil
}

// GenerateInvoice generates an invoice for a customer
func (s *Service) GenerateInvoice(ctx context.Context, req *InvoiceRequest) (*Invoice, error) {
	if !s.config.Enabled || !s.config.InvoiceGenerationEnabled {
		return nil, fmt.Errorf("invoice generation is disabled")
	}
	
	if s.invoiceEmitter == nil {
		return nil, fmt.Errorf("invoice emitter not initialized")
	}
	
	// Get customer information
	// Note: This would typically come from a customer service
	customer := &Customer{
		ID:       req.OwnerID,
		OwnerID:  req.OwnerID,
		TenantID: req.TenantID,
		Name:     "Customer " + req.OwnerID, // Placeholder
		Email:    "customer@example.com",    // Placeholder
	}
	
	// Calculate costs
	costReq := &CostCalculationRequest{
		OwnerID:     req.OwnerID,
		TenantID:    req.TenantID,
		Usage:       req.Usage,
		PricingTier: nil, // Will use default
	}
	
	calculation, err := s.CalculateCosts(ctx, costReq)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate costs: %w", err)
	}
	
	// Generate invoice
	invoice, err := s.invoiceEmitter.EmitInvoice(ctx, calculation.Breakdown, customer)
	if err != nil {
		return nil, fmt.Errorf("failed to emit invoice: %w", err)
	}
	
	// Set additional invoice fields
	invoice.Number = s.generateInvoiceNumber()
	invoice.Period = req.Period
	invoice.Usage = req.Usage
	invoice.DueDate = req.DueDate
	invoice.Notes = req.Notes
	invoice.Status = InvoiceStatusDraft
	
	// Store invoice
	if err := s.storage.StoreInvoice(ctx, invoice); err != nil {
		return nil, fmt.Errorf("failed to store invoice: %w", err)
	}
	
	// Send invoice if auto-send is enabled
	if s.config.AutoSendInvoices {
		if err := s.invoiceEmitter.SendInvoiceNotification(ctx, invoice); err != nil {
			logger.Warnf("Failed to send invoice notification: %v", err)
		} else {
			invoice.Status = InvoiceStatusSent
			if err := s.invoiceEmitter.UpdateInvoiceStatus(ctx, invoice.ID, InvoiceStatusSent); err != nil {
				logger.Warnf("Failed to update invoice status: %v", err)
			}
		}
	}
	
	logger.Infof("Generated invoice %s for owner %s, amount: %d %s", 
		invoice.Number, req.OwnerID, invoice.TotalAmount, invoice.Currency)
	
	return invoice, nil
}

// GetInvoices retrieves invoices for a customer
func (s *Service) GetInvoices(ctx context.Context, ownerID string, period *TimePeriod) ([]*Invoice, error) {
	if !s.config.Enabled {
		return nil, fmt.Errorf("billing service is disabled")
	}
	
	return s.storage.GetInvoicesByOwner(ctx, ownerID, period)
}

// ProcessBillingEvent processes a billing event
func (s *Service) ProcessBillingEvent(ctx context.Context, event *BillingEvent) error {
	if !s.config.Enabled {
		return nil
	}
	
	logger.Debugf("Processing billing event: type=%s, owner=%s", event.Type, event.OwnerID)
	
	switch event.Type {
	case "usage":
		return s.processBillingUsageEvent(ctx, event)
	case "payment":
		return s.processBillingPaymentEvent(ctx, event)
	case "invoice":
		return s.processBillingInvoiceEvent(ctx, event)
	default:
		logger.Warnf("Unknown billing event type: %s", event.Type)
		return nil
	}
}

// UpdatePricingTier updates a pricing tier
func (s *Service) UpdatePricingTier(ctx context.Context, tier *PricingTier) error {
	if !s.config.Enabled {
		return fmt.Errorf("billing service is disabled")
	}
	
	tier.UpdatedAt = time.Now()
	return s.storage.StorePricingTier(ctx, tier)
}

// GetPricingTiers retrieves all pricing tiers
func (s *Service) GetPricingTiers(ctx context.Context) ([]*PricingTier, error) {
	if !s.config.Enabled {
		return nil, fmt.Errorf("billing service is disabled")
	}
	
	return s.storage.GetPricingTiers(ctx)
}

// IsEnabled returns whether the billing service is enabled
func (s *Service) IsEnabled() bool {
	return s.config.Enabled
}

// Shutdown gracefully shuts down the billing service
func (s *Service) Shutdown(ctx context.Context) error {
	logger.Info("Shutting down billing service")
	
	if s.natsConn != nil {
		s.natsConn.Close()
	}
	
	return nil
}

// initializeComponents initializes billing service components
func (s *Service) initializeComponents(ctx context.Context) error {
	// Initialize usage collector
	if s.config.UsageCollectionEnabled {
		s.usageCollector = NewUsageCollector(s.storage, s.config)
	}
	
	// Initialize cost calculator
	s.costCalculator = NewCostCalculator(s.config)
	
	// Initialize invoice emitter
	if s.config.InvoiceGenerationEnabled {
		s.invoiceEmitter = NewInvoiceEmitter(s.storage, s.config)
	}
	
	// Initialize payment processor
	if s.config.PaymentProcessingEnabled {
		paymentConfig := loadPaymentConfigFromEnv()
		
		var err error
		s.paymentProcessor, err = NewPaymentProcessor(s.config, paymentConfig, s.storage)
		if err != nil {
			return fmt.Errorf("failed to initialize payment processor: %w", err)
		}
	}
	
	return nil
}

// connectNATS connects to NATS for event publishing
func (s *Service) connectNATS(ctx context.Context) error {
	var err error
	s.natsConn, err = nats.Connect(s.config.NATSURL)
	if err != nil {
		return fmt.Errorf("failed to connect to NATS: %w", err)
	}
	
	logger.Infof("Connected to NATS at %s", s.config.NATSURL)
	return nil
}

// startBackgroundServices starts background services
func (s *Service) startBackgroundServices(ctx context.Context) error {
	// Start daily rollup service
	if s.config.UsageCollectionEnabled && s.usageCollector != nil {
		go s.runDailyRollupService(ctx)
	}
	
	// Start usage aggregation service
	if s.config.UsageCollectionEnabled {
		go s.runUsageAggregationService(ctx)
	}
	
	return nil
}

// publishBillingEvent publishes a billing event to NATS
func (s *Service) publishBillingEvent(ctx context.Context, event *BillingEvent) error {
	if s.natsConn == nil {
		return fmt.Errorf("NATS connection not available")
	}
	
	data, err := marshalBillingEvent(event)
	if err != nil {
		return fmt.Errorf("failed to marshal billing event: %w", err)
	}
	
	return s.natsConn.Publish(s.config.BillingTopic, data)
}

// processBillingUsageEvent processes a usage billing event
func (s *Service) processBillingUsageEvent(ctx context.Context, event *BillingEvent) error {
	// Extract usage event from data
	_, ok := event.Data["usage_event"]
	if !ok {
		return fmt.Errorf("missing usage_event in billing event data")
	}
	
	// Process usage event (e.g., trigger aggregation, alerts, etc.)
	logger.Debugf("Processed usage billing event for owner %s", event.OwnerID)
	return nil
}

// processBillingPaymentEvent processes a payment billing event
func (s *Service) processBillingPaymentEvent(ctx context.Context, event *BillingEvent) error {
	// Process payment event (e.g., update invoice status, send notifications)
	logger.Debugf("Processed payment billing event for owner %s", event.OwnerID)
	return nil
}

// processBillingInvoiceEvent processes an invoice billing event
func (s *Service) processBillingInvoiceEvent(ctx context.Context, event *BillingEvent) error {
	// Process invoice event (e.g., send notifications, update status)
	logger.Debugf("Processed invoice billing event for owner %s", event.OwnerID)
	return nil
}

// generateInvoiceNumber generates a unique invoice number
func (s *Service) generateInvoiceNumber() string {
	timestamp := time.Now().Unix()
	return fmt.Sprintf("%s%d", s.config.InvoiceNumberPrefix, timestamp)
}

// runDailyRollupService runs the daily usage rollup service
func (s *Service) runDailyRollupService(ctx context.Context) {
	ticker := time.NewTicker(time.Hour) // Check every hour
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if s.shouldRunDailyRollup() {
				yesterday := time.Now().AddDate(0, 0, -1)
				if err := s.usageCollector.PerformDailyRollup(ctx, yesterday); err != nil {
					logger.Errorf("Failed to perform daily rollup: %v", err)
				}
			}
		}
	}
}

// runUsageAggregationService runs the usage aggregation service
func (s *Service) runUsageAggregationService(ctx context.Context) {
	ticker := time.NewTicker(s.config.UsageAggregationPeriod)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Perform periodic usage aggregation
			logger.Debug("Running usage aggregation")
		}
	}
}

// shouldRunDailyRollup checks if it's time to run daily rollup
func (s *Service) shouldRunDailyRollup() bool {
	now := time.Now()
	rollupTime, err := time.Parse("15:04", s.config.DailyRollupTime)
	if err != nil {
		return false
	}
	
	// Check if current time matches rollup time (within 1 hour window)
	currentTime := time.Date(0, 0, 0, now.Hour(), now.Minute(), 0, 0, time.UTC)
	rollupTimeToday := time.Date(0, 0, 0, rollupTime.Hour(), rollupTime.Minute(), 0, 0, time.UTC)
	
	diff := currentTime.Sub(rollupTimeToday)
	return diff >= 0 && diff < time.Hour
}

// generateEventID generates a unique event ID
func generateEventID() string {
	return fmt.Sprintf("evt_%d", time.Now().UnixNano())
}

// marshalBillingEvent marshals a billing event to bytes
func marshalBillingEvent(event *BillingEvent) ([]byte, error) {
	// This would typically use JSON or protobuf
	// For now, return a placeholder
	return []byte(fmt.Sprintf("billing_event:%s", event.ID)), nil
}

// loadPaymentConfigFromEnv loads payment configuration from environment variables
func loadPaymentConfigFromEnv() *PaymentConfig {
	return &PaymentConfig{
		// Stripe configuration
		StripeSecretKey:      os.Getenv("STRIPE_SECRET_KEY"),
		StripeWebhookSecret:  os.Getenv("STRIPE_WEBHOOK_SECRET"),
		StripePublishableKey: os.Getenv("STRIPE_PUBLISHABLE_KEY"),
		
		// Filecoin configuration
		FilecoinNodeURL:    getEnvOrDefault("FILECOIN_NODE_URL", "https://api.node.glif.io"),
		FilecoinWalletAddr: os.Getenv("FILECOIN_WALLET_ADDR"),
		FilecoinPrivateKey: os.Getenv("FILECOIN_PRIVATE_KEY"),
		
		// Ethereum/L2 configuration
		EthereumRPCURL:      getEnvOrDefault("ETHEREUM_RPC_URL", "https://mainnet.infura.io/v3/YOUR_PROJECT_ID"),
		PolygonRPCURL:       getEnvOrDefault("POLYGON_RPC_URL", "https://polygon-rpc.com"),
		ArbitrumRPCURL:      getEnvOrDefault("ARBITRUM_RPC_URL", "https://arb1.arbitrum.io/rpc"),
		PaymentContractAddr: os.Getenv("PAYMENT_CONTRACT_ADDR"),
		WalletPrivateKey:    os.Getenv("WALLET_PRIVATE_KEY"),
		
		// General settings
		ConfirmationBlocks: 12,
		PaymentTimeout:     time.Minute * 10,
	}
}

// getEnvOrDefault returns environment variable value or default if not set
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}