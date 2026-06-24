package checkout

import (
	"context"
	"time"

	webirr "github.com/webirr/webirr-api-go-client"
)

// PayableStatus is the safe customer-facing checkout status.
type PayableStatus string

const (
	StatusPending PayableStatus = "Pending"
	StatusPaid    PayableStatus = "Paid"
	StatusFailed  PayableStatus = "Failed"
	StatusUnknown PayableStatus = "Unknown"
)

// Payable is the merchant-side invoice, order, enrollment, or other item being paid.
type Payable struct {
	MerchantReference string
	Amount            string
	Currency          string
	CustomerName      string
	CustomerCode      string
	CustomerPhone     string
	Description       string
	BillTime          string
	SuccessURL        string
	CancelURL         string
	WebirrPaymentCode string
	PaymentStatus     *int
}

// PaymentResult is the normalized paid payment data passed to Store.MarkPaid.
type PaymentResult struct {
	PaymentCode      string
	PaymentStatus    int
	PaymentReference string
	PaymentIssuer    string
	PaidAt           string
	Raw              *webirr.PaymentStatus
}

// SupportedBank is safe to return to browser/mobile checkout UI.
type SupportedBank struct {
	BankID string `json:"bankID"`
	Name   string `json:"name"`
}

// CheckoutInstructions is rendered by browser checkout UI.
type CheckoutInstructions struct {
	Title string   `json:"title,omitempty"`
	Steps []string `json:"steps,omitempty"`
}

// CheckoutViewModel is returned by the create/resume endpoint.
type CheckoutViewModel struct {
	MerchantReference string               `json:"merchantReference"`
	PaymentCode       string               `json:"paymentCode"`
	Amount            string               `json:"amount"`
	Currency          string               `json:"currency"`
	Description       string               `json:"description"`
	CustomerName      string               `json:"customerName,omitempty"`
	CustomerCode      string               `json:"customerCode,omitempty"`
	CustomerPhone     string               `json:"customerPhone,omitempty"`
	Status            PayableStatus        `json:"status"`
	PaymentStatus     *int                 `json:"paymentStatus,omitempty"`
	PollIntervalMS    int                  `json:"pollIntervalMs"`
	SuccessURL        string               `json:"successUrl,omitempty"`
	CancelURL         string               `json:"cancelUrl,omitempty"`
	Instructions      CheckoutInstructions `json:"instructions"`
	SupportedBanks    []SupportedBank      `json:"supportedBanks"`
}

// CheckoutStatusResult is returned by the status endpoint.
type CheckoutStatusResult struct {
	MerchantReference string        `json:"merchantReference"`
	PaymentCode       string        `json:"paymentCode,omitempty"`
	Amount            string        `json:"amount,omitempty"`
	Currency          string        `json:"currency,omitempty"`
	Description       string        `json:"description,omitempty"`
	CustomerName      string        `json:"customerName,omitempty"`
	CustomerCode      string        `json:"customerCode,omitempty"`
	CustomerPhone     string        `json:"customerPhone,omitempty"`
	Status            PayableStatus `json:"status"`
	PaymentStatus     *int          `json:"paymentStatus,omitempty"`
	PaymentReference  string        `json:"paymentReference,omitempty"`
	PaymentIssuer     string        `json:"paymentIssuer,omitempty"`
	PaidAt            string        `json:"paidAt,omitempty"`
	ReceiptURL        string        `json:"receiptUrl,omitempty"`
}

// GatewayClient is the subset of the official Go SDK used by checkout.
type GatewayClient interface {
	CreateBill(context.Context, *webirr.Bill) (*webirr.ApiResponse[string], error)
	UpdateBill(context.Context, *webirr.Bill) (*webirr.ApiResponse[string], error)
	GetPaymentStatus(context.Context, string) (*webirr.ApiResponse[webirr.PaymentStatus], error)
	GetBillByReference(context.Context, string) (*webirr.ApiResponse[webirr.BillResponse], error)
	GetBillByPaymentCode(context.Context, string) (*webirr.ApiResponse[webirr.BillResponse], error)
	GetSupportedBanks(context.Context) (*webirr.ApiResponse[[]webirr.SupportedBank], error)
}

// Store is implemented by the merchant application.
type Store interface {
	LoadPayable(context.Context, string) (Payable, error)
	SavePaymentCode(context.Context, string, string) error
	MarkPaid(context.Context, string, PaymentResult) error
}

// LocalStatusReader is implemented by stores that can answer checkout status
// from webhook-updated or bulk-polling-updated local state.
type LocalStatusReader interface {
	LoadCheckoutStatus(context.Context, string) (CheckoutStatusResult, error)
}

// StatusResolver allows advanced merchants to choose where checkout status comes from.
type StatusResolver interface {
	ResolveStatus(context.Context, *Checkout, Payable) (CheckoutStatusResult, error)
}

// StatusResolverFunc adapts a function to StatusResolver.
type StatusResolverFunc func(context.Context, *Checkout, Payable) (CheckoutStatusResult, error)

func (f StatusResolverFunc) ResolveStatus(ctx context.Context, checkout *Checkout, payable Payable) (CheckoutStatusResult, error) {
	return f(ctx, checkout, payable)
}

// Option configures checkout behavior.
type Option func(*Checkout)

// WithPollInterval changes the browser polling interval in returned view models.
func WithPollInterval(interval time.Duration) Option {
	return func(c *Checkout) {
		if interval > 0 {
			c.pollInterval = interval
		}
	}
}

// WithInstructions sets a base instruction title. Steps are generated from supported banks.
func WithInstructions(instructions CheckoutInstructions) Option {
	return func(c *Checkout) {
		c.instructions = instructions
	}
}

// WithStatusResolver uses a custom status source.
func WithStatusResolver(resolver StatusResolver) Option {
	return func(c *Checkout) {
		if resolver != nil {
			c.statusResolver = resolver
		}
	}
}

// WithLocalStatus reads checkout status only from the merchant local store.
func WithLocalStatus() Option {
	return WithStatusResolver(LocalStatusResolver{})
}

// WithHybridStatus reads local status first and falls back to WeBirr polling while pending or unknown.
func WithHybridStatus() Option {
	return WithStatusResolver(HybridStatusResolver{})
}
