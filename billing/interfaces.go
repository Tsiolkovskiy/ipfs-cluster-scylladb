package billing

import (
	"context"
	"time"
)

// Pin represents a simplified pin structure for billing
type Pin struct {
	CID      string            `json:"cid"`
	Type     string            `json:"type"`
	Metadata map[string]string `json:"metadata"`
}

// BillingService provides billing and usage tracking services
type BillingService interface {
	// Usage tracking
	RecordPinUsage(ctx context.Context, event *PinUsageEvent) error
	GetUsageMetrics(ctx context.Context, req *UsageMetricsRequest) (*UsageMetrics, error)
	
	// Cost calculation
	CalculateCosts(ctx context.Context, req *CostCalculationRequest) (*CostCalculation, error)
	
	// Invoice management
	GenerateInvoice(ctx context.Context, req *InvoiceRequest) (*Invoice, error)
	GetInvoices(ctx context.Context, ownerID string, period *TimePeriod) ([]*Invoice, error)
	
	// Billing events
	ProcessBillingEvent(ctx context.Context, event *BillingEvent) error
	
	// Configuration
	UpdatePricingTier(ctx context.Context, tier *PricingTier) error
	GetPricingTiers(ctx context.Context) ([]*PricingTier, error)
}

// UsageCollector collects and aggregates usage data
type UsageCollector interface {
	// Collect usage data from pin operations
	CollectPinUsage(ctx context.Context, pin Pin, operation UsageOperation) error
	
	// Aggregate usage data for billing periods
	AggregateUsage(ctx context.Context, ownerID string, period *TimePeriod) (*UsageMetrics, error)
	
	// Daily rollup operations
	PerformDailyRollup(ctx context.Context, date time.Time) error
}

// CostCalculator calculates costs based on usage and pricing tiers
type CostCalculator interface {
	// Calculate costs for usage metrics
	CalculateUsageCosts(ctx context.Context, usage *UsageMetrics, tier *PricingTier) (*CostBreakdown, error)
	
	// Apply discounts and promotions
	ApplyDiscounts(ctx context.Context, costs *CostBreakdown, discounts []*Discount) (*CostBreakdown, error)
	
	// Calculate taxes
	CalculateTaxes(ctx context.Context, costs *CostBreakdown, taxConfig *TaxConfiguration) (*CostBreakdown, error)
}

// InvoiceEmitter generates and manages invoices
type InvoiceEmitter interface {
	// Generate invoice from cost calculation
	EmitInvoice(ctx context.Context, costs *CostBreakdown, customer *Customer) (*Invoice, error)
	
	// Send invoice notifications
	SendInvoiceNotification(ctx context.Context, invoice *Invoice) error
	
	// Update invoice status
	UpdateInvoiceStatus(ctx context.Context, invoiceID string, status InvoiceStatus) error
}

// PaymentProcessor handles payment processing
type PaymentProcessor interface {
	// Process payment for invoice
	ProcessPayment(ctx context.Context, payment *PaymentRequest) (*PaymentResult, error)
	
	// Handle payment webhooks
	HandlePaymentWebhook(ctx context.Context, webhook *PaymentWebhook) error
	
	// Refund payment
	RefundPayment(ctx context.Context, paymentID string, amount int64) (*RefundResult, error)
}

// BillingStorage provides storage operations for billing data
type BillingStorage interface {
	// Usage storage
	StoreUsageEvent(ctx context.Context, event *UsageEvent) error
	GetUsageEvents(ctx context.Context, ownerID string, period *TimePeriod) ([]*UsageEvent, error)
	
	// Aggregated usage storage
	StoreUsageMetrics(ctx context.Context, metrics *UsageMetrics) error
	GetUsageMetrics(ctx context.Context, ownerID string, period *TimePeriod) (*UsageMetrics, error)
	
	// Invoice storage
	StoreInvoice(ctx context.Context, invoice *Invoice) error
	GetInvoice(ctx context.Context, invoiceID string) (*Invoice, error)
	GetInvoicesByOwner(ctx context.Context, ownerID string, period *TimePeriod) ([]*Invoice, error)
	
	// Payment storage
	StorePayment(ctx context.Context, payment *Payment) error
	GetPayment(ctx context.Context, paymentID string) (*Payment, error)
	
	// Pricing configuration storage
	StorePricingTier(ctx context.Context, tier *PricingTier) error
	GetPricingTier(ctx context.Context, tierID string) (*PricingTier, error)
	GetPricingTiers(ctx context.Context) ([]*PricingTier, error)
}

// UsageOperation represents different types of usage operations
type UsageOperation string

