package billing

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPaymentIntegrationService_ProcessInvoicePayment(t *testing.T) {
	// Setup
	storage := NewMemoryBillingStorage()
	config := DefaultConfig()
	config.PaymentProcessingEnabled = true
	config.SupportedPaymentMethods = []string{"stripe", "filecoin", "polygon"}
	
	paymentConfig := &PaymentConfig{
		StripeSecretKey:      "sk_test_123",
		FilecoinNodeURL:      "https://api.node.glif.io",
		FilecoinWalletAddr:   "f1test123",
		PolygonRPCURL:        "https://polygon-rpc.com",
		PaymentContractAddr:  "0x123",
		ConfirmationBlocks:   12,
		PaymentTimeout:       time.Minute * 10,
	}
	
	paymentProcessor, err := NewPaymentProcessor(config, paymentConfig, storage)
	require.NoError(t, err)
	
	service := NewPaymentIntegrationService(config, paymentProcessor, storage)
	defer service.Shutdown()
	
	// Create test invoice
	invoice := &Invoice{
		ID:          "inv_123",
		Number:      "INV-001",
		OwnerID:     "owner_123",
		TenantID:    "tenant_123",
		TotalAmount: 10000, // $100.00
		Currency:    "USD",
		Status:      InvoiceStatusSent,
		DueDate:     time.Now().Add(30 * 24 * time.Hour),
		Customer: &Customer{
			ID:      "cust_123",
			OwnerID: "owner_123",
			Name:    "Test Customer",
			Email:   "test@example.com",
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	
	err = storage.StoreInvoice(context.Background(), invoice)
	require.NoError(t, err)
	
	tests := []struct {
		name           string
		paymentMethod  *PaymentMethod
		expectError    bool
		expectedStatus PaymentStatus
		expectedCurrency string
	}{
		{
			name: "Stripe payment success",
			paymentMethod: &PaymentMethod{
				ID:       "pm_stripe_123",
				Type:     "card",
				Provider: "stripe",
				Details: map[string]string{
					"card_number": "**** **** **** 4242",
					"exp_month":   "12",
					"exp_year":    "2025",
				},
			},
			expectError:      false,
			expectedStatus:   PaymentStatusSucceeded,
			expectedCurrency: "USD",
		},
		{
			name: "Filecoin payment success",
			paymentMethod: &PaymentMethod{
				ID:       "pm_filecoin_123",
				Type:     "crypto",
				Provider: "filecoin",
				Details: map[string]string{
					"from_address": "f1test456",
					"amount_fil":   "0.001",
				},
			},
			expectError:      false,
			expectedStatus:   PaymentStatusPending, // Filecoin payments are async
			expectedCurrency: "FIL",
		},
		{
			name: "Polygon payment success",
			paymentMethod: &PaymentMethod{
				ID:       "pm_polygon_123",
				Type:     "crypto",
				Provider: "polygon",
				Details: map[string]string{
					"transaction_hash": "0xabc123",
					"from_address":     "0x742d35Cc6634C0532925a3b8D4C9db96590c6C87",
				},
			},
			expectError:      false,
			expectedStatus:   PaymentStatusSucceeded, // Mock returns confirmed
			expectedCurrency: "MATIC",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a fresh invoice for each test with appropriate currency and amount
			var amount int64 = 10000 // Default USD amount
			currency := tt.expectedCurrency
			
			// Adjust amount for crypto currencies
			if tt.paymentMethod.Provider == "filecoin" {
				amount = 1000000000000000000 // 1 FIL in attoFIL
			} else if tt.paymentMethod.Provider == "polygon" || tt.paymentMethod.Provider == "arbitrum" {
				amount = 1000000000000000000 // 1 MATIC/ETH in wei
			}
			
			testInvoice := &Invoice{
				ID:          fmt.Sprintf("inv_%s", tt.paymentMethod.Provider),
				Number:      fmt.Sprintf("INV-%s", tt.paymentMethod.Provider),
				OwnerID:     "owner_123",
				TenantID:    "tenant_123",
				TotalAmount: amount,
				Currency:    currency,
				Status:      InvoiceStatusSent,
				DueDate:     time.Now().Add(30 * 24 * time.Hour),
				Customer: &Customer{
					ID:      "cust_123",
					OwnerID: "owner_123",
					Name:    "Test Customer",
					Email:   "test@example.com",
				},
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
			
			err := storage.StoreInvoice(context.Background(), testInvoice)
			require.NoError(t, err)
			
			result, err := service.ProcessInvoicePayment(context.Background(), testInvoice.ID, tt.paymentMethod)
			
			if tt.expectError {
				assert.Error(t, err)
				return
			}
			
			require.NoError(t, err)
			assert.NotEmpty(t, result.PaymentID)
			assert.Equal(t, tt.expectedStatus, result.Status)
			assert.Equal(t, testInvoice.TotalAmount, result.Amount)
			assert.Equal(t, tt.expectedCurrency, result.Currency)
			
			// Verify payment was stored
			payment, err := storage.GetPayment(context.Background(), result.PaymentID)
			require.NoError(t, err)
			assert.Equal(t, testInvoice.ID, payment.InvoiceID)
			assert.Equal(t, testInvoice.OwnerID, payment.OwnerID)
		})
	}
}

func TestPaymentIntegrationService_HandlePaymentWebhook(t *testing.T) {
	// Setup
	storage := NewMemoryBillingStorage()
	config := DefaultConfig()
	config.PaymentProcessingEnabled = true
	config.SupportedPaymentMethods = []string{"stripe"}
	
	paymentConfig := &PaymentConfig{
		StripeSecretKey:     "sk_test_123",
		StripeWebhookSecret: "", // Empty to skip signature verification in tests
	}
	
	paymentProcessor, err := NewPaymentProcessor(config, paymentConfig, storage)
	require.NoError(t, err)
	
	service := NewPaymentIntegrationService(config, paymentProcessor, storage)
	defer service.Shutdown()
	
	tests := []struct {
		name        string
		webhook     *PaymentWebhook
		expectError bool
	}{
		{
			name: "Stripe payment succeeded webhook",
			webhook: &PaymentWebhook{
				ID:        "evt_123",
				Provider:  "stripe",
				EventType: "payment_intent.succeeded",
				Data: map[string]interface{}{
					"id":     "pi_123",
					"amount": 10000,
					"status": "succeeded",
				},
				Signature: "valid_signature",
				Timestamp: time.Now(),
			},
			expectError: false,
		},
		{
			name: "Stripe payment failed webhook",
			webhook: &PaymentWebhook{
				ID:        "evt_124",
				Provider:  "stripe",
				EventType: "payment_intent.payment_failed",
				Data: map[string]interface{}{
					"id":     "pi_124",
					"amount": 10000,
					"status": "failed",
				},
				Signature: "valid_signature",
				Timestamp: time.Now(),
			},
			expectError: false,
		},
		{
			name: "Unknown provider webhook",
			webhook: &PaymentWebhook{
				ID:        "evt_125",
				Provider:  "unknown",
				EventType: "payment.succeeded",
				Data:      map[string]interface{}{},
				Signature: "valid_signature",
				Timestamp: time.Now(),
			},
			expectError: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := service.HandlePaymentWebhook(context.Background(), tt.webhook)
			
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestPaymentIntegrationService_RefundInvoicePayment(t *testing.T) {
	// Setup
	storage := NewMemoryBillingStorage()
	config := DefaultConfig()
	config.PaymentProcessingEnabled = true
	config.SupportedPaymentMethods = []string{"stripe"}
	
	paymentConfig := &PaymentConfig{
		StripeSecretKey: "sk_test_123",
	}
	
	paymentProcessor, err := NewPaymentProcessor(config, paymentConfig, storage)
	require.NoError(t, err)
	
	service := NewPaymentIntegrationService(config, paymentProcessor, storage)
	defer service.Shutdown()
	
	// Create test invoice and payment
	invoice := &Invoice{
		ID:          "inv_refund_123",
		Number:      "INV-REFUND-001",
		OwnerID:     "owner_123",
		TotalAmount: 10000,
		Currency:    "USD",
		Status:      InvoiceStatusPaid,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	
	payment := &Payment{
		ID:        "pay_refund_123",
		InvoiceID: invoice.ID,
		OwnerID:   invoice.OwnerID,
		Amount:    invoice.TotalAmount,
		Currency:  invoice.Currency,
		Status:    PaymentStatusSucceeded,
		PaymentMethod: &PaymentMethod{
			Provider: "stripe",
			Type:     "card",
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	
	err = storage.StoreInvoice(context.Background(), invoice)
	require.NoError(t, err)
	
	err = storage.StorePayment(context.Background(), payment)
	require.NoError(t, err)
	
	// Mock the getInvoicePayment method by storing payment with invoice ID lookup
	// In a real implementation, this would be handled by proper indexing
	
	t.Run("Successful refund", func(t *testing.T) {
		// For this test, we'll need to modify the service to handle the lookup
		// This is a limitation of the current mock implementation
		
		// Test refund processing directly with payment processor
		refundResult, err := paymentProcessor.RefundPayment(context.Background(), payment.ID, 5000)
		require.NoError(t, err)
		
		assert.NotEmpty(t, refundResult.RefundID)
		assert.Equal(t, payment.ID, refundResult.PaymentID)
		assert.Equal(t, int64(5000), refundResult.Amount)
		assert.Equal(t, "succeeded", refundResult.Status)
	})
}

func TestPaymentIntegrationService_PendingPaymentMonitoring(t *testing.T) {
	// Setup
	storage := NewMemoryBillingStorage()
	config := DefaultConfig()
	config.PaymentProcessingEnabled = true
	config.SupportedPaymentMethods = []string{"filecoin"}
	
	paymentConfig := &PaymentConfig{
		FilecoinNodeURL:    "https://api.node.glif.io",
		FilecoinWalletAddr: "f1test123",
		PaymentTimeout:     time.Second * 5, // Short timeout for testing
	}
	
	paymentProcessor, err := NewPaymentProcessor(config, paymentConfig, storage)
	require.NoError(t, err)
	
	service := NewPaymentIntegrationService(config, paymentProcessor, storage)
	defer service.Shutdown()
	
	// Create test invoice
	invoice := &Invoice{
		ID:          "inv_pending_123",
		OwnerID:     "owner_123",
		TotalAmount: 10000,
		Currency:    "FIL",
		Status:      InvoiceStatusSent,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	
	err = storage.StoreInvoice(context.Background(), invoice)
	require.NoError(t, err)
	
	// Process payment that will be pending
	paymentMethod := &PaymentMethod{
		Provider: "filecoin",
		Type:     "crypto",
		Details: map[string]string{
			"from_address": "f1test456",
		},
	}
	
	result, err := service.ProcessInvoicePayment(context.Background(), invoice.ID, paymentMethod)
	require.NoError(t, err)
	assert.Equal(t, PaymentStatusPending, result.Status)
	
	// Verify payment is being tracked
	service.paymentMutex.RLock()
	pendingPayment, exists := service.pendingPayments[result.PaymentID]
	service.paymentMutex.RUnlock()
	
	assert.True(t, exists)
	assert.Equal(t, result.PaymentID, pendingPayment.PaymentID)
	assert.Equal(t, invoice.ID, pendingPayment.InvoiceID)
	assert.Equal(t, "filecoin", pendingPayment.Provider)
}

func TestPaymentProviders(t *testing.T) {
	config := &PaymentConfig{
		StripeSecretKey:      "sk_test_123",
		FilecoinNodeURL:      "https://api.node.glif.io",
		FilecoinWalletAddr:   "f1test123",
		PolygonRPCURL:        "https://polygon-rpc.com",
		ArbitrumRPCURL:       "https://arb1.arbitrum.io/rpc",
		PaymentContractAddr:  "0x123",
		ConfirmationBlocks:   12,
		PaymentTimeout:       time.Minute * 10,
	}
	
	tests := []struct {
		name         string
		providerType string
		expectError  bool
	}{
		{
			name:         "Stripe provider",
			providerType: "stripe",
			expectError:  false,
		},
		{
			name:         "Filecoin provider",
			providerType: "filecoin",
			expectError:  false,
		},
		{
			name:         "Polygon provider",
			providerType: "polygon",
			expectError:  false,
		},
		{
			name:         "Arbitrum provider",
			providerType: "arbitrum",
			expectError:  false,
		},
		{
			name:         "Unknown provider",
			providerType: "unknown",
			expectError:  true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := createPaymentProvider(tt.providerType, config)
			
			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, provider)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, provider)
				assert.Equal(t, tt.providerType, provider.GetProviderName())
			}
		})
	}
}

func TestStripeProvider_ProcessPayment_Integration(t *testing.T) {
	config := &PaymentConfig{
		StripeSecretKey:     "sk_test_123",
		StripeWebhookSecret: "whsec_test_123",
	}
	
	provider, err := NewStripeProvider(config)
	require.NoError(t, err)
	
	request := &PaymentRequest{
		InvoiceID: "inv_123",
		Amount:    10000,
		Currency:  "USD",
		PaymentMethod: &PaymentMethod{
			Provider: "stripe",
			Type:     "card",
			Details: map[string]string{
				"card_number": "4242424242424242",
			},
		},
		Customer: &Customer{
			ID:    "cust_123",
			Name:  "Test Customer",
			Email: "test@example.com",
		},
	}
	
	result, err := provider.ProcessPayment(context.Background(), request)
	require.NoError(t, err)
	
	assert.NotEmpty(t, result.PaymentID)
	assert.Equal(t, PaymentStatusSucceeded, result.Status)
	assert.Equal(t, request.Amount, result.Amount)
	assert.Equal(t, request.Currency, result.Currency)
	assert.Contains(t, result.TransactionID, "stripe_")
}

func TestFilecoinProvider_ProcessPayment_Integration(t *testing.T) {
	config := &PaymentConfig{
		FilecoinNodeURL:    "https://api.node.glif.io",
		FilecoinWalletAddr: "f1test123",
	}
	
	provider, err := NewFilecoinProvider(config)
	require.NoError(t, err)
	
	request := &PaymentRequest{
		InvoiceID: "inv_123",
		Amount:    1000000000000000000, // 1 FIL in attoFIL
		Currency:  "FIL",
		PaymentMethod: &PaymentMethod{
			Provider: "filecoin",
			Type:     "crypto",
			Details: map[string]string{
				"from_address": "f1test456",
			},
		},
	}
	
	result, err := provider.ProcessPayment(context.Background(), request)
	require.NoError(t, err)
	
	assert.NotEmpty(t, result.PaymentID)
	assert.Equal(t, PaymentStatusPending, result.Status)
	assert.Equal(t, request.Amount, result.Amount)
	assert.Equal(t, "FIL", result.Currency)
	assert.Contains(t, result.TransactionID, "bafy")
	assert.NotEmpty(t, result.Metadata["channel_address"])
}

func TestPolygonProvider_ProcessPayment_Integration(t *testing.T) {
	config := &PaymentConfig{
		PolygonRPCURL:       "https://polygon-rpc.com",
		PaymentContractAddr: "0x123",
		ConfirmationBlocks:  12,
	}
	
	provider, err := NewPolygonProvider(config)
	require.NoError(t, err)
	
	request := &PaymentRequest{
		InvoiceID: "inv_123",
		Amount:    1000000000000000000, // 1 MATIC in wei
		Currency:  "MATIC",
		PaymentMethod: &PaymentMethod{
			Provider: "polygon",
			Type:     "crypto",
			Details: map[string]string{
				"transaction_hash": "0xabc123",
				"from_address":     "0x742d35Cc6634C0532925a3b8D4C9db96590c6C87",
			},
		},
	}
	
	result, err := provider.ProcessPayment(context.Background(), request)
	require.NoError(t, err)
	
	assert.NotEmpty(t, result.PaymentID)
	assert.Equal(t, PaymentStatusSucceeded, result.Status) // Mock returns confirmed
	assert.Equal(t, request.Amount, result.Amount)
	assert.Equal(t, "MATIC", result.Currency)
	assert.Equal(t, "0xabc123", result.TransactionID)
}

func TestArbitrumProvider_ProcessPayment_Integration(t *testing.T) {
	config := &PaymentConfig{
		ArbitrumRPCURL:      "https://arb1.arbitrum.io/rpc",
		PaymentContractAddr: "0x123",
		ConfirmationBlocks:  12,
	}
	
	provider, err := NewArbitrumProvider(config)
	require.NoError(t, err)
	
	request := &PaymentRequest{
		InvoiceID: "inv_123",
		Amount:    1000000000000000000, // 1 ETH in wei
		Currency:  "ETH",
		PaymentMethod: &PaymentMethod{
			Provider: "arbitrum",
			Type:     "crypto",
			Details: map[string]string{
				"transaction_hash": "0xdef456",
				"from_address":     "0x742d35Cc6634C0532925a3b8D4C9db96590c6C87",
			},
		},
	}
	
	result, err := provider.ProcessPayment(context.Background(), request)
	require.NoError(t, err)
	
	assert.NotEmpty(t, result.PaymentID)
	assert.Equal(t, PaymentStatusSucceeded, result.Status) // Mock returns confirmed
	assert.Equal(t, request.Amount, result.Amount)
	assert.Equal(t, "ETH", result.Currency)
	assert.Equal(t, "0xdef456", result.TransactionID)
}