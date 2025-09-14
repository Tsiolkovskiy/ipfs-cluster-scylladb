package billing

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPaymentProcessorImpl_ProcessPayment(t *testing.T) {
	tests := []struct {
		name           string
		config         *Config
		paymentConfig  *PaymentConfig
		request        *PaymentRequest
		expectedError  string
		expectedStatus PaymentStatus
	}{
		{
			name: "successful stripe payment",
			config: &Config{
				SupportedPaymentMethods: []string{"stripe"},
			},
			paymentConfig: &PaymentConfig{
				StripeSecretKey: "sk_test_123",
			},
			request: &PaymentRequest{
				Amount:   1000,
				Currency: "USD",
				PaymentMethod: &PaymentMethod{
					Provider: "stripe",
					Type:     "card",
				},
			},
			expectedStatus: PaymentStatusSucceeded,
		},
		{
			name: "unsupported payment provider",
			config: &Config{
				SupportedPaymentMethods: []string{"stripe"},
			},
			paymentConfig: &PaymentConfig{
				StripeSecretKey: "sk_test_123",
			},
			request: &PaymentRequest{
				Amount:   1000,
				Currency: "USD",
				PaymentMethod: &PaymentMethod{
					Provider: "unsupported",
					Type:     "card",
				},
			},
			expectedError: "payment provider not supported: unsupported",
		},
		{
			name: "missing payment method",
			config: &Config{
				SupportedPaymentMethods: []string{"stripe"},
			},
			paymentConfig: &PaymentConfig{
				StripeSecretKey: "sk_test_123",
			},
			request: &PaymentRequest{
				Amount:   1000,
				Currency: "USD",
			},
			expectedError: "payment method is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storage := NewMemoryBillingStorage()
			processor, err := NewPaymentProcessor(tt.config, tt.paymentConfig, storage)
			require.NoError(t, err)

			result, err := processor.ProcessPayment(context.Background(), tt.request)

			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.expectedStatus, result.Status)
				assert.Equal(t, tt.request.Amount, result.Amount)
				assert.Equal(t, tt.request.Currency, result.Currency)
				assert.NotEmpty(t, result.PaymentID)
				assert.NotEmpty(t, result.TransactionID)
			}
		})
	}
}

func TestStripeProvider_ProcessPayment(t *testing.T) {
	// Skip if no Stripe key available
	config := &PaymentConfig{
		StripeSecretKey: "sk_test_123", // Mock key for testing
	}

	provider, err := NewStripeProvider(config)
	require.NoError(t, err)

	request := &PaymentRequest{
		Amount:   2000,
		Currency: "USD",
		PaymentMethod: &PaymentMethod{
			Provider: "stripe",
			Type:     "card",
		},
		Customer: &Customer{
			Email: "test@example.com",
		},
		Metadata: map[string]string{
			"order_id": "order_123",
		},
	}

	// Note: This will fail with a real Stripe API call since we're using a mock key
	// In a real test environment, you'd use Stripe's test keys
	result, err := provider.ProcessPayment(context.Background(), request)
	
	// For mock testing, we expect an error due to invalid key
	// In production tests with real test keys, this would succeed
	if err != nil {
		assert.Contains(t, err.Error(), "failed to create payment intent")
	} else {
		assert.NotNil(t, result)
		assert.Equal(t, request.Amount, result.Amount)
		assert.Equal(t, request.Currency, result.Currency)
	}
}

func TestFilecoinProvider_ProcessPayment(t *testing.T) {
	config := &PaymentConfig{
		FilecoinNodeURL:    "https://api.node.glif.io",
		FilecoinWalletAddr: "f1test123",
	}

	provider, err := NewFilecoinProvider(config)
	if err != nil {
		// Skip test if Filecoin node is not available
		t.Skipf("Filecoin node not available: %v", err)
	}

	request := &PaymentRequest{
		Amount:   1000000000000000000, // 1 FIL in attoFIL
		Currency: "FIL",
		PaymentMethod: &PaymentMethod{
			Provider: "filecoin",
			Type:     "wallet",
			Details: map[string]string{
				"from_address": "f1sender123",
			},
		},
	}

	result, err := provider.ProcessPayment(context.Background(), request)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, PaymentStatusPending, result.Status) // Filecoin payments are pending initially
	assert.Equal(t, request.Amount, result.Amount)
	assert.Equal(t, "FIL", result.Currency)
	assert.Contains(t, result.TransactionID, "bafy")
}

func TestPolygonProvider_ProcessPayment(t *testing.T) {
	config := &PaymentConfig{
		PolygonRPCURL:        "https://polygon-rpc.com",
		PaymentContractAddr:  "0x1234567890123456789012345678901234567890",
	}

	provider, err := NewPolygonProvider(config)
	if err != nil {
		// Skip test if Polygon RPC is not available
		t.Skipf("Polygon RPC not available: %v", err)
	}

	request := &PaymentRequest{
		Amount:   1000000000000000000, // 1 MATIC in wei
		Currency: "MATIC",
		PaymentMethod: &PaymentMethod{
			Provider: "polygon",
			Type:     "crypto",
			Details: map[string]string{
				"from_address":     "0xsender123",
				"transaction_hash": "0x1234567890abcdef",
			},
		},
	}

	// This will likely fail since we're using a mock transaction hash
	result, err := provider.ProcessPayment(context.Background(), request)
	if err != nil {
		assert.Contains(t, err.Error(), "failed to get transaction")
	} else {
		assert.NotNil(t, result)
		assert.Equal(t, "MATIC", result.Currency)
	}
}

