package checkout

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	webirr "github.com/webirr/webirr-api-go-client"
)

const defaultPollInterval = 3 * time.Second

var (
	// ErrMerchantReferenceRequired is returned when a browser request omits merchantReference.
	ErrMerchantReferenceRequired = errors.New("merchantReference is required")

	// ErrUnsupportedLocalStatus is returned when WithLocalStatus or WithHybridStatus
	// is used with a store that cannot read local checkout status.
	ErrUnsupportedLocalStatus = errors.New("checkout store does not implement LocalStatusReader")
)

// Checkout coordinates merchant payable resolution, WeBirr bill creation, and status checks.
type Checkout struct {
	gateway        GatewayClient
	store          Store
	pollInterval   time.Duration
	instructions   CheckoutInstructions
	statusResolver StatusResolver

	paidMu sync.Mutex
	paid   map[string]struct{}
}

// New creates a checkout service. If no status resolver is configured, WeBirr
// payment-status polling is used.
func New(gateway GatewayClient, store Store, options ...Option) *Checkout {
	c := &Checkout{
		gateway:      gateway,
		store:        store,
		pollInterval: defaultPollInterval,
		instructions: CheckoutInstructions{
			Title: "Payment Instruction",
		},
		paid: make(map[string]struct{}),
	}
	for _, option := range options {
		if option != nil {
			option(c)
		}
	}
	if c.statusResolver == nil {
		c.statusResolver = GatewayPollingStatusResolver{}
	}
	return c
}

// CreateCheckout creates or resumes a WeBirr payment code for a merchant payable.
func (c *Checkout) CreateCheckout(ctx context.Context, merchantReference string) (CheckoutViewModel, error) {
	merchantReference, err := requireMerchantReference(merchantReference)
	if err != nil {
		return CheckoutViewModel{}, err
	}

	payable, err := c.loadPayable(ctx, merchantReference)
	if err != nil {
		return CheckoutViewModel{}, err
	}

	bill := toBill(payable)
	supportedBanks := c.supportedBanks(ctx)
	instructions := instructionsForSupportedBanks(c.instructions, supportedBanks)
	paymentCode := clean(payable.WebirrPaymentCode)
	paymentStatus := payable.PaymentStatus

	if paymentCode == "" {
		recovered, err := c.recoverBill(ctx, merchantReference)
		if err != nil {
			return CheckoutViewModel{}, err
		}
		if recovered != nil {
			paymentCode = clean(recovered.WbcCode)
			status := recovered.PaymentStatus
			paymentStatus = &status
			if paymentCode != "" {
				if err := c.store.SavePaymentCode(ctx, merchantReference, paymentCode); err != nil {
					return CheckoutViewModel{}, err
				}
			}
		}
	}

	if paymentCode == "" {
		created, err := c.gateway.CreateBill(ctx, bill)
		if err != nil {
			return CheckoutViewModel{}, err
		}
		if err := gatewayError(created, "create bill"); err != nil {
			return CheckoutViewModel{}, err
		}
		paymentCode = clean(created.Res)
		if paymentCode == "" {
			return CheckoutViewModel{}, errors.New("WeBirr did not return a payment code")
		}
		if err := c.store.SavePaymentCode(ctx, merchantReference, paymentCode); err != nil {
			return CheckoutViewModel{}, err
		}
	} else {
		status, err := c.gateway.GetPaymentStatus(ctx, paymentCode)
		if err != nil {
			return CheckoutViewModel{}, err
		}
		if status != nil && status.Error == "" {
			paymentStatus = &status.Res.Status
			if status.Res.IsPaid() {
				if _, err := c.markPaidOnce(ctx, merchantReference, paymentCode, status.Res); err != nil {
					return CheckoutViewModel{}, err
				}
				return viewModelFromStatus(payable, paymentCode, paymentStatus, StatusPaid, c.pollInterval, instructions, supportedBanks), nil
			}
		}

		existing, err := c.gateway.GetBillByPaymentCode(ctx, paymentCode)
		if err != nil {
			return CheckoutViewModel{}, err
		}
		if existing != nil && existing.Error == "" {
			status := existing.Res.PaymentStatus
			paymentStatus = &status
			if status != 2 && billChanged(existing.Res, bill) {
				updated, err := c.gateway.UpdateBill(ctx, bill)
				if err != nil {
					return CheckoutViewModel{}, err
				}
				if err := gatewayError(updated, "update bill"); err != nil {
					return CheckoutViewModel{}, err
				}
			}
		}
	}

	status := StatusPending
	if paymentStatus != nil && *paymentStatus == 2 {
		status = StatusPaid
	}
	return viewModelFromStatus(payable, paymentCode, paymentStatus, status, c.pollInterval, instructions, supportedBanks), nil
}