const (
	UsageOperationPinAdd    UsageOperation = "pin_add"
	UsageOperationPinRemove UsageOperation = "pin_remove"
	UsageOperationPinUpdate UsageOperation = "pin_update"
	UsageOperationStorage   UsageOperation = "storage"
	UsageOperationBandwidth UsageOperation = "bandwidth"
	UsageOperationAPI       UsageOperation = "api_call"
)

// PinUsageEvent represents a pin usage event
type PinUsageEvent struct {
	EventID     string         `json:"event_id"`
	OwnerID     string         `json:"owner_id"`
	TenantID    string         `json:"tenant_id"`
	CID         string         `json:"cid"`
	Operation   UsageOperation `json:"operation"`
	Size        int64          `json:"size"`        // Size in bytes
	Duration    time.Duration  `json:"duration"`    // Duration for storage operations
	Timestamp   time.Time      `json:"timestamp"`
	Metadata    map[string]string `json:"metadata"`
}

// UsageEvent represents a stored usage event
type UsageEvent struct {
	ID          string            `json:"id"`
	OwnerID     string            `json:"owner_id"`
	TenantID    string            `json:"tenant_id"`
	Operation   UsageOperation    `json:"operation"`
	ResourceID  string            `json:"resource_id"` // CID or other resource identifier
	Quantity    int64             `json:"quantity"`    // Bytes, API calls, etc.
	Unit        string            `json:"unit"`        // "bytes", "calls", "hours"
	Timestamp   time.Time         `json:"timestamp"`
	Metadata    map[string]string `json:"metadata"`
	ProcessedAt *time.Time        `json:"processed_at,omitempty"`
}

// UsageMetrics represents aggregated usage metrics
type UsageMetrics struct {
	OwnerID         string            `json:"owner_id"`
	TenantID        string            `json:"tenant_id"`
	Period          *TimePeriod       `json:"period"`
	StorageBytes    int64             `json:"storage_bytes"`     // Total bytes stored
	StorageHours    int64             `json:"storage_hours"`     // Total storage hours
	BandwidthBytes  int64             `json:"bandwidth_bytes"`   // Total bandwidth used
	APICalls        int64             `json:"api_calls"`         // Total API calls
	PinOperations   int64             `json:"pin_operations"`    // Total pin operations
	UniqueObjects   int64             `json:"unique_objects"`    // Number of unique objects
	Metadata        map[string]string `json:"metadata"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
}

// TimePeriod represents a billing time period
type TimePeriod struct {
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	Type      string    `json:"type"` // "daily", "monthly", "yearly"
}

// PricingTier represents a pricing configuration
type PricingTier struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	Description     string            `json:"description"`
	Currency        string            `json:"currency"`
	StoragePricing  *StoragePricing   `json:"storage_pricing"`
	BandwidthPricing *BandwidthPricing `json:"bandwidth_pricing"`
	APIPricing      *APIPricing       `json:"api_pricing"`
	Discounts       []*VolumeDiscount `json:"discounts"`
	IsActive        bool              `json:"is_active"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
}

// StoragePricing represents storage pricing configuration
type StoragePricing struct {
	PricePerGBMonth int64  `json:"price_per_gb_month"` // Price in smallest currency unit (cents)
	MinimumCharge   int64  `json:"minimum_charge"`     // Minimum monthly charge
	FreeQuotaGB     int64  `json:"free_quota_gb"`      // Free storage quota in GB
	Model           string `json:"model"`              // "per_gb_month", "tiered"
}

// BandwidthPricing represents bandwidth pricing configuration
type BandwidthPricing struct {
	PricePerGB    int64  `json:"price_per_gb"`    // Price per GB transferred
	FreeQuotaGB   int64  `json:"free_quota_gb"`   // Free bandwidth quota in GB
	Model         string `json:"model"`           // "per_gb", "tiered"
}

// APIPricing represents API call pricing configuration
type APIPricing struct {
	PricePerCall     int64  `json:"price_per_call"`      // Price per API call
	PricePerThousand int64  `json:"price_per_thousand"`  // Price per 1000 API calls
	FreeQuotaCalls   int64  `json:"free_quota_calls"`    // Free API calls quota
	Model            string `json:"model"`               // "per_call", "per_thousand", "tiered"
}

// VolumeDiscount represents volume-based discounts
type VolumeDiscount struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Threshold   int64   `json:"threshold"`   // Threshold for discount (bytes, calls, etc.)
	DiscountPct float64 `json:"discount_pct"` // Discount percentage (0.1 = 10%)
	MaxDiscount int64   `json:"max_discount"` // Maximum discount amount
	AppliesTo   string  `json:"applies_to"`   // "storage", "bandwidth", "api", "total"
}

