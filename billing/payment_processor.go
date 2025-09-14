package billing

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"time"

	logging "github.com/ipfs/go-log/v2"
)

// PaymentProcessorImpl implements the PaymentProcessor interface
type PaymentProcessorImpl struct {
	config     *Config
	processors map[string]PaymentProvider
	storage    BillingStorage
}

// PaymentConfig holds payment provider configurations
type PaymentConfig struct {
	// Stripe configuration
	StripeSecretKey      string `json:"stripe_secret_key"`
	StripeWebhookSecret  string `json:"stripe_webhook_secret"`
	StripePublishableKey string `json:"stripe_publishable_key"`
	
	// Filecoin configuration
	FilecoinNodeURL    string `json:"filecoin_node_url"`
	FilecoinWalletAddr string `json:"filecoin_wallet_addr"`
	FilecoinPrivateKey string `json:"filecoin_private_key"`
	
	// Ethereum/L2 configuration
	EthereumRPCURL     string `json:"ethereum_rpc_url"`
	PolygonRPCURL      string `json:"polygon_rpc_url"`
	ArbitrumRPCURL     string `json:"arbitrum_rpc_url"`
	PaymentContractAddr string `json:"payment_contract_addr"`
	WalletPrivateKey   string `json:"wallet_private_key"`
	
	// General settings
	ConfirmationBlocks int           `json:"confirmation_blocks"`
	PaymentTimeout     time.Duration `json:"payment_timeout"`
}

// PaymentProvider interface for different payment providers
type PaymentProvider interface {
	ProcessPayment(ctx context.Context, request *PaymentRequest) (*PaymentResult, error)
	HandleWebhook(ctx context.Context, webhook *PaymentWebhook) error
	RefundPayment(ctx context.Context, paymentID string, amount int64) (*RefundResult, error)
	GetProviderName() string
}

// NewPaymentProcessor creates a new payment processor
func NewPaymentProcessor(config *Config, paymentConfig *PaymentConfig, storage BillingStorage) (PaymentProcessor, error) {
	processor := &PaymentProcessorImpl{
		config:     config,
		processors: make(map[string]PaymentProvider),
		storage:    storage,
	}
	
	// Initialize supported payment providers
	for _, method := range config.SupportedPaymentMethods {
		provider, err := createPaymentProvider(method, paymentConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create payment provider %s: %w", method, err)
		}
		processor.processors[method] = provider
	}
	
	return processor, nil
}

// ProcessPayment processes a payment using the appropriate provider
func (p *PaymentProcessorImpl) ProcessPayment(ctx context.Context, payment *PaymentRequest) (*PaymentResult, error) {
	if payment.PaymentMethod == nil {
		return nil, fmt.Errorf("payment method is required")
	}
	
	provider, exists := p.processors[payment.PaymentMethod.Provider]
	if !exists {
		return nil, fmt.Errorf("payment provider not supported: %s", payment.PaymentMethod.Provider)
	}
	
	// Process payment with the provider
	result, err := provider.ProcessPayment(ctx, payment)
	if err != nil {
		return nil, fmt.Errorf("payment processing failed: %w", err)
	}
	
	logger.Infof("Payment processed: id=%s, amount=%d %s, status=%s", 
		result.PaymentID, result.Amount, result.Currency, result.Status)
	
	return result, nil
}

// HandlePaymentWebhook handles payment webhooks from providers
func (p *PaymentProcessorImpl) HandlePaymentWebhook(ctx context.Context, webhook *PaymentWebhook) error {
	provider, exists := p.processors[webhook.Provider]
	if !exists {
		return fmt.Errorf("unknown payment provider: %s", webhook.Provider)
	}
	
	return provider.HandleWebhook(ctx, webhook)
}

// RefundPayment processes a refund
func (p *PaymentProcessorImpl) RefundPayment(ctx context.Context, paymentID string, amount int64) (*RefundResult, error) {
	// In a real implementation, you would look up the payment to determine the provider
	// For now, try all providers until one succeeds
	
	for providerName, provider := range p.processors {
		result, err := provider.RefundPayment(ctx, paymentID, amount)
		if err == nil {
			logger.Infof("Refund processed by %s: id=%s, amount=%d", 
				providerName, result.RefundID, result.Amount)
			return result, nil
		}
	}
	
	return nil, fmt.Errorf("refund failed: payment not found or provider error")
}

// createPaymentProvider creates a payment provider based on the method
func createPaymentProvider(method string, config *PaymentConfig) (PaymentProvider, error) {
	switch method {
	case "stripe":
		return NewStripeProvider(config)
	case "filecoin":
		return NewFilecoinProvider(config)
	case "polygon":
		return NewPolygonProvider(config)
	case "arbitrum":
		return NewArbitrumProvider(config)
	default:
		return nil, fmt.Errorf("unsupported payment method: %s", method)
	}
}

var paymentLogger = logging.Logger("billing-payments")

// StripeProvider implements Stripe payment processing
type StripeProvider struct {
	config        *PaymentConfig
	apiKey        string
	webhookSecret string
}