// GetStatus returns a safe status response for the checkout UI.
func (c *Checkout) GetStatus(ctx context.Context, merchantReference string) (CheckoutStatusResult, error) {
	merchantReference, err := requireMerchantReference(merchantReference)
	if err != nil {
		return CheckoutStatusResult{}, err
	}
	payable, err := c.loadPayable(ctx, merchantReference)
	if err != nil {
		return CheckoutStatusResult{}, err
	}
	return c.statusResolver.ResolveStatus(ctx, c, payable)
}

// ResolveGatewayStatus checks WeBirr payment status through the official SDK.
func (c *Checkout) ResolveGatewayStatus(ctx context.Context, payable Payable) (CheckoutStatusResult, error) {
	paymentCode := clean(payable.WebirrPaymentCode)
	if paymentCode == "" {
		result := c.statusFromPayable(payable, StatusUnknown)
		return result, nil
	}

	status, err := c.gateway.GetPaymentStatus(ctx, paymentCode)
	if err != nil {
		return CheckoutStatusResult{}, err
	}
	if err := gatewayError(status, "get payment status"); err != nil {
		return CheckoutStatusResult{}, err
	}

	paymentStatus := status.Res.Status
	if status.Res.IsPaid() {
		payment, err := c.markPaidOnce(ctx, payable.MerchantReference, paymentCode, status.Res)
		if err != nil {
			return CheckoutStatusResult{}, err
		}
		result := c.statusFromPayable(payable, StatusPaid)
		result.PaymentCode = paymentCode
		result.PaymentStatus = &paymentStatus
		result.PaymentReference = payment.PaymentReference
		result.PaymentIssuer = payment.PaymentIssuer
		result.PaidAt = payment.PaidAt
		result.ReceiptURL = payable.SuccessURL
		return result, nil
	}

	result := c.statusFromPayable(payable, StatusPending)
	result.PaymentCode = paymentCode
	result.PaymentStatus = &paymentStatus
	return result, nil
}

// ResolveLocalStatus reads checkout status from the merchant store.
func (c *Checkout) ResolveLocalStatus(ctx context.Context, payable Payable) (CheckoutStatusResult, error) {
	reader, ok := c.store.(LocalStatusReader)
	if !ok {
		return CheckoutStatusResult{}, ErrUnsupportedLocalStatus
	}
	status, err := reader.LoadCheckoutStatus(ctx, payable.MerchantReference)
	if err != nil {
		return CheckoutStatusResult{}, err
	}
	status = mergeStatusDisplay(status, payable)
	if status.Status == StatusPaid {
		payment := PaymentResult{
			PaymentCode:      status.PaymentCode,
			PaymentStatus:    statusInt(status.PaymentStatus, 2),
			PaymentReference: status.PaymentReference,
			PaymentIssuer:    status.PaymentIssuer,
			PaidAt:           status.PaidAt,
		}
		if err := c.markPaidResultOnce(ctx, payable.MerchantReference, payment); err != nil {
			return CheckoutStatusResult{}, err
		}
	}
	return status, nil
}