// CostCalculationRequest represents a cost calculation request
type CostCalculationRequest struct {
	OwnerID     string        `json:"owner_id"`
	TenantID    string        `json:"tenant_id"`
	Usage       *UsageMetrics `json:"usage"`
	PricingTier *PricingTier  `json:"pricing_tier"`
	Discounts   []*Discount   `json:"discounts"`
	TaxConfig   *TaxConfiguration `json:"tax_config"`
}

// CostCalculation represents the result of cost calculation
type CostCalculation struct {
	OwnerID       string         `json:"owner_id"`
	TenantID      string         `json:"tenant_id"`
	Period        *TimePeriod    `json:"period"`
	Usage         *UsageMetrics  `json:"usage"`
	Breakdown     *CostBreakdown `json:"breakdown"`
	TotalAmount   int64          `json:"total_amount"`   // Total in smallest currency unit
	Currency      string         `json:"currency"`
	CalculatedAt  time.Time      `json:"calculated_at"`
}

// CostBreakdown represents detailed cost breakdown
type CostBreakdown struct {
	StorageCosts    *LineItem   `json:"storage_costs"`
	BandwidthCosts  *LineItem   `json:"bandwidth_costs"`
	APICosts        *LineItem   `json:"api_costs"`
	Discounts       []*LineItem `json:"discounts"`
	Taxes           []*LineItem `json:"taxes"`
	Subtotal        int64       `json:"subtotal"`
	TotalDiscounts  int64       `json:"total_discounts"`
	TotalTaxes      int64       `json:"total_taxes"`
	Total           int64       `json:"total"`
	Currency        string      `json:"currency"`
}

// LineItem represents a single cost line item
type LineItem struct {
	Description string  `json:"description"`
	Quantity    int64   `json:"quantity"`
	Unit        string  `json:"unit"`
	UnitPrice   int64   `json:"unit_price"`
	Amount      int64   `json:"amount"`
	Metadata    map[string]string `json:"metadata"`
}

// Discount represents a discount applied to billing
type Discount struct {
	ID          string    `json:"id"`
	Code        string    `json:"code"`
	Name        string    `json:"name"`
	Type        string    `json:"type"`        // "percentage", "fixed_amount"
	Value       int64     `json:"value"`       // Percentage (1-100) or fixed amount
	AppliesTo   string    `json:"applies_to"`  // "storage", "bandwidth", "api", "total"
	MaxUses     int       `json:"max_uses"`
	UsedCount   int       `json:"used_count"`
	ValidFrom   time.Time `json:"valid_from"`
	ValidUntil  time.Time `json:"valid_until"`
	IsActive    bool      `json:"is_active"`
}

// TaxConfiguration represents tax calculation configuration
type TaxConfiguration struct {
	Country     string  `json:"country"`
	Region      string  `json:"region"`
	TaxRate     float64 `json:"tax_rate"`     // Tax rate as decimal (0.08 = 8%)
	TaxName     string  `json:"tax_name"`     // "VAT", "GST", "Sales Tax"
	TaxID       string  `json:"tax_id"`       // Tax registration ID
	IsInclusive bool    `json:"is_inclusive"` // Whether tax is included in prices
}