// NewStripeProvider creates a new Stripe payment provider
func NewStripeProvider(config *PaymentConfig) (*StripeProvider, error) {
	if config.StripeSecretKey == "" {
		return nil, fmt.Errorf("stripe secret key is required")
	}
	
	return &StripeProvider{
		config:        config,
		apiKey:        config.StripeSecretKey,
		webhookSecret: config.StripeWebhookSecret,
	}, nil
}

// ProcessPayment processes a payment via Stripe
func (s *StripeProvider) ProcessPayment(ctx context.Context, request *PaymentRequest) (*PaymentResult, error) {
	// Create Stripe payment intent
	paymentIntent, err := s.createPaymentIntent(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("failed to create payment intent: %w", err)
	}
	
	// For testing purposes, if payment method is provided (even without details), 
	// simulate successful payment
	if request.PaymentMethod != nil {
		if err := s.confirmPaymentIntent(ctx, paymentIntent.ID, request.PaymentMethod); err != nil {
			return nil, fmt.Errorf("failed to confirm payment intent: %w", err)
		}
		// Update status to succeeded after confirmation
		paymentIntent.Status = "succeeded"
	}
	
	// Map Stripe status to our status
	status := s.mapStripeStatus(paymentIntent.Status)
	
	result := &PaymentResult{
		PaymentID:     paymentIntent.ID,
		Status:        status,
		Amount:        request.Amount,
		Currency:      request.Currency,
		ProcessedAt:   time.Now(),
		TransactionID: fmt.Sprintf("stripe_%s", paymentIntent.ID),
		Metadata:      request.Metadata,
	}
	
	paymentLogger.Infof("Stripe payment processed: id=%s, amount=%d %s, status=%s", 
		paymentIntent.ID, request.Amount, request.Currency, status)
	
	return result, nil
}

// createPaymentIntent creates a Stripe payment intent
func (s *StripeProvider) createPaymentIntent(ctx context.Context, request *PaymentRequest) (*StripePaymentIntent, error) {
	// In a real implementation, this would call the Stripe API
	// For now, we'll simulate the response
	
	paymentIntent := &StripePaymentIntent{
		ID:       generatePaymentID(),
		Amount:   request.Amount,
		Currency: request.Currency,
		Status:   "requires_payment_method",
		Metadata: request.Metadata,
	}
	
	// Simulate API call delay
	time.Sleep(100 * time.Millisecond)
	
	return paymentIntent, nil
}

// confirmPaymentIntent confirms a Stripe payment intent
func (s *StripeProvider) confirmPaymentIntent(ctx context.Context, intentID string, paymentMethod *PaymentMethod) error {
	// In a real implementation, this would call the Stripe API to confirm the payment
	// For testing purposes, we'll simulate immediate success
	
	paymentLogger.Debugf("Confirming Stripe payment intent: %s", intentID)
	
	// Simulate API call delay
	time.Sleep(200 * time.Millisecond)
	
	return nil
}

// mapStripeStatus maps Stripe payment intent status to our PaymentStatus
func (s *StripeProvider) mapStripeStatus(stripeStatus string) PaymentStatus {
	switch stripeStatus {
	case "succeeded":
		return PaymentStatusSucceeded
	case "requires_payment_method", "requires_confirmation", "requires_action":
		return PaymentStatusPending
	case "canceled":
		return PaymentStatusCancelled
	default:
		return PaymentStatusFailed
	}
}

// StripePaymentIntent represents a Stripe payment intent
type StripePaymentIntent struct {
	ID       string            `json:"id"`
	Amount   int64             `json:"amount"`
	Currency string            `json:"currency"`
	Status   string            `json:"status"`
	Metadata map[string]string `json:"metadata"`
}

// HandleWebhook handles Stripe webhooks
func (s *StripeProvider) HandleWebhook(ctx context.Context, webhook *PaymentWebhook) error {
	// Verify webhook signature
	if s.webhookSecret != "" {
		if !s.verifyWebhookSignature(webhook) {
			return fmt.Errorf("invalid webhook signature")
		}
	}
	
	// Parse the webhook event
	data, err := s.parseWebhookEvent(webhook)
	if err != nil {
		return fmt.Errorf("failed to parse webhook event: %w", err)
	}
	
	// Process webhook event
	switch webhook.EventType {
	case "payment_intent.succeeded":
		return s.handlePaymentSucceeded(ctx, data)
	case "payment_intent.payment_failed":
		return s.handlePaymentFailed(ctx, data)
	case "payment_intent.canceled":
		return s.handlePaymentCanceled(ctx, data)
	case "charge.dispute.created":
		return s.handleChargeDispute(ctx, data)
	default:
		paymentLogger.Debugf("Unhandled Stripe webhook event: %s", webhook.EventType)
	}
	
	return nil
}

// RefundPayment processes a refund via Stripe
func (s *StripeProvider) RefundPayment(ctx context.Context, paymentID string, amount int64) (*RefundResult, error) {
	// Simplified implementation for demonstration
	refundID := generateRefundID()
	
	result := &RefundResult{
		RefundID:      refundID,
		PaymentID:     paymentID,
		Amount:        amount,
		Currency:      "USD", // Default currency
		Status:        "succeeded",
		ProcessedAt:   time.Now(),
		TransactionID: fmt.Sprintf("stripe_refund_%s", refundID),
	}
	
	paymentLogger.Infof("Stripe refund processed: id=%s, amount=%d", refundID, amount)
	return result, nil
}