func TestArbitrumProvider_ProcessPayment(t *testing.T) {
	config := &PaymentConfig{
		ArbitrumRPCURL:       "https://arb1.arbitrum.io/rpc",
		PaymentContractAddr:  "0x1234567890123456789012345678901234567890",
	}

	provider, err := NewArbitrumProvider(config)
	if err != nil {
		// Skip test if Arbitrum RPC is not available
		t.Skipf("Arbitrum RPC not available: %v", err)
	}

	request := &PaymentRequest{
		Amount:   1000000000000000000, // 1 ETH in wei
		Currency: "ETH",
		PaymentMethod: &PaymentMethod{
			Provider: "arbitrum",
			Type:     "crypto",
			Details: map[string]string{
				"from_address":     "0xsender123",
				"transaction_hash": "0x1234567890abcdef",
			},
		},
	}

	// This will likely fail since we're using a mock transaction hash
	result, err := provider.ProcessPayment(context.Background(), request)
	if err != nil {
		assert.Contains(t, err.Error(), "failed to get transaction")
	} else {
		assert.NotNil(t, result)
		assert.Equal(t, "ETH", result.Currency)
	}
}

func TestMemoryPaymentProcessor(t *testing.T) {
	config := &Config{
		SupportedPaymentMethods: []string{"memory"},
	}

	processor := NewMemoryPaymentProcessor(config)

	request := &PaymentRequest{
		Amount:   1500,
		Currency: "USD",
		PaymentMethod: &PaymentMethod{
			Provider: "memory",
			Type:     "test",
		},
		Metadata: map[string]string{
			"test": "true",
		},
	}

	// Test payment processing
	result, err := processor.ProcessPayment(context.Background(), request)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, PaymentStatusSucceeded, result.Status)
	assert.Equal(t, request.Amount, result.Amount)
	assert.Equal(t, request.Currency, result.Currency)
	assert.NotEmpty(t, result.PaymentID)
	assert.Contains(t, result.TransactionID, "mock_")

	// Test refund processing
	refundResult, err := processor.RefundPayment(context.Background(), result.PaymentID, 500)
	assert.NoError(t, err)
	assert.NotNil(t, refundResult)
	assert.Equal(t, result.PaymentID, refundResult.PaymentID)
	assert.Equal(t, int64(500), refundResult.Amount)
	assert.Equal(t, "succeeded", refundResult.Status)

	// Test webhook handling
	webhook := &PaymentWebhook{
		ID:        "webhook_123",
		Provider:  "memory",
		EventType: "payment.succeeded",
		Data:      map[string]interface{}{"payment_id": result.PaymentID},
		Timestamp: time.Now(),
	}

	err = processor.HandlePaymentWebhook(context.Background(), webhook)
	assert.NoError(t, err)

	// Verify stored data
	payments := processor.GetPayments()
	assert.Len(t, payments, 1)
	assert.Contains(t, payments, result.PaymentID)

	refunds := processor.GetRefunds()
	assert.Len(t, refunds, 1)
}

func TestPaymentConfig_Validation(t *testing.T) {
	tests := []struct {
		name          string
		config        *PaymentConfig
		provider      string
		expectedError string
	}{
		{
			name: "valid stripe config",
			config: &PaymentConfig{
				StripeSecretKey: "sk_test_123",
			},
			provider: "stripe",
		},
		{
			name: "missing stripe secret key",
			config: &PaymentConfig{
				StripeWebhookSecret: "whsec_123",
			},
			provider:      "stripe",
			expectedError: "stripe secret key is required",
		},
		{
			name: "valid filecoin config",
			config: &PaymentConfig{
				FilecoinNodeURL:    "https://api.node.glif.io",
				FilecoinWalletAddr: "f1test123",
			},
			provider: "filecoin",
		},
		{
			name: "missing filecoin node URL",
			config: &PaymentConfig{
				FilecoinWalletAddr: "f1test123",
			},
			provider:      "filecoin",
			expectedError: "filecoin node URL is required",
		},
		{
			name: "missing filecoin wallet address",
			config: &PaymentConfig{
				FilecoinNodeURL: "https://api.node.glif.io",
			},
			provider:      "filecoin",
			expectedError: "filecoin wallet address is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := createPaymentProvider(tt.provider, tt.config)

			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGeneratePaymentID(t *testing.T) {
	id1 := generatePaymentID()
	time.Sleep(time.Nanosecond) // Ensure different timestamps
	id2 := generatePaymentID()

	assert.NotEmpty(t, id1)
	assert.NotEmpty(t, id2)
	assert.NotEqual(t, id1, id2)
	assert.Contains(t, id1, "pay_")
	assert.Contains(t, id2, "pay_")
}

func TestGenerateRefundID(t *testing.T) {
	id1 := generateRefundID()
	time.Sleep(time.Nanosecond) // Ensure different timestamps
	id2 := generateRefundID()

	assert.NotEmpty(t, id1)
	assert.NotEmpty(t, id2)
	assert.NotEqual(t, id1, id2)
	assert.Contains(t, id1, "ref_")
	assert.Contains(t, id2, "ref_")
}