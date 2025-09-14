package billing

import (
	"context"
	"fmt"
	"math"
)

// CostCalculatorImpl implements the CostCalculator interface
type CostCalculatorImpl struct {
	config *Config
}

// NewCostCalculator creates a new cost calculator
func NewCostCalculator(config *Config) CostCalculator {
	return &CostCalculatorImpl{
		config: config,
	}
}

// CalculateUsageCosts calculates costs for usage metrics based on pricing tier
func (c *CostCalculatorImpl) CalculateUsageCosts(ctx context.Context, usage *UsageMetrics, tier *PricingTier) (*CostBreakdown, error) {
	breakdown := &CostBreakdown{
		Currency:       tier.Currency,
		Discounts:      make([]*LineItem, 0),
		Taxes:          make([]*LineItem, 0),
		Subtotal:       0,
		TotalDiscounts: 0,
		TotalTaxes:     0,
	}
	
	// Calculate storage costs
	if tier.StoragePricing != nil {
		storageCosts, err := c.calculateStorageCosts(usage, tier.StoragePricing)
		if err != nil {
			return nil, fmt.Errorf("failed to calculate storage costs: %w", err)
		}
		breakdown.StorageCosts = storageCosts
		breakdown.Subtotal += storageCosts.Amount
	}
	
	// Calculate bandwidth costs
	if tier.BandwidthPricing != nil {
		bandwidthCosts, err := c.calculateBandwidthCosts(usage, tier.BandwidthPricing)
		if err != nil {
			return nil, fmt.Errorf("failed to calculate bandwidth costs: %w", err)
		}
		breakdown.BandwidthCosts = bandwidthCosts
		breakdown.Subtotal += bandwidthCosts.Amount
	}
	
	// Calculate API costs
	if tier.APIPricing != nil {
		apiCosts, err := c.calculateAPICosts(usage, tier.APIPricing)
		if err != nil {
			return nil, fmt.Errorf("failed to calculate API costs: %w", err)
		}
		breakdown.APICosts = apiCosts
		breakdown.Subtotal += apiCosts.Amount
	}
	
	// Apply volume discounts from pricing tier
	if len(tier.Discounts) > 0 {
		volumeDiscounts := c.calculateVolumeDiscounts(breakdown, tier.Discounts)
		breakdown.Discounts = append(breakdown.Discounts, volumeDiscounts...)
		for _, discount := range volumeDiscounts {
			breakdown.TotalDiscounts += discount.Amount
		}
	}
	
	// Calculate total
	breakdown.Total = breakdown.Subtotal - breakdown.TotalDiscounts + breakdown.TotalTaxes
	
	return breakdown, nil
}

// ApplyDiscounts applies discount codes to the cost breakdown
func (c *CostCalculatorImpl) ApplyDiscounts(ctx context.Context, costs *CostBreakdown, discounts []*Discount) (*CostBreakdown, error) {
	for _, discount := range discounts {
		if !discount.IsActive {
			continue
		}
		
		// Check if discount is still valid
		now := costs.Total // Using total as a placeholder for current time
		if discount.ValidUntil.Unix() < now {
			continue
		}
		
		// Check usage limits
		if discount.MaxUses > 0 && discount.UsedCount >= discount.MaxUses {
			continue
		}
		
		// Calculate discount amount
		discountAmount, err := c.calculateDiscountAmount(costs, discount)
		if err != nil {
			logger.Warnf("Failed to calculate discount %s: %v", discount.Code, err)
			continue
		}
		
		if discountAmount > 0 {
			lineItem := &LineItem{
				Description: fmt.Sprintf("Discount: %s", discount.Name),
				Quantity:    1,
				Unit:        "discount",
				UnitPrice:   -discountAmount,
				Amount:      -discountAmount,
				Metadata: map[string]string{
					"discount_code": discount.Code,
					"discount_type": discount.Type,
				},
			}
			
			costs.Discounts = append(costs.Discounts, lineItem)
			costs.TotalDiscounts += discountAmount
		}
	}
	
	// Recalculate total
	costs.Total = costs.Subtotal - costs.TotalDiscounts + costs.TotalTaxes
	
	return costs, nil
}

