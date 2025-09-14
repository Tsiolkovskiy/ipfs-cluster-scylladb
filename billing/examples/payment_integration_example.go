package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/ipfs-cluster/ipfs-cluster/billing"
)

// PaymentIntegrationExample demonstrates how to use the payment integration system
func main() {
	// Initialize billing storage
	storage := billing.NewMemoryBillingStorage()
	
	// Configure billing service
	config := billing.DefaultConfig()
	config.Enabled = true
	config.PaymentProcessingEnabled = true
	config.SupportedPaymentMethods = []string{"stripe", "filecoin", "polygon", "arbitrum"}
	
	// Configure payment providers
	paymentConfig := &billing.PaymentConfig{
		// Stripe configuration
		StripeSecretKey:      "sk_test_your_stripe_key",
		StripeWebhookSecret:  "whsec_your_webhook_secret",
		StripePublishableKey: "pk_test_your_publishable_key",
		
		// Filecoin configuration
		FilecoinNodeURL:    "https://api.node.glif.io",
		FilecoinWalletAddr: "f1your_wallet_address",
		FilecoinPrivateKey: "your_private_key",
		
		// Ethereum/L2 configuration
		EthereumRPCURL:      "https://mainnet.infura.io/v3/YOUR_PROJECT_ID",
		PolygonRPCURL:       "https://polygon-rpc.com",
		ArbitrumRPCURL:      "https://arb1.arbitrum.io/rpc",
		PaymentContractAddr: "0x123456789abcdef",
		WalletPrivateKey:    "your_wallet_private_key",
		
		// General settings
		ConfirmationBlocks: 12,
		PaymentTimeout:     time.Minute * 10,
	}
	
	// Initialize payment processor
	paymentProcessor, err := billing.NewPaymentProcessor(config, paymentConfig, storage)
	if err != nil {
		log.Fatalf("Failed to initialize payment processor: %v", err)
	}
	
	// Initialize payment integration service
	integrationService := billing.NewPaymentIntegrationService(config, paymentProcessor, storage)
	defer integrationService.Shutdown()
	
	// Create a sample invoice
	invoice := &billing.Invoice{
		ID:          "inv_example_123",
		Number:      "INV-2024-001",
		OwnerID:     "user_123",
		TenantID:    "tenant_123",
		TotalAmount: 10000, // $100.00 in cents
		Currency:    "USD",
		Status:      billing.InvoiceStatusSent,
		DueDate:     time.Now().Add(30 * 24 * time.Hour),
		Customer: &billing.Customer{
			ID:      "cust_123",
			OwnerID: "user_123",
			Name:    "John Doe",
			Email:   "john@example.com",
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	
	// Store the invoice
	err = storage.StoreInvoice(context.Background(), invoice)
	if err != nil {
		log.Fatalf("Failed to store invoice: %v", err)
	}
	
	fmt.Printf("Created invoice: %s for amount %d %s\n", 
		invoice.Number, invoice.TotalAmount, invoice.Currency)
	
	// Example 1: Process Stripe payment
	fmt.Println("\n=== Stripe Payment Example ===")
	stripePaymentMethod := &billing.PaymentMethod{
		ID:       "pm_stripe_123",
		Type:     "card",
		Provider: "stripe",
		Details: map[string]string{
			"card_number": "**** **** **** 4242",
			"exp_month":   "12",
			"exp_year":    "2025",
		},
	}
	
	result, err := integrationService.ProcessInvoicePayment(
		context.Background(), 
		invoice.ID, 
		stripePaymentMethod,
	)
	if err != nil {
		log.Printf("Stripe payment failed: %v", err)
	} else {
		fmt.Printf("Stripe payment processed: ID=%s, Status=%s, Amount=%d %s\n",
			result.PaymentID, result.Status, result.Amount, result.Currency)
	}
	
	// Example 2: Process Filecoin payment
	fmt.Println("\n=== Filecoin Payment Example ===")
	
	// Create a new invoice for Filecoin payment
	filecoinInvoice := &billing.Invoice{
		ID:          "inv_filecoin_123",
		Number:      "INV-FIL-2024-001",
		OwnerID:     "user_123",
		TenantID:    "tenant_123",
		TotalAmount: 1000000000000000000, // 1 FIL in attoFIL
		Currency:    "FIL",
		Status:      billing.InvoiceStatusSent,
		DueDate:     time.Now().Add(30 * 24 * time.Hour),
		Customer:    invoice.Customer,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	
	err = storage.StoreInvoice(context.Background(), filecoinInvoice)
	if err != nil {
		log.Fatalf("Failed to store Filecoin invoice: %v", err)
	}
	
	filecoinPaymentMethod := &billing.PaymentMethod{
		ID:       "pm_filecoin_123",
		Type:     "crypto",
		Provider: "filecoin",
		Details: map[string]string{
			"from_address": "f1sender_address",
			"amount_fil":   "1.0",
		},
	}
	
	result, err = integrationService.ProcessInvoicePayment(
		context.Background(), 
		filecoinInvoice.ID, 
		filecoinPaymentMethod,
	)
	if err != nil {
		log.Printf("Filecoin payment failed: %v", err)
	} else {
		fmt.Printf("Filecoin payment processed: ID=%s, Status=%s, Amount=%d %s\n",
			result.PaymentID, result.Status, result.Amount, result.Currency)
		fmt.Printf("Transaction ID: %s\n", result.TransactionID)
		fmt.Printf("Channel Address: %s\n", result.Metadata["channel_address"])
	}
	
	// Example 3: Process Polygon payment
	fmt.Println("\n=== Polygon Payment Example ===")
	
	// Create a new invoice for Polygon payment
	polygonInvoice := &billing.Invoice{
		ID:          "inv_polygon_123",
		Number:      "INV-MATIC-2024-001",
		OwnerID:     "user_123",
		TenantID:    "tenant_123",
		TotalAmount: 1000000000000000000, // 1 MATIC in wei
		Currency:    "MATIC",
		Status:      billing.InvoiceStatusSent,
		DueDate:     time.Now().Add(30 * 24 * time.Hour),
		Customer:    invoice.Customer,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	
	err = storage.StoreInvoice(context.Background(), polygonInvoice)
	if err != nil {
		log.Fatalf("Failed to store Polygon invoice: %v", err)
	}
	
	polygonPaymentMethod := &billing.PaymentMethod{
		ID:       "pm_polygon_123",
		Type:     "crypto",
		Provider: "polygon",
		Details: map[string]string{
			"transaction_hash": "0xabc123def456",
			"from_address":     "0x742d35Cc6634C0532925a3b8D4C9db96590c6C87",
		},
	}
	
	result, err = integrationService.ProcessInvoicePayment(
		context.Background(), 
		polygonInvoice.ID, 
		polygonPaymentMethod,
	)
	if err != nil {
		log.Printf("Polygon payment failed: %v", err)
	} else {
		fmt.Printf("Polygon payment processed: ID=%s, Status=%s, Amount=%d %s\n",
			result.PaymentID, result.Status, result.Amount, result.Currency)
		fmt.Printf("Transaction ID: %s\n", result.TransactionID)
		fmt.Printf("Network: %s\n", result.Metadata["network"])
	}
	
	// Example 4: Handle payment webhook
	fmt.Println("\n=== Payment Webhook Example ===")
	
	webhook := &billing.PaymentWebhook{
		ID:        "evt_webhook_123",
		Provider:  "stripe",
		EventType: "payment_intent.succeeded",
		Data: map[string]interface{}{
			"id":     "pi_123456",
			"amount": 10000,
			"status": "succeeded",
		},
		Signature: "valid_signature",
		Timestamp: time.Now(),
	}
	
	err = integrationService.HandlePaymentWebhook(context.Background(), webhook)
	if err != nil {
		log.Printf("Webhook processing failed: %v", err)
	} else {
		fmt.Printf("Webhook processed successfully: %s\n", webhook.EventType)
	}
	
	// Example 5: Process refund
	fmt.Println("\n=== Payment Refund Example ===")
	
	refundResult, err := paymentProcessor.RefundPayment(
		context.Background(), 
		"pay_example_123", 
		5000, // Refund $50.00
	)
	if err != nil {
		log.Printf("Refund failed: %v", err)
	} else {
		fmt.Printf("Refund processed: ID=%s, Amount=%d, Status=%s\n",
			refundResult.RefundID, refundResult.Amount, refundResult.Status)
	}
	
	fmt.Println("\n=== Payment Integration Example Complete ===")
}