// Customer represents a billing customer
type Customer struct {
	ID            string            `json:"id"`
	OwnerID       string            `json:"owner_id"`
	TenantID      string            `json:"tenant_id"`
	Name          string            `json:"name"`
	Email         string            `json:"email"`
	BillingAddress *Address         `json:"billing_address"`
	TaxID         string            `json:"tax_id"`
	PricingTierID string            `json:"pricing_tier_id"`
	PaymentMethod *PaymentMethod    `json:"payment_method"`
	Metadata      map[string]string `json:"metadata"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
}

// Address represents a billing address
type Address struct {
	Line1      string `json:"line1"`
	Line2      string `json:"line2"`
	City       string `json:"city"`
	State      string `json:"state"`
	PostalCode string `json:"postal_code"`
	Country    string `json:"country"`
}

// PaymentMethod represents a payment method
type PaymentMethod struct {
	ID       string            `json:"id"`
	Type     string            `json:"type"`     // "card", "bank_transfer", "crypto"
	Provider string            `json:"provider"` // "stripe", "filecoin", "polygon"
	Details  map[string]string `json:"details"`  // Provider-specific details
	IsDefault bool             `json:"is_default"`
}

// Invoice represents a billing invoice
type Invoice struct {
	ID              string         `json:"id"`
	Number          string         `json:"number"`
	OwnerID         string         `json:"owner_id"`
	TenantID        string         `json:"tenant_id"`
	Customer        *Customer      `json:"customer"`
	Period          *TimePeriod    `json:"period"`
	Usage           *UsageMetrics  `json:"usage"`
	CostBreakdown   *CostBreakdown `json:"cost_breakdown"`
	TotalAmount     int64          `json:"total_amount"`
	Currency        string         `json:"currency"`
	Status          InvoiceStatus  `json:"status"`
	DueDate         time.Time      `json:"due_date"`
	PaidAt          *time.Time     `json:"paid_at,omitempty"`
	PaymentMethod   *PaymentMethod `json:"payment_method,omitempty"`
	Notes           string         `json:"notes"`
	Metadata        map[string]string `json:"metadata"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

// InvoiceStatus represents invoice status
type InvoiceStatus string

const (
	InvoiceStatusDraft     InvoiceStatus = "draft"
	InvoiceStatusSent      InvoiceStatus = "sent"
	InvoiceStatusPaid      InvoiceStatus = "paid"
	InvoiceStatusOverdue   InvoiceStatus = "overdue"
	InvoiceStatusCancelled InvoiceStatus = "cancelled"
	InvoiceStatusRefunded  InvoiceStatus = "refunded"
)

// InvoiceRequest represents an invoice generation request
type InvoiceRequest struct {
	OwnerID       string        `json:"owner_id"`
	TenantID      string        `json:"tenant_id"`
	Period        *TimePeriod   `json:"period"`
	Usage         *UsageMetrics `json:"usage"`
	PricingTierID string        `json:"pricing_tier_id"`
	DiscountCodes []string      `json:"discount_codes"`
	DueDate       time.Time     `json:"due_date"`
	Notes         string        `json:"notes"`
}

// UsageMetricsRequest represents a usage metrics request
type UsageMetricsRequest struct {
	OwnerID  string      `json:"owner_id"`
	TenantID string      `json:"tenant_id"`
	Period   *TimePeriod `json:"period"`
	GroupBy  string      `json:"group_by"` // "day", "week", "month"
}

// BillingEvent represents an event in the billing system
type BillingEvent struct {
	ID        string            `json:"id"`
	Type      string            `json:"type"`      // "usage", "payment", "invoice"
	OwnerID   string            `json:"owner_id"`
	TenantID  string            `json:"tenant_id"`
	Data      map[string]interface{} `json:"data"`
	Timestamp time.Time         `json:"timestamp"`
	Metadata  map[string]string `json:"metadata"`
}

// PaymentRequest represents a payment processing request
type PaymentRequest struct {
	InvoiceID     string         `json:"invoice_id"`
	Amount        int64          `json:"amount"`
	Currency      string         `json:"currency"`
	PaymentMethod *PaymentMethod `json:"payment_method"`
	Customer      *Customer      `json:"customer"`
	Metadata      map[string]string `json:"metadata"`
}

// PaymentResult represents the result of payment processing
type PaymentResult struct {
	PaymentID     string        `json:"payment_id"`
	Status        PaymentStatus `json:"status"`
	Amount        int64         `json:"amount"`
	Currency      string        `json:"currency"`
	ProcessedAt   time.Time     `json:"processed_at"`
	TransactionID string        `json:"transaction_id"`
	Metadata      map[string]string `json:"metadata"`
}

// PaymentStatus represents payment status
type PaymentStatus string

const (
	PaymentStatusPending   PaymentStatus = "pending"
	PaymentStatusSucceeded PaymentStatus = "succeeded"
	PaymentStatusFailed    PaymentStatus = "failed"
	PaymentStatusCancelled PaymentStatus = "cancelled"
	PaymentStatusRefunded  PaymentStatus = "refunded"
)

// Payment represents a payment record
type Payment struct {
	ID            string        `json:"id"`
	InvoiceID     string        `json:"invoice_id"`
	OwnerID       string        `json:"owner_id"`
	Amount        int64         `json:"amount"`
	Currency      string        `json:"currency"`
	Status        PaymentStatus `json:"status"`
	PaymentMethod *PaymentMethod `json:"payment_method"`
	TransactionID string        `json:"transaction_id"`
	ProcessedAt   time.Time     `json:"processed_at"`
	Metadata      map[string]string `json:"metadata"`
	CreatedAt     time.Time     `json:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at"`
}

// PaymentWebhook represents a payment webhook event
type PaymentWebhook struct {
	ID        string            `json:"id"`
	Provider  string            `json:"provider"`
	EventType string            `json:"event_type"`
	Data      map[string]interface{} `json:"data"`
	Signature string            `json:"signature"`
	Timestamp time.Time         `json:"timestamp"`
}

// RefundResult represents the result of a refund operation
type RefundResult struct {
	RefundID      string    `json:"refund_id"`
	PaymentID     string    `json:"payment_id"`
	Amount        int64     `json:"amount"`
	Currency      string    `json:"currency"`
	Status        string    `json:"status"`
	ProcessedAt   time.Time `json:"processed_at"`
	TransactionID string    `json:"transaction_id"`
}