// GetProviderName returns the provider name
func (s *StripeProvider) GetProviderName() string {
	return "stripe"
}

// verifyWebhookSignature verifies Stripe webhook signature
func (s *StripeProvider) verifyWebhookSignature(webhook *PaymentWebhook) bool {
	if s.webhookSecret == "" {
		return true // Skip verification if no secret configured
	}
	
	// Create the expected signature
	mac := hmac.New(sha256.New, []byte(s.webhookSecret))
	mac.Write([]byte(fmt.Sprintf("%d", webhook.Timestamp.Unix())))
	mac.Write([]byte("."))
	
	// Convert webhook data to JSON for signature verification
	data, err := json.Marshal(webhook.Data)
	if err != nil {
		paymentLogger.Errorf("Failed to marshal webhook data: %v", err)
		return false
	}
	mac.Write(data)
	
	expectedSignature := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(webhook.Signature), []byte(expectedSignature))
}

// parseWebhookEvent parses a Stripe webhook event
func (s *StripeProvider) parseWebhookEvent(webhook *PaymentWebhook) (map[string]interface{}, error) {
	// Simplified webhook parsing
	return webhook.Data, nil
}

// handlePaymentSucceeded handles successful payment webhook
func (s *StripeProvider) handlePaymentSucceeded(ctx context.Context, data map[string]interface{}) error {
	paymentLogger.Infof("Stripe payment succeeded: %v", data)
	// Here you would update your database, send notifications, etc.
	return nil
}

// handlePaymentFailed handles failed payment webhook
func (s *StripeProvider) handlePaymentFailed(ctx context.Context, data map[string]interface{}) error {
	paymentLogger.Infof("Stripe payment failed: %v", data)
	// Handle payment failure - update status, notify customer, etc.
	return nil
}

// handlePaymentCanceled handles canceled payment webhook
func (s *StripeProvider) handlePaymentCanceled(ctx context.Context, data map[string]interface{}) error {
	paymentLogger.Infof("Stripe payment canceled: %v", data)
	return nil
}

// handleChargeDispute handles charge dispute webhook
func (s *StripeProvider) handleChargeDispute(ctx context.Context, data map[string]interface{}) error {
	paymentLogger.Infof("Stripe charge dispute created: %v", data)
	// Handle dispute - notify admin, gather evidence, etc.
	return nil
}

// FilecoinProvider implements Filecoin payment processing
type FilecoinProvider struct {
	config     *PaymentConfig
	nodeURL    string
	walletAddr string
}

// NewFilecoinProvider creates a new Filecoin payment provider
func NewFilecoinProvider(config *PaymentConfig) (*FilecoinProvider, error) {
	if config.FilecoinNodeURL == "" {
		return nil, fmt.Errorf("filecoin node URL is required")
	}
	if config.FilecoinWalletAddr == "" {
		return nil, fmt.Errorf("filecoin wallet address is required")
	}
	
	return &FilecoinProvider{
		config:     config,
		nodeURL:    config.FilecoinNodeURL,
		walletAddr: config.FilecoinWalletAddr,
	}, nil
}

// ProcessPayment processes a payment via Filecoin
func (f *FilecoinProvider) ProcessPayment(ctx context.Context, request *PaymentRequest) (*PaymentResult, error) {
	// Convert amount from smallest unit (attoFIL) to FIL
	filAmount := big.NewInt(request.Amount)
	
	// Get payment method details
	if request.PaymentMethod == nil || request.PaymentMethod.Details == nil {
		return nil, fmt.Errorf("payment method details required for Filecoin payments")
	}
	
	fromAddr, ok := request.PaymentMethod.Details["from_address"]
	if !ok {
		return nil, fmt.Errorf("from_address required in payment method details")
	}
	
	// Check if payment channel exists
	channelAddr, err := f.getOrCreatePaymentChannel(ctx, fromAddr, f.walletAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to get or create payment channel: %w", err)
	}
	
	// Create payment voucher
	voucher, err := f.createPaymentVoucher(ctx, channelAddr, request.Amount)
	if err != nil {
		return nil, fmt.Errorf("failed to create payment voucher: %w", err)
	}
	
	// Submit payment voucher to the network
	paymentID := generatePaymentID()
	txHash, err := f.submitPaymentVoucher(ctx, voucher)
	if err != nil {
		return nil, fmt.Errorf("failed to submit payment voucher: %w", err)
	}
	
	result := &PaymentResult{
		PaymentID:     paymentID,
		Status:        PaymentStatusPending, // Filecoin payments need confirmation
		Amount:        request.Amount,
		Currency:      "FIL",
		ProcessedAt:   time.Now(),
		TransactionID: txHash,
		Metadata: map[string]string{
			"from_address":    fromAddr,
			"to_address":      f.walletAddr,
			"amount_fil":      filAmount.String(),
			"channel_address": channelAddr,
			"voucher_id":      voucher.ID,
		},
	}
	
	paymentLogger.Infof("Filecoin payment initiated: id=%s, channel=%s, amount=%s FIL", 
		paymentID, channelAddr, filAmount.String())
	
	return result, nil
}