func (c *Checkout) loadPayable(ctx context.Context, merchantReference string) (Payable, error) {
	payable, err := c.store.LoadPayable(ctx, merchantReference)
	if err != nil {
		return Payable{}, err
	}
	if clean(payable.MerchantReference) == "" {
		payable.MerchantReference = merchantReference
	}
	if clean(payable.MerchantReference) != merchantReference {
		return Payable{}, fmt.Errorf("store returned payable for %q while %q was requested", payable.MerchantReference, merchantReference)
	}
	if payable.Currency == "" {
		payable.Currency = "ETB"
	}
	return payable, nil
}

func (c *Checkout) recoverBill(ctx context.Context, merchantReference string) (*webirr.BillResponse, error) {
	response, err := c.gateway.GetBillByReference(ctx, merchantReference)
	if err != nil {
		return nil, err
	}
	if response == nil || response.Error != "" || clean(response.Res.WbcCode) == "" {
		return nil, nil
	}
	return &response.Res, nil
}

func (c *Checkout) supportedBanks(ctx context.Context) []SupportedBank {
	response, err := c.gateway.GetSupportedBanks(ctx)
	if err != nil || response == nil || response.Error != "" {
		return nil
	}
	banks := make([]SupportedBank, 0, len(response.Res))
	for _, bank := range response.Res {
		bankID := clean(bank.BankID)
		name := clean(bank.Name)
		if bankID == "" || name == "" {
			continue
		}
		banks = append(banks, SupportedBank{BankID: bankID, Name: name})
	}
	return banks
}

func (c *Checkout) markPaidOnce(ctx context.Context, merchantReference, paymentCode string, status webirr.PaymentStatus) (PaymentResult, error) {
	payment := PaymentResult{
		PaymentCode:      paymentCode,
		PaymentStatus:    2,
		PaymentReference: "",
		PaymentIssuer:    "",
		Raw:              &status,
	}
	if status.Data != nil {
		payment.PaymentReference = status.Data.PaymentReference
		payment.PaymentIssuer = formatIssuer(status.Data.BankID)
		payment.PaidAt = firstNonEmpty(status.Data.PaymentDate, status.Data.Time)
	}
	return payment, c.markPaidResultOnce(ctx, merchantReference, payment)
}

func (c *Checkout) markPaidResultOnce(ctx context.Context, merchantReference string, payment PaymentResult) error {
	key := merchantReference + "\x00" + payment.PaymentCode
	c.paidMu.Lock()
	if _, ok := c.paid[key]; ok {
		c.paidMu.Unlock()
		return nil
	}
	c.paid[key] = struct{}{}
	c.paidMu.Unlock()

	if err := c.store.MarkPaid(ctx, merchantReference, payment); err != nil {
		c.paidMu.Lock()
		delete(c.paid, key)
		c.paidMu.Unlock()
		return err
	}
	return nil
}

func (c *Checkout) statusFromPayable(payable Payable, status PayableStatus) CheckoutStatusResult {
	return CheckoutStatusResult{
		MerchantReference: payable.MerchantReference,
		PaymentCode:       clean(payable.WebirrPaymentCode),
		Amount:            payable.Amount,
		Currency:          payable.Currency,
		Description:       payable.Description,
		CustomerName:      payable.CustomerName,
		CustomerCode:      payable.CustomerCode,
		CustomerPhone:     payable.CustomerPhone,
		Status:            status,
		PaymentStatus:     payable.PaymentStatus,
	}
}

func requireMerchantReference(value string) (string, error) {
	value = clean(value)
	if value == "" {
		return "", ErrMerchantReferenceRequired
	}
	return value, nil
}

func toBill(payable Payable) *webirr.Bill {
	return &webirr.Bill{
		Amount:        payable.Amount,
		CustomerCode:  firstNonEmpty(payable.CustomerCode, payable.CustomerPhone, payable.MerchantReference),
		CustomerName:  firstNonEmpty(payable.CustomerName, payable.CustomerCode, payable.MerchantReference),
		CustomerPhone: payable.CustomerPhone,
		Time:          firstNonEmpty(payable.BillTime, time.Now().Format("2006-01-02 15:04")),
		Description:   firstNonEmpty(payable.Description, "Payment for "+payable.MerchantReference),
		BillReference: payable.MerchantReference,
		Extras:        map[string]string{},
	}
}