// CalculateTaxes calculates taxes based on tax configuration
func (c *CostCalculatorImpl) CalculateTaxes(ctx context.Context, costs *CostBreakdown, taxConfig *TaxConfiguration) (*CostBreakdown, error) {
	if taxConfig.TaxRate <= 0 {
		return costs, nil
	}
	
	// Calculate taxable amount (subtotal minus discounts)
	taxableAmount := costs.Subtotal - costs.TotalDiscounts
	if taxableAmount <= 0 {
		return costs, nil
	}
	
	// Calculate tax amount
	taxAmount := int64(float64(taxableAmount) * taxConfig.TaxRate)
	
	if taxAmount > 0 {
		taxLineItem := &LineItem{
			Description: taxConfig.TaxName,
			Quantity:    1,
			Unit:        "tax",
			UnitPrice:   taxAmount,
			Amount:      taxAmount,
			Metadata: map[string]string{
				"tax_rate":    fmt.Sprintf("%.2f%%", taxConfig.TaxRate*100),
				"tax_country": taxConfig.Country,
				"tax_region":  taxConfig.Region,
			},
		}
		
		costs.Taxes = append(costs.Taxes, taxLineItem)
		costs.TotalTaxes += taxAmount
	}
	
	// Recalculate total
	costs.Total = costs.Subtotal - costs.TotalDiscounts + costs.TotalTaxes
	
	return costs, nil
}

// calculateStorageCosts calculates storage costs based on usage and pricing
func (c *CostCalculatorImpl) calculateStorageCosts(usage *UsageMetrics, pricing *StoragePricing) (*LineItem, error) {
	switch pricing.Model {
	case "per_gb_month":
		return c.calculateStoragePerGBMonth(usage, pricing)
	case "tiered":
		return c.calculateStorageTiered(usage, pricing)
	default:
		return c.calculateStoragePerGBMonth(usage, pricing)
	}
}

// calculateStoragePerGBMonth calculates storage costs using per-GB-month model
func (c *CostCalculatorImpl) calculateStoragePerGBMonth(usage *UsageMetrics, pricing *StoragePricing) (*LineItem, error) {
	// Convert bytes to GB
	storageGB := float64(usage.StorageBytes) / (1024 * 1024 * 1024)
	
	// Apply free quota
	billableGB := math.Max(0, storageGB-float64(pricing.FreeQuotaGB))
	
	// Calculate cost
	cost := int64(billableGB * float64(pricing.PricePerGBMonth))
	
	// Apply minimum charge
	if cost > 0 && cost < pricing.MinimumCharge {
		cost = pricing.MinimumCharge
	}
	
	return &LineItem{
		Description: "Storage (GB-month)",
		Quantity:    int64(math.Ceil(billableGB)),
		Unit:        "GB-month",
		UnitPrice:   pricing.PricePerGBMonth,
		Amount:      cost,
		Metadata: map[string]string{
			"total_gb":     fmt.Sprintf("%.2f", storageGB),
			"billable_gb":  fmt.Sprintf("%.2f", billableGB),
			"free_quota":   fmt.Sprintf("%d", pricing.FreeQuotaGB),
		},
	}, nil
}

// calculateStorageTiered calculates storage costs using tiered pricing
func (c *CostCalculatorImpl) calculateStorageTiered(usage *UsageMetrics, pricing *StoragePricing) (*LineItem, error) {
	// This is a placeholder for tiered pricing implementation
	// In a real system, you would define pricing tiers and calculate accordingly
	return c.calculateStoragePerGBMonth(usage, pricing)
}