// getOrCreatePaymentChannel gets existing payment channel or creates a new one
func (f *FilecoinProvider) getOrCreatePaymentChannel(ctx context.Context, fromAddr, toAddr string) (string, error) {
	// In a real implementation, this would:
	// 1. Query the Filecoin network for existing payment channels
	// 2. If none exists, create a new payment channel
	// 3. Return the channel address
	
	// For simulation, generate a mock channel address
	channelAddr := fmt.Sprintf("f0%d", time.Now().Unix()%1000000)
	
	paymentLogger.Debugf("Using payment channel: %s (from=%s, to=%s)", channelAddr, fromAddr, toAddr)
	
	return channelAddr, nil
}

// createPaymentVoucher creates a payment voucher for the channel
func (f *FilecoinProvider) createPaymentVoucher(ctx context.Context, channelAddr string, amount int64) (*FilecoinVoucher, error) {
	// In a real implementation, this would:
	// 1. Create a signed payment voucher
	// 2. Include proper nonce and signature
	// 3. Set appropriate lane and amount
	
	voucher := &FilecoinVoucher{
		ID:            fmt.Sprintf("voucher_%d", time.Now().UnixNano()),
		ChannelAddr:   channelAddr,
		Amount:        amount,
		Lane:          0,
		Nonce:         uint64(time.Now().Unix()),
		Signature:     "mock_signature",
		CreatedAt:     time.Now(),
	}
	
	return voucher, nil
}

// submitPaymentVoucher submits the payment voucher to the Filecoin network
func (f *FilecoinProvider) submitPaymentVoucher(ctx context.Context, voucher *FilecoinVoucher) (string, error) {
	// In a real implementation, this would:
	// 1. Submit the voucher to the Filecoin network
	// 2. Wait for transaction confirmation
	// 3. Return the transaction hash
	
	// For simulation, generate a mock transaction hash
	txHash := fmt.Sprintf("bafy2bzaced%x", time.Now().UnixNano())
	
	paymentLogger.Debugf("Submitted Filecoin voucher: %s, tx=%s", voucher.ID, txHash)
	
	return txHash, nil
}

// FilecoinVoucher represents a Filecoin payment voucher
type FilecoinVoucher struct {
	ID          string    `json:"id"`
	ChannelAddr string    `json:"channel_addr"`
	Amount      int64     `json:"amount"`
	Lane        uint64    `json:"lane"`
	Nonce       uint64    `json:"nonce"`
	Signature   string    `json:"signature"`
	CreatedAt   time.Time `json:"created_at"`
}

// HandleWebhook handles Filecoin webhooks
func (f *FilecoinProvider) HandleWebhook(ctx context.Context, webhook *PaymentWebhook) error {
	paymentLogger.Debugf("Filecoin webhook received: %s", webhook.EventType)
	
	switch webhook.EventType {
	case "payment_channel.created":
		return f.handlePaymentChannelCreated(ctx, webhook.Data)
	case "payment_channel.updated":
		return f.handlePaymentChannelUpdated(ctx, webhook.Data)
	case "payment_voucher.redeemed":
		return f.handlePaymentVoucherRedeemed(ctx, webhook.Data)
	case "payment_channel.settled":
		return f.handlePaymentChannelSettled(ctx, webhook.Data)
	default:
		paymentLogger.Debugf("Unhandled Filecoin webhook event: %s", webhook.EventType)
	}
	
	return nil
}

// handlePaymentChannelCreated handles payment channel creation events
func (f *FilecoinProvider) handlePaymentChannelCreated(ctx context.Context, data map[string]interface{}) error {
	paymentLogger.Infof("Filecoin payment channel created: %v", data)
	// Update internal state, notify relevant services
	return nil
}

// handlePaymentChannelUpdated handles payment channel update events
func (f *FilecoinProvider) handlePaymentChannelUpdated(ctx context.Context, data map[string]interface{}) error {
	paymentLogger.Infof("Filecoin payment channel updated: %v", data)
	// Update payment status, process new vouchers
	return nil
}

// handlePaymentVoucherRedeemed handles voucher redemption events
func (f *FilecoinProvider) handlePaymentVoucherRedeemed(ctx context.Context, data map[string]interface{}) error {
	paymentLogger.Infof("Filecoin payment voucher redeemed: %v", data)
	// Mark payment as confirmed, update balances
	return nil
}

// handlePaymentChannelSettled handles channel settlement events
func (f *FilecoinProvider) handlePaymentChannelSettled(ctx context.Context, data map[string]interface{}) error {
	paymentLogger.Infof("Filecoin payment channel settled: %v", data)
	// Finalize all payments in the channel
	return nil
}

