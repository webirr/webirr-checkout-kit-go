package checkout

import (
	"context"
	"fmt"
	"sync"
)

// MemoryStore is a small in-memory store for examples and tests.
// Production merchants should implement Store using their own database.
type MemoryStore struct {
	mu       sync.Mutex
	payable  map[string]Payable
	statuses map[string]CheckoutStatusResult
	paid     map[string]PaymentResult
}

// NewMemoryStore creates an empty example store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		payable:  make(map[string]Payable),
		statuses: make(map[string]CheckoutStatusResult),
		paid:     make(map[string]PaymentResult),
	}
}

// PutPayable inserts or replaces a payable for examples/tests.
func (s *MemoryStore) PutPayable(payable Payable) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.payable[payable.MerchantReference] = payable
}

// PutStatus inserts or replaces local checkout status for examples/tests.
func (s *MemoryStore) PutStatus(status CheckoutStatusResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.statuses[status.MerchantReference] = status
}

func (s *MemoryStore) LoadPayable(_ context.Context, merchantReference string) (Payable, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	payable, ok := s.payable[merchantReference]
	if !ok {
		return Payable{}, fmt.Errorf("payable %q was not found", merchantReference)
	}
	return payable, nil
}

func (s *MemoryStore) SavePaymentCode(_ context.Context, merchantReference, paymentCode string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	payable, ok := s.payable[merchantReference]
	if !ok {
		return fmt.Errorf("payable %q was not found", merchantReference)
	}
	payable.WebirrPaymentCode = paymentCode
	payable.PaymentStatus = intPtr(1)
	s.payable[merchantReference] = payable
	s.statuses[merchantReference] = mergeStatusDisplay(CheckoutStatusResult{
		MerchantReference: merchantReference,
		PaymentCode:       paymentCode,
		Status:            StatusPending,
		PaymentStatus:     intPtr(1),
	}, payable)
	return nil
}

func (s *MemoryStore) MarkPaid(_ context.Context, merchantReference string, payment PaymentResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	payable, ok := s.payable[merchantReference]
	if !ok {
		return fmt.Errorf("payable %q was not found", merchantReference)
	}
	payable.WebirrPaymentCode = firstNonEmpty(payable.WebirrPaymentCode, payment.PaymentCode)
	payable.PaymentStatus = intPtr(2)
	s.payable[merchantReference] = payable
	s.paid[merchantReference] = payment
	s.statuses[merchantReference] = mergeStatusDisplay(CheckoutStatusResult{
		MerchantReference: merchantReference,
		PaymentCode:       payable.WebirrPaymentCode,
		Status:            StatusPaid,
		PaymentStatus:     intPtr(2),
		PaymentReference:  payment.PaymentReference,
		PaymentIssuer:     payment.PaymentIssuer,
		PaidAt:            payment.PaidAt,
		ReceiptURL:        payable.SuccessURL,
	}, payable)
	return nil
}

func (s *MemoryStore) LoadCheckoutStatus(_ context.Context, merchantReference string) (CheckoutStatusResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if status, ok := s.statuses[merchantReference]; ok {
		return status, nil
	}
	payable, ok := s.payable[merchantReference]
	if !ok {
		return CheckoutStatusResult{}, fmt.Errorf("payable %q was not found", merchantReference)
	}
	return mergeStatusDisplay(CheckoutStatusResult{
		MerchantReference: merchantReference,
		PaymentCode:       payable.WebirrPaymentCode,
		Status:            StatusUnknown,
		PaymentStatus:     payable.PaymentStatus,
	}, payable), nil
}