// calculateBandwidthCosts calculates bandwidth costs based on usage and pricing
func (c *CostCalculatorImpl) calculateBandwidthCosts(usage *UsageMetrics, pricing *BandwidthPricing) (*LineItem, error) {
	switch pricing.Model {
	case "per_gb":
		return c.calculateBandwidthPerGB(usage, pricing)
	case "tiered":
		return c.calculateBandwidthTiered(usage, pricing)
	default:
		return c.calculateBandwidthPerGB(usage, pricing)
	}
}

// calculateBandwidthPerGB calculates bandwidth costs using per-GB model
func (c *CostCalculatorImpl) calculateBandwidthPerGB(usage *UsageMetrics, pricing *BandwidthPricing) (*LineItem, error) {
	// Convert bytes to GB
	bandwidthGB := float64(usage.BandwidthBytes) / (1024 * 1024 * 1024)
	
	// Apply free quota
	billableGB := math.Max(0, bandwidthGB-float64(pricing.FreeQuotaGB))
	
	// Calculate cost
	cost := int64(billableGB * float64(pricing.PricePerGB))
	
	return &LineItem{
		Description: "Bandwidth (GB)",
		Quantity:    int64(math.Ceil(billableGB)),
		Unit:        "GB",
		UnitPrice:   pricing.PricePerGB,
		Amount:      cost,
		Metadata: map[string]string{
			"total_gb":     fmt.Sprintf("%.2f", bandwidthGB),
			"billable_gb":  fmt.Sprintf("%.2f", billableGB),
			"free_quota":   fmt.Sprintf("%d", pricing.FreeQuotaGB),
		},
	}, nil
}

// calculateBandwidthTiered calculates bandwidth costs using tiered pricing
func (c *CostCalculatorImpl) calculateBandwidthTiered(usage *UsageMetrics, pricing *BandwidthPricing) (*LineItem, error) {
	// Placeholder for tiered pricing implementation
	return c.calculateBandwidthPerGB(usage, pricing)
}

// calculateAPICosts calculates API costs based on usage and pricing
func (c *CostCalculatorImpl) calculateAPICosts(usage *UsageMetrics, pricing *APIPricing) (*LineItem, error) {
	switch pricing.Model {
	case "per_call":
		return c.calculateAPIPerCall(usage, pricing)
	case "per_thousand":
		return c.calculateAPIPerThousand(usage, pricing)
	case "tiered":
		return c.calculateAPITiered(usage, pricing)
	default:
		return c.calculateAPIPerThousand(usage, pricing)
	}
}

// calculateAPIPerCall calculates API costs using per-call model
func (c *CostCalculatorImpl) calculateAPIPerCall(usage *UsageMetrics, pricing *APIPricing) (*LineItem, error) {
	// Apply free quota
	billableCalls := math.Max(0, float64(usage.APICalls)-float64(pricing.FreeQuotaCalls))
	
	// Calculate cost
	cost := int64(billableCalls * float64(pricing.PricePerCall))
	
	return &LineItem{
		Description: "API Calls",
		Quantity:    int64(billableCalls),
		Unit:        "calls",
		UnitPrice:   pricing.PricePerCall,
		Amount:      cost,
		Metadata: map[string]string{
			"total_calls":    fmt.Sprintf("%d", usage.APICalls),
			"billable_calls": fmt.Sprintf("%.0f", billableCalls),
			"free_quota":     fmt.Sprintf("%d", pricing.FreeQuotaCalls),
		},
	}, nil
}