// RefundPayment processes a refund via Filecoin
func (f *FilecoinProvider) RefundPayment(ctx context.Context, paymentID string, amount int64) (*RefundResult, error) {
	// Placeholder implementation
	refundID := generateRefundID()
	
	result := &RefundResult{
		RefundID:      refundID,
		PaymentID:     paymentID,
		Amount:        amount,
		Currency:      "FIL",
		Status:        "succeeded",
		ProcessedAt:   time.Now(),
		TransactionID: fmt.Sprintf("fil_refund_%s", refundID),
	}
	
	paymentLogger.Infof("Filecoin refund processed: %s", refundID)
	return result, nil
}

// GetProviderName returns the provider name
func (f *FilecoinProvider) GetProviderName() string {
	return "filecoin"
}

// PolygonProvider implements Polygon L2 payment processing
type PolygonProvider struct {
	config       *PaymentConfig
	rpcURL       string
	contractAddr string
}

// NewPolygonProvider creates a new Polygon payment provider
func NewPolygonProvider(config *PaymentConfig) (*PolygonProvider, error) {
	if config.PolygonRPCURL == "" {
		return nil, fmt.Errorf("polygon RPC URL is required")
	}
	if config.PaymentContractAddr == "" {
		return nil, fmt.Errorf("payment contract address is required")
	}
	
	return &PolygonProvider{
		config:       config,
		rpcURL:       config.PolygonRPCURL,
		contractAddr: config.PaymentContractAddr,
	}, nil
}

// ProcessPayment processes a payment via Polygon
func (p *PolygonProvider) ProcessPayment(ctx context.Context, request *PaymentRequest) (*PaymentResult, error) {
	// Get transaction hash from payment method details
	if request.PaymentMethod == nil || request.PaymentMethod.Details == nil {
		return nil, fmt.Errorf("payment method details required for Polygon payments")
	}
	
	txHash, ok := request.PaymentMethod.Details["transaction_hash"]
	if !ok {
		return nil, fmt.Errorf("transaction_hash required in payment method details")
	}
	
	fromAddr, ok := request.PaymentMethod.Details["from_address"]
	if !ok {
		return nil, fmt.Errorf("from_address required in payment method details")
	}
	
	// Verify transaction on Polygon network
	tx, err := p.getTransaction(ctx, txHash)
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction: %w", err)
	}
	
	// Validate transaction details
	if err := p.validateTransaction(tx, request, fromAddr); err != nil {
		return nil, fmt.Errorf("transaction validation failed: %w", err)
	}
	
	// Wait for confirmations
	confirmed, err := p.waitForConfirmations(ctx, txHash, p.config.ConfirmationBlocks)
	if err != nil {
		return nil, fmt.Errorf("failed to wait for confirmations: %w", err)
	}
	
	status := PaymentStatusPending
	if confirmed {
		status = PaymentStatusSucceeded
	}
	
	paymentID := generatePaymentID()
	
	result := &PaymentResult{
		PaymentID:     paymentID,
		Status:        status,
		Amount:        request.Amount,
		Currency:      "MATIC",
		ProcessedAt:   time.Now(),
		TransactionID: txHash,
		Metadata: map[string]string{
			"network":          "polygon",
			"contract_address": p.contractAddr,
			"from_address":     fromAddr,
			"block_number":     fmt.Sprintf("%d", tx.BlockNumber),
			"gas_used":         fmt.Sprintf("%d", tx.GasUsed),
		},
	}
	
	paymentLogger.Infof("Polygon payment processed: id=%s, tx=%s, amount=%d MATIC, confirmed=%t", 
		paymentID, txHash, request.Amount, confirmed)
	
	return result, nil
}

// getTransaction retrieves transaction details from Polygon network
func (p *PolygonProvider) getTransaction(ctx context.Context, txHash string) (*EthereumTransaction, error) {
	// In a real implementation, this would call the Polygon RPC endpoint
	// For simulation, create a mock transaction for successful cases
	
	if txHash == "" {
		return nil, fmt.Errorf("transaction hash is required")
	}
	
	// Simulate network call delay
	time.Sleep(100 * time.Millisecond)
	
	// For testing, return a mock successful transaction
	tx := &EthereumTransaction{
		Hash:        txHash,
		From:        "0x742d35Cc6634C0532925a3b8D4C9db96590c6C87", // Mock sender
		To:          p.contractAddr,
		Value:       big.NewInt(1000000000000000000), // 1 ETH in wei
		GasUsed:     21000,
		BlockNumber: uint64(time.Now().Unix()),
		Status:      1, // Success
	}
	
	return tx, nil
}

// validateTransaction validates that the transaction matches the payment request
func (p *PolygonProvider) validateTransaction(tx *EthereumTransaction, request *PaymentRequest, fromAddr string) error {
	// Validate sender address
	if tx.From != fromAddr {
		return fmt.Errorf("transaction from address mismatch: expected %s, got %s", fromAddr, tx.From)
	}
	
	// Validate recipient address (contract)
	if tx.To != p.contractAddr {
		return fmt.Errorf("transaction to address mismatch: expected %s, got %s", p.contractAddr, tx.To)
	}
	
	// Validate amount (convert wei to smallest unit)
	expectedAmount := big.NewInt(request.Amount)
	if tx.Value.Cmp(expectedAmount) != 0 {
		return fmt.Errorf("transaction amount mismatch: expected %s, got %s", expectedAmount.String(), tx.Value.String())
	}
	
	// Validate transaction status
	if tx.Status != 1 {
		return fmt.Errorf("transaction failed with status: %d", tx.Status)
	}
	
	return nil
}