func viewModelFromStatus(payable Payable, paymentCode string, paymentStatus *int, status PayableStatus, pollInterval time.Duration, instructions CheckoutInstructions, supportedBanks []SupportedBank) CheckoutViewModel {
	return CheckoutViewModel{
		MerchantReference: payable.MerchantReference,
		PaymentCode:       paymentCode,
		Amount:            payable.Amount,
		Currency:          payable.Currency,
		Description:       firstNonEmpty(payable.Description, "Payment for "+payable.MerchantReference),
		CustomerName:      payable.CustomerName,
		CustomerCode:      payable.CustomerCode,
		CustomerPhone:     payable.CustomerPhone,
		Status:            status,
		PaymentStatus:     paymentStatus,
		PollIntervalMS:    int(pollInterval / time.Millisecond),
		SuccessURL:        payable.SuccessURL,
		CancelURL:         payable.CancelURL,
		Instructions:      instructions,
		SupportedBanks:    supportedBanks,
	}
}

func mergeStatusDisplay(status CheckoutStatusResult, payable Payable) CheckoutStatusResult {
	status.MerchantReference = firstNonEmpty(status.MerchantReference, payable.MerchantReference)
	status.PaymentCode = firstNonEmpty(status.PaymentCode, payable.WebirrPaymentCode)
	status.Amount = firstNonEmpty(status.Amount, payable.Amount)
	status.Currency = firstNonEmpty(status.Currency, payable.Currency, "ETB")
	status.Description = firstNonEmpty(status.Description, payable.Description)
	status.CustomerName = firstNonEmpty(status.CustomerName, payable.CustomerName)
	status.CustomerCode = firstNonEmpty(status.CustomerCode, payable.CustomerCode)
	status.CustomerPhone = firstNonEmpty(status.CustomerPhone, payable.CustomerPhone)
	if status.Status == "" {
		status.Status = StatusUnknown
	}
	return status
}

func instructionsForSupportedBanks(base CheckoutInstructions, supportedBanks []SupportedBank) CheckoutInstructions {
	title := firstNonEmpty(base.Title, "Payment Instruction")
	steps := make([]string, 0, len(supportedBanks))
	for _, bank := range supportedBanks {
		name := clean(bank.Name)
		if name == "" {
			continue
		}
		steps = append(steps, name+" -> WeBirr -> Payment Code")
	}
	return CheckoutInstructions{Title: title, Steps: steps}
}

func gatewayError[T any](response *webirr.ApiResponse[T], action string) error {
	if response == nil {
		return fmt.Errorf("could not %s: empty response", action)
	}
	if response.Error != "" {
		return fmt.Errorf("could not %s: %s", action, response.Error)
	}
	return nil
}

func billChanged(existing webirr.BillResponse, next *webirr.Bill) bool {
	return clean(existing.Amount) != clean(next.Amount) ||
		clean(existing.CustomerCode) != clean(next.CustomerCode) ||
		clean(existing.CustomerName) != clean(next.CustomerName) ||
		clean(existing.CustomerPhone) != clean(next.CustomerPhone) ||
		clean(existing.Description) != clean(next.Description)
}

func clean(value string) string {
	return strings.TrimSpace(value)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if clean(value) != "" {
			return clean(value)
		}
	}
	return ""
}

func intPtr(value int) *int {
	return &value
}

func statusInt(value *int, fallback int) int {
	if value == nil {
		return fallback
	}
	return *value
}

func formatIssuer(value string) string {
	value = clean(value)
	if value == "" {
		return ""
	}
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == '_' || r == '-' || r == ' '
	})
	for i, part := range parts {
		if len(part) <= 4 {
			parts[i] = strings.ToUpper(part)
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
	}
	return strings.Join(parts, " ")
}