// calculateAPIPerThousand calculates API costs using per-thousand model
func (c *CostCalculatorImpl) calculateAPIPerThousand(usage *UsageMetrics, pricing *APIPricing) (*LineItem, error) {
	// Apply free quota
	billableCalls := math.Max(0, float64(usage.APICalls)-float64(pricing.FreeQuotaCalls))
	
	// Calculate thousands of calls
	thousandCalls := billableCalls / 1000.0
	
	// Calculate cost
	cost := int64(thousandCalls * float64(pricing.PricePerThousand))
	
	return &LineItem{
		Description: "API Calls (per 1000)",
		Quantity:    int64(math.Ceil(thousandCalls)),
		Unit:        "thousand calls",
		UnitPrice:   pricing.PricePerThousand,
		Amount:      cost,
		Metadata: map[string]string{
			"total_calls":      fmt.Sprintf("%d", usage.APICalls),
			"billable_calls":   fmt.Sprintf("%.0f", billableCalls),
			"thousand_calls":   fmt.Sprintf("%.2f", thousandCalls),
			"free_quota":       fmt.Sprintf("%d", pricing.FreeQuotaCalls),
		},
	}, nil
}

// calculateAPITiered calculates API costs using tiered pricing
func (c *CostCalculatorImpl) calculateAPITiered(usage *UsageMetrics, pricing *APIPricing) (*LineItem, error) {
	// Placeholder for tiered pricing implementation
	return c.calculateAPIPerThousand(usage, pricing)
}

// calculateVolumeDiscounts calculates volume-based discounts
func (c *CostCalculatorImpl) calculateVolumeDiscounts(costs *CostBreakdown, discounts []*VolumeDiscount) []*LineItem {
	var lineItems []*LineItem
	
	for _, discount := range discounts {
		discountAmount := c.calculateVolumeDiscountAmount(costs, discount)
		if discountAmount > 0 {
			lineItem := &LineItem{
				Description: fmt.Sprintf("Volume Discount: %s", discount.Name),
				Quantity:    1,
				Unit:        "discount",
				UnitPrice:   -discountAmount,
				Amount:      -discountAmount,
				Metadata: map[string]string{
					"discount_type": "volume",
					"threshold":     fmt.Sprintf("%d", discount.Threshold),
					"discount_pct":  fmt.Sprintf("%.1f%%", discount.DiscountPct*100),
				},
			}
			lineItems = append(lineItems, lineItem)
		}
	}
	
	return lineItems
}

// calculateVolumeDiscountAmount calculates the discount amount for a volume discount
func (c *CostCalculatorImpl) calculateVolumeDiscountAmount(costs *CostBreakdown, discount *VolumeDiscount) int64 {
	var applicableAmount int64
	
	switch discount.AppliesTo {
	case "storage":
		if costs.StorageCosts != nil {
			applicableAmount = costs.StorageCosts.Amount
		}
	case "bandwidth":
		if costs.BandwidthCosts != nil {
			applicableAmount = costs.BandwidthCosts.Amount
		}
	case "api":
		if costs.APICosts != nil {
			applicableAmount = costs.APICosts.Amount
		}
	case "total":
		applicableAmount = costs.Subtotal
	}
	
	// Check if threshold is met
	if applicableAmount < discount.Threshold {
		return 0
	}
	
	// Calculate discount amount
	discountAmount := int64(float64(applicableAmount) * discount.DiscountPct)
	
	// Apply maximum discount limit
	if discount.MaxDiscount > 0 && discountAmount > discount.MaxDiscount {
		discountAmount = discount.MaxDiscount
	}
	
	return discountAmount
}

// calculateDiscountAmount calculates the discount amount for a discount code
func (c *CostCalculatorImpl) calculateDiscountAmount(costs *CostBreakdown, discount *Discount) (int64, error) {
	var applicableAmount int64
	
	switch discount.AppliesTo {
	case "storage":
		if costs.StorageCosts != nil {
			applicableAmount = costs.StorageCosts.Amount
		}
	case "bandwidth":
		if costs.BandwidthCosts != nil {
			applicableAmount = costs.BandwidthCosts.Amount
		}
	case "api":
		if costs.APICosts != nil {
			applicableAmount = costs.APICosts.Amount
		}
	case "total":
		applicableAmount = costs.Subtotal
	default:
		applicableAmount = costs.Subtotal
	}
	
	if applicableAmount <= 0 {
		return 0, nil
	}
	
	var discountAmount int64
	
	switch discount.Type {
	case "percentage":
		// Value is percentage (1-100)
		discountAmount = int64(float64(applicableAmount) * float64(discount.Value) / 100.0)
	case "fixed_amount":
		// Value is fixed amount in smallest currency unit
		discountAmount = discount.Value
		if discountAmount > applicableAmount {
			discountAmount = applicableAmount
		}
	default:
		return 0, fmt.Errorf("unknown discount type: %s", discount.Type)
	}
	
	return discountAmount, nil
}