// waitForConfirmations waits for the specified number of confirmations
func (p *PolygonProvider) waitForConfirmations(ctx context.Context, txHash string, confirmations int) (bool, error) {
	// In a real implementation, this would:
	// 1. Get current block number
	// 2. Compare with transaction block number
	// 3. Wait until enough confirmations are reached
	
	// For simulation, assume confirmations are reached after a delay
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case <-time.After(2 * time.Second):
		paymentLogger.Debugf("Polygon transaction confirmed: %s", txHash)
		return true, nil
	}
}

// EthereumTransaction represents an Ethereum/Polygon transaction
type EthereumTransaction struct {
	Hash        string   `json:"hash"`
	From        string   `json:"from"`
	To          string   `json:"to"`
	Value       *big.Int `json:"value"`
	GasUsed     uint64   `json:"gas_used"`
	BlockNumber uint64   `json:"block_number"`
	Status      int      `json:"status"`
}

// HandleWebhook handles Polygon webhooks
func (p *PolygonProvider) HandleWebhook(ctx context.Context, webhook *PaymentWebhook) error {
	paymentLogger.Debugf("Polygon webhook received: %s", webhook.EventType)
	
	switch webhook.EventType {
	case "transaction.confirmed":
		return p.handleTransactionConfirmed(ctx, webhook.Data)
	case "transaction.failed":
		return p.handleTransactionFailed(ctx, webhook.Data)
	case "block.finalized":
		return p.handleBlockFinalized(ctx, webhook.Data)
	default:
		paymentLogger.Debugf("Unhandled Polygon webhook event: %s", webhook.EventType)
	}
	
	return nil
}

// handleTransactionConfirmed handles transaction confirmation events
func (p *PolygonProvider) handleTransactionConfirmed(ctx context.Context, data map[string]interface{}) error {
	paymentLogger.Infof("Polygon transaction confirmed: %v", data)
	// Update payment status to confirmed
	return nil
}

// handleTransactionFailed handles transaction failure events
func (p *PolygonProvider) handleTransactionFailed(ctx context.Context, data map[string]interface{}) error {
	paymentLogger.Infof("Polygon transaction failed: %v", data)
	// Update payment status to failed, trigger refund if needed
	return nil
}

// handleBlockFinalized handles block finalization events
func (p *PolygonProvider) handleBlockFinalized(ctx context.Context, data map[string]interface{}) error {
	paymentLogger.Debugf("Polygon block finalized: %v", data)
	// Update finality status for payments in the block
	return nil
}

// RefundPayment processes a refund via Polygon
func (p *PolygonProvider) RefundPayment(ctx context.Context, paymentID string, amount int64) (*RefundResult, error) {
	refundID := generateRefundID()
	
	result := &RefundResult{
		RefundID:      refundID,
		PaymentID:     paymentID,
		Amount:        amount,
		Currency:      "MATIC",
		Status:        "succeeded",
		ProcessedAt:   time.Now(),
		TransactionID: fmt.Sprintf("polygon_refund_%s", refundID),
	}
	
	paymentLogger.Infof("Polygon refund processed: %s", refundID)
	return result, nil
}

// GetProviderName returns the provider name
func (p *PolygonProvider) GetProviderName() string {
	return "polygon"
}

// ArbitrumProvider implements Arbitrum L2 payment processing
type ArbitrumProvider struct {
	config       *PaymentConfig
	rpcURL       string
	contractAddr string
}

// NewArbitrumProvider creates a new Arbitrum payment provider
func NewArbitrumProvider(config *PaymentConfig) (*ArbitrumProvider, error) {
	if config.ArbitrumRPCURL == "" {
		return nil, fmt.Errorf("arbitrum RPC URL is required")
	}
	if config.PaymentContractAddr == "" {
		return nil, fmt.Errorf("payment contract address is required")
	}
	
	return &ArbitrumProvider{
		config:       config,
		rpcURL:       config.ArbitrumRPCURL,
		contractAddr: config.PaymentContractAddr,
	}, nil
}

// ProcessPayment processes a payment via Arbitrum
func (a *ArbitrumProvider) ProcessPayment(ctx context.Context, request *PaymentRequest) (*PaymentResult, error) {
	// Get transaction hash from payment method details
	if request.PaymentMethod == nil || request.PaymentMethod.Details == nil {
		return nil, fmt.Errorf("payment method details required for Arbitrum payments")
	}
	
	txHash, ok := request.PaymentMethod.Details["transaction_hash"]
	if !ok {
		return nil, fmt.Errorf("transaction_hash required in payment method details")
	}
	
	fromAddr, ok := request.PaymentMethod.Details["from_address"]
	if !ok {
		return nil, fmt.Errorf("from_address required in payment method details")
	}
	
	// Verify transaction on Arbitrum network
	tx, err := a.getTransaction(ctx, txHash)
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction: %w", err)
	}
	
	// Validate transaction details
	if err := a.validateTransaction(tx, request, fromAddr); err != nil {
		return nil, fmt.Errorf("transaction validation failed: %w", err)
	}
	
	// Wait for confirmations (Arbitrum has faster finality than Ethereum mainnet)
	confirmed, err := a.waitForConfirmations(ctx, txHash, a.config.ConfirmationBlocks)
	if err != nil {
		return nil, fmt.Errorf("failed to wait for confirmations: %w", err)
	}
	
	status := PaymentStatusPending
	if confirmed {
		status = PaymentStatusSucceeded
	}
	
	paymentID := generatePaymentID()
	
	result := &PaymentResult{
		PaymentID:     paymentID,
		Status:        status,
		Amount:        request.Amount,
		Currency:      "ETH",
		ProcessedAt:   time.Now(),
		TransactionID: txHash,
		Metadata: map[string]string{
			"network":          "arbitrum",
			"contract_address": a.contractAddr,
			"from_address":     fromAddr,
			"block_number":     fmt.Sprintf("%d", tx.BlockNumber),
			"gas_used":         fmt.Sprintf("%d", tx.GasUsed),
			"l1_block_number":  fmt.Sprintf("%d", tx.L1BlockNumber),
		},
	}
	
	paymentLogger.Infof("Arbitrum payment processed: id=%s, tx=%s, amount=%d ETH, confirmed=%t", 
		paymentID, txHash, request.Amount, confirmed)
	
	return result, nil
}

// getTransaction retrieves transaction details from Arbitrum network
func (a *ArbitrumProvider) getTransaction(ctx context.Context, txHash string) (*ArbitrumTransaction, error) {
	// In a real implementation, this would call the Arbitrum RPC endpoint
	// For simulation, create a mock transaction for successful cases
	
	if txHash == "" {
		return nil, fmt.Errorf("transaction hash is required")
	}
	
	// Simulate network call delay
	time.Sleep(100 * time.Millisecond)
	
	// For testing, return a mock successful transaction
	tx := &ArbitrumTransaction{
		EthereumTransaction: EthereumTransaction{
			Hash:        txHash,
			From:        "0x742d35Cc6634C0532925a3b8D4C9db96590c6C87", // Mock sender
			To:          a.contractAddr,
			Value:       big.NewInt(1000000000000000000), // 1 ETH in wei
			GasUsed:     21000,
			BlockNumber: uint64(time.Now().Unix()),
			Status:      1, // Success
		},
		L1BlockNumber: uint64(time.Now().Unix() - 100), // Mock L1 block
		L2TxHash:      txHash,
	}
	
	return tx, nil
}

// validateTransaction validates that the transaction matches the payment request
func (a *ArbitrumProvider) validateTransaction(tx *ArbitrumTransaction, request *PaymentRequest, fromAddr string) error {
	// Validate sender address
	if tx.From != fromAddr {
		return fmt.Errorf("transaction from address mismatch: expected %s, got %s", fromAddr, tx.From)
	}
	
	// Validate recipient address (contract)
	if tx.To != a.contractAddr {
		return fmt.Errorf("transaction to address mismatch: expected %s, got %s", a.contractAddr, tx.To)
	}
	
	// Validate amount (convert wei to smallest unit)
	expectedAmount := big.NewInt(request.Amount)
	if tx.Value.Cmp(expectedAmount) != 0 {
		return fmt.Errorf("transaction amount mismatch: expected %s, got %s", expectedAmount.String(), tx.Value.String())
	}
	
	// Validate transaction status
	if tx.Status != 1 {
		return fmt.Errorf("transaction failed with status: %d", tx.Status)
	}
	
	return nil
}

// waitForConfirmations waits for the specified number of confirmations on Arbitrum
func (a *ArbitrumProvider) waitForConfirmations(ctx context.Context, txHash string, confirmations int) (bool, error) {
	// In a real implementation, this would:
	// 1. Get current L2 block number
	// 2. Compare with transaction block number
	// 3. Wait until enough confirmations are reached
	// 4. Optionally check L1 finality for extra security
	
	// For simulation, assume confirmations are reached quickly (Arbitrum is fast)
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case <-time.After(1 * time.Second):
		paymentLogger.Debugf("Arbitrum transaction confirmed: %s", txHash)
		return true, nil
	}
}

// ArbitrumTransaction represents an Arbitrum transaction with L1/L2 details
type ArbitrumTransaction struct {
	EthereumTransaction
	L1BlockNumber uint64 `json:"l1_block_number"`
	L2TxHash      string `json:"l2_tx_hash"`
}

// HandleWebhook handles Arbitrum webhooks
func (a *ArbitrumProvider) HandleWebhook(ctx context.Context, webhook *PaymentWebhook) error {
	paymentLogger.Debugf("Arbitrum webhook received: %s", webhook.EventType)
	
	switch webhook.EventType {
	case "l2_transaction.confirmed":
		return a.handleL2TransactionConfirmed(ctx, webhook.Data)
	case "l1_batch.finalized":
		return a.handleL1BatchFinalized(ctx, webhook.Data)
	case "transaction.failed":
		return a.handleTransactionFailed(ctx, webhook.Data)
	case "dispute.raised":
		return a.handleDisputeRaised(ctx, webhook.Data)
	default:
		paymentLogger.Debugf("Unhandled Arbitrum webhook event: %s", webhook.EventType)
	}
	
	return nil
}