// MemoryCostCalculator is an in-memory implementation for testing
type MemoryCostCalculator struct {
	config *Config
}

// NewMemoryCostCalculator creates a new in-memory cost calculator
func NewMemoryCostCalculator(config *Config) *MemoryCostCalculator {
	return &MemoryCostCalculator{
		config: config,
	}
}

// CalculateUsageCosts implements CostCalculator interface with simplified logic
func (m *MemoryCostCalculator) CalculateUsageCosts(ctx context.Context, usage *UsageMetrics, tier *PricingTier) (*CostBreakdown, error) {
	// Simplified calculation for testing
	storageGB := float64(usage.StorageBytes) / (1024 * 1024 * 1024)
	storageCost := int64(storageGB * 100) // $1.00 per GB
	
	bandwidthGB := float64(usage.BandwidthBytes) / (1024 * 1024 * 1024)
	bandwidthCost := int64(bandwidthGB * 50) // $0.50 per GB
	
	apiCost := usage.APICalls * 1 // $0.01 per API call
	
	breakdown := &CostBreakdown{
		StorageCosts: &LineItem{
			Description: "Storage",
			Quantity:    int64(storageGB),
			Unit:        "GB",
			UnitPrice:   100,
			Amount:      storageCost,
		},
		BandwidthCosts: &LineItem{
			Description: "Bandwidth",
			Quantity:    int64(bandwidthGB),
			Unit:        "GB",
			UnitPrice:   50,
			Amount:      bandwidthCost,
		},
		APICosts: &LineItem{
			Description: "API Calls",
			Quantity:    usage.APICalls,
			Unit:        "calls",
			UnitPrice:   1,
			Amount:      apiCost,
		},
		Subtotal:       storageCost + bandwidthCost + apiCost,
		TotalDiscounts: 0,
		TotalTaxes:     0,
		Currency:       tier.Currency,
	}
	
	breakdown.Total = breakdown.Subtotal
	
	return breakdown, nil
}

// ApplyDiscounts implements CostCalculator interface
func (m *MemoryCostCalculator) ApplyDiscounts(ctx context.Context, costs *CostBreakdown, discounts []*Discount) (*CostBreakdown, error) {
	// Simplified discount application for testing
	for _, discount := range discounts {
		if discount.Type == "percentage" {
			discountAmount := int64(float64(costs.Subtotal) * float64(discount.Value) / 100.0)
			costs.TotalDiscounts += discountAmount
		}
	}
	
	costs.Total = costs.Subtotal - costs.TotalDiscounts + costs.TotalTaxes
	return costs, nil
}

// CalculateTaxes implements CostCalculator interface
func (m *MemoryCostCalculator) CalculateTaxes(ctx context.Context, costs *CostBreakdown, taxConfig *TaxConfiguration) (*CostBreakdown, error) {
	// Simplified tax calculation for testing
	if taxConfig.TaxRate > 0 {
		taxableAmount := costs.Subtotal - costs.TotalDiscounts
		taxAmount := int64(float64(taxableAmount) * taxConfig.TaxRate)
		costs.TotalTaxes = taxAmount
	}
	
	costs.Total = costs.Subtotal - costs.TotalDiscounts + costs.TotalTaxes
	return costs, nil
}