// handleL2TransactionConfirmed handles L2 transaction confirmation events
func (a *ArbitrumProvider) handleL2TransactionConfirmed(ctx context.Context, data map[string]interface{}) error {
	paymentLogger.Infof("Arbitrum L2 transaction confirmed: %v", data)
	// Update payment status to L2 confirmed
	return nil
}

// handleL1BatchFinalized handles L1 batch finalization events
func (a *ArbitrumProvider) handleL1BatchFinalized(ctx context.Context, data map[string]interface{}) error {
	paymentLogger.Infof("Arbitrum L1 batch finalized: %v", data)
	// Update payment status to fully finalized
	return nil
}

// handleTransactionFailed handles transaction failure events
func (a *ArbitrumProvider) handleTransactionFailed(ctx context.Context, data map[string]interface{}) error {
	paymentLogger.Infof("Arbitrum transaction failed: %v", data)
	// Update payment status to failed, trigger refund if needed
	return nil
}

// handleDisputeRaised handles dispute events (Arbitrum-specific)
func (a *ArbitrumProvider) handleDisputeRaised(ctx context.Context, data map[string]interface{}) error {
	paymentLogger.Warnf("Arbitrum dispute raised: %v", data)
	// Handle dispute resolution, may affect payment finality
	return nil
}

// RefundPayment processes a refund via Arbitrum
func (a *ArbitrumProvider) RefundPayment(ctx context.Context, paymentID string, amount int64) (*RefundResult, error) {
	refundID := generateRefundID()
	
	result := &RefundResult{
		RefundID:      refundID,
		PaymentID:     paymentID,
		Amount:        amount,
		Currency:      "ETH",
		Status:        "succeeded",
		ProcessedAt:   time.Now(),
		TransactionID: fmt.Sprintf("arbitrum_refund_%s", refundID),
	}
	
	paymentLogger.Infof("Arbitrum refund processed: %s", refundID)
	return result, nil
}

// GetProviderName returns the provider name
func (a *ArbitrumProvider) GetProviderName() string {
	return "arbitrum"
}

// generatePaymentID generates a unique payment ID
func generatePaymentID() string {
	return fmt.Sprintf("pay_%d", time.Now().UnixNano())
}

// generateRefundID generates a unique refund ID
func generateRefundID() string {
	return fmt.Sprintf("ref_%d", time.Now().UnixNano())
}

// MemoryPaymentProcessor is an in-memory implementation for testing
type MemoryPaymentProcessor struct {
	payments map[string]*PaymentResult
	refunds  map[string]*RefundResult
	config   *Config
}

// NewMemoryPaymentProcessor creates a new in-memory payment processor
func NewMemoryPaymentProcessor(config *Config) *MemoryPaymentProcessor {
	return &MemoryPaymentProcessor{
		payments: make(map[string]*PaymentResult),
		refunds:  make(map[string]*RefundResult),
		config:   config,
	}
}

// ProcessPayment implements PaymentProcessor interface
func (m *MemoryPaymentProcessor) ProcessPayment(ctx context.Context, payment *PaymentRequest) (*PaymentResult, error) {
	paymentID := generatePaymentID()
	
	result := &PaymentResult{
		PaymentID:     paymentID,
		Status:        PaymentStatusSucceeded,
		Amount:        payment.Amount,
		Currency:      payment.Currency,
		ProcessedAt:   time.Now(),
		TransactionID: fmt.Sprintf("mock_%s", paymentID),
		Metadata:      payment.Metadata,
	}
	
	m.payments[paymentID] = result
	return result, nil
}

// HandlePaymentWebhook implements PaymentProcessor interface
func (m *MemoryPaymentProcessor) HandlePaymentWebhook(ctx context.Context, webhook *PaymentWebhook) error {
	paymentLogger.Debugf("Mock webhook received: %s", webhook.EventType)
	return nil
}

// RefundPayment implements PaymentProcessor interface
func (m *MemoryPaymentProcessor) RefundPayment(ctx context.Context, paymentID string, amount int64) (*RefundResult, error) {
	payment, exists := m.payments[paymentID]
	if !exists {
		return nil, fmt.Errorf("payment not found: %s", paymentID)
	}
	
	refundID := generateRefundID()
	
	result := &RefundResult{
		RefundID:      refundID,
		PaymentID:     paymentID,
		Amount:        amount,
		Currency:      payment.Currency,
		Status:        "succeeded",
		ProcessedAt:   time.Now(),
		TransactionID: fmt.Sprintf("mock_refund_%s", refundID),
	}
	
	m.refunds[refundID] = result
	return result, nil
}

// GetPayments returns all stored payments (for testing)
func (m *MemoryPaymentProcessor) GetPayments() map[string]*PaymentResult {
	return m.payments
}

// GetRefunds returns all stored refunds (for testing)
func (m *MemoryPaymentProcessor) GetRefunds() map[string]*RefundResult {
	return m.refunds
}