package checkout

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	webirr "github.com/webirr/webirr-api-go-client"
)

func TestCreateCheckoutCreatesBillAndReturnsSupportedBanks(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	store.PutPayable(testPayable("ord_2026_06_24_10033"))
	gateway := &fakeGateway{
		createCode: "835771",
		supportedBanks: []webirr.SupportedBank{
			{BankID: "cbe_mobile", Name: "CBE Mobile"},
			{BankID: "telebirr", Name: "Telebirr"},
		},
	}

	service := New(gateway, store)
	view, err := service.CreateCheckout(ctx, "ord_2026_06_24_10033")
	if err != nil {
		t.Fatalf("CreateCheckout returned error: %v", err)
	}

	if view.PaymentCode != "835771" {
		t.Fatalf("PaymentCode = %q, want 835771", view.PaymentCode)
	}
	if gateway.createCalls != 1 {
		t.Fatalf("createCalls = %d, want 1", gateway.createCalls)
	}
	if len(view.SupportedBanks) != 2 {
		t.Fatalf("supported banks = %d, want 2", len(view.SupportedBanks))
	}
	if len(view.Instructions.Steps) != 2 || view.Instructions.Steps[0] != "CBE Mobile -> WeBirr -> Payment Code" {
		t.Fatalf("unexpected instructions: %#v", view.Instructions.Steps)
	}

	status, err := store.LoadCheckoutStatus(ctx, "ord_2026_06_24_10033")
	if err != nil {
		t.Fatalf("LoadCheckoutStatus returned error: %v", err)
	}
	if status.PaymentCode != "835771" || status.Status != StatusPending {
		t.Fatalf("saved local status = %#v", status)
	}
}

func TestCreateCheckoutRecoversExistingBillByMerchantReference(t *testing.T) {
	store := NewMemoryStore()
	store.PutPayable(testPayable("ord_recover"))
	gateway := &fakeGateway{
		recoveredBill: &webirr.BillResponse{
			BillReference: "ord_recover",
			WbcCode:       "765432",
			PaymentStatus: 1,
		},
	}

	view, err := New(gateway, store).CreateCheckout(context.Background(), "ord_recover")
	if err != nil {
		t.Fatalf("CreateCheckout returned error: %v", err)
	}
	if view.PaymentCode != "765432" {
		t.Fatalf("PaymentCode = %q, want 765432", view.PaymentCode)
	}
	if gateway.createCalls != 0 {
		t.Fatalf("createCalls = %d, want 0", gateway.createCalls)
	}
}

func TestCreateCheckoutUpdatesExistingUnpaidBillWhenPayableChanges(t *testing.T) {
	store := NewMemoryStore()
	payable := testPayable("ord_update")
	payable.WebirrPaymentCode = "500123"
	store.PutPayable(payable)
	gateway := &fakeGateway{
		paymentStatus: webirr.PaymentStatus{Status: 1},
		billByCode: &webirr.BillResponse{
			BillReference: "ord_update",
			WbcCode:       "500123",
			Amount:        "100.00",
			CustomerName:  "Elias",
			Description:   "old item",
			PaymentStatus: 1,
		},
	}

	if _, err := New(gateway, store).CreateCheckout(context.Background(), "ord_update"); err != nil {
		t.Fatalf("CreateCheckout returned error: %v", err)
	}
	if gateway.updateCalls != 1 {
		t.Fatalf("updateCalls = %d, want 1", gateway.updateCalls)
	}
}

func TestCreateCheckoutPropagatesPlatformErrors(t *testing.T) {
	store := NewMemoryStore()
	store.PutPayable(testPayable("ord_platform_error"))
	expected := errors.New("connection reset")
	gateway := &fakeGateway{createErr: expected}

	_, err := New(gateway, store).CreateCheckout(context.Background(), "ord_platform_error")
	if !errors.Is(err, expected) {
		t.Fatalf("CreateCheckout error = %v, want %v", err, expected)
	}
}

func TestCreateCheckoutKeepsBusinessErrorsInApiResponsePath(t *testing.T) {
	store := NewMemoryStore()
	store.PutPayable(testPayable("ord_business_error"))
	gateway := &fakeGateway{
		createResponse: &webirr.ApiResponse[string]{
			Error:     "invalid amount",
			ErrorCode: "INVALID_AMOUNT",
		},
	}

	_, err := New(gateway, store).CreateCheckout(context.Background(), "ord_business_error")
	if err == nil || !strings.Contains(err.Error(), "could not create bill: invalid amount") {
		t.Fatalf("CreateCheckout error = %v, want create bill business error", err)
	}
}

func TestDefaultStatusResolverMarksPaidFromGateway(t *testing.T) {
	store := NewMemoryStore()
	payable := testPayable("ord_paid")
	payable.WebirrPaymentCode = "900111"
	store.PutPayable(payable)
	gateway := &fakeGateway{
		paymentStatus: webirr.PaymentStatus{
			Status: 2,
			Data: &webirr.PaymentDetail{
				Status:           2,
				PaymentReference: "TX123",
				BankID:           "cbe_mobile",
				PaymentDate:      "2026-06-24 10:00",
				WbcCode:          "900111",
			},
		},
	}

	status, err := New(gateway, store).GetStatus(context.Background(), "ord_paid")
	if err != nil {
		t.Fatalf("GetStatus returned error: %v", err)
	}
	if status.Status != StatusPaid || status.PaymentReference != "TX123" || status.PaymentIssuer != "CBE Mobile" {
		t.Fatalf("status = %#v", status)
	}

	again, err := New(gateway, store).GetStatus(context.Background(), "ord_paid")
	if err != nil {
		t.Fatalf("second GetStatus returned error: %v", err)
	}
	if again.Status != StatusPaid {
		t.Fatalf("second status = %#v", again)
	}
}

func TestLocalStatusResolverDoesNotCallGateway(t *testing.T) {
	store := NewMemoryStore()
	payable := testPayable("ord_local")
	payable.WebirrPaymentCode = "222333"
	store.PutPayable(payable)
	store.PutStatus(CheckoutStatusResult{
		MerchantReference: "ord_local",
		PaymentCode:       "222333",
		Status:            StatusPaid,
		PaymentStatus:     intPtr(2),
		PaymentReference:  "LOCALREF",
		PaymentIssuer:     "Telebirr",
	})
	gateway := &fakeGateway{}

	status, err := New(gateway, store, WithLocalStatus()).GetStatus(context.Background(), "ord_local")
	if err != nil {
		t.Fatalf("GetStatus returned error: %v", err)
	}
	if status.Status != StatusPaid || status.PaymentReference != "LOCALREF" {
		t.Fatalf("status = %#v", status)
	}
	if gateway.statusCalls != 0 {
		t.Fatalf("statusCalls = %d, want 0", gateway.statusCalls)
	}
}

func TestHybridStatusResolverFallsBackToGatewayWhilePending(t *testing.T) {
	store := NewMemoryStore()
	payable := testPayable("ord_hybrid")
	payable.WebirrPaymentCode = "222444"
	store.PutPayable(payable)
	store.PutStatus(CheckoutStatusResult{
		MerchantReference: "ord_hybrid",
		PaymentCode:       "222444",
		Status:            StatusPending,
		PaymentStatus:     intPtr(1),
	})
	gateway := &fakeGateway{paymentStatus: webirr.PaymentStatus{Status: 1}}

	status, err := New(gateway, store, WithHybridStatus()).GetStatus(context.Background(), "ord_hybrid")
	if err != nil {
		t.Fatalf("GetStatus returned error: %v", err)
	}
	if status.Status != StatusPending {
		t.Fatalf("status = %#v", status)
	}
	if gateway.statusCalls != 1 {
		t.Fatalf("statusCalls = %d, want 1", gateway.statusCalls)
	}
}

func TestCustomStatusResolver(t *testing.T) {
	store := NewMemoryStore()
	store.PutPayable(testPayable("ord_custom"))
	resolver := StatusResolverFunc(func(_ context.Context, _ *Checkout, payable Payable) (CheckoutStatusResult, error) {
		return CheckoutStatusResult{
			MerchantReference: payable.MerchantReference,
			Status:            StatusUnknown,
		}, nil
	})

	status, err := New(&fakeGateway{}, store, WithStatusResolver(resolver)).GetStatus(context.Background(), "ord_custom")
	if err != nil {
		t.Fatalf("GetStatus returned error: %v", err)
	}
	if status.Status != StatusUnknown {
		t.Fatalf("status = %#v", status)
	}
}

func TestHTTPHandlers(t *testing.T) {
	store := NewMemoryStore()
	store.PutPayable(testPayable("ord_http"))
	gateway := &fakeGateway{createCode: "555888"}
	handler := NewHandler(gateway, store)

	body := bytes.NewBufferString(`{"merchantReference":"ord_http"}`)
	req := httptest.NewRequest(http.MethodPost, "/webirr/checkout", body)
	rec := httptest.NewRecorder()
	handler.CreateCheckout(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("CreateCheckout HTTP status = %d body=%s", rec.Code, rec.Body.String())
	}
	var view CheckoutViewModel
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatalf("could not decode create response: %v", err)
	}
	if view.PaymentCode != "555888" {
		t.Fatalf("PaymentCode = %q, want 555888", view.PaymentCode)
	}

	req = httptest.NewRequest(http.MethodGet, "/webirr/checkout/status?merchantReference=ord_http", nil)
	rec = httptest.NewRecorder()
	handler.GetStatus(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GetStatus HTTP status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func testPayable(reference string) Payable {
	return Payable{
		MerchantReference: reference,
		Amount:            "640.00",
		Currency:          "ETB",
		CustomerName:      "Elias",
		CustomerCode:      "CUST-1001",
		CustomerPhone:     "0911000000",
		Description:       "Sample Audio Book",
		SuccessURL:        "/success",
		CancelURL:         "/cart",
	}
}

type fakeGateway struct {
	createCode     string
	createResponse *webirr.ApiResponse[string]
	createErr      error
	recoveredBill  *webirr.BillResponse
	billByCode     *webirr.BillResponse
	paymentStatus  webirr.PaymentStatus
	supportedBanks []webirr.SupportedBank

	createCalls int
	updateCalls int
	statusCalls int
}

func (g *fakeGateway) CreateBill(_ context.Context, _ *webirr.Bill) (*webirr.ApiResponse[string], error) {
	g.createCalls++
	if g.createErr != nil {
		return nil, g.createErr
	}
	if g.createResponse != nil {
		return g.createResponse, nil
	}
	return &webirr.ApiResponse[string]{Res: firstNonEmpty(g.createCode, "100200")}, nil
}

func (g *fakeGateway) UpdateBill(_ context.Context, _ *webirr.Bill) (*webirr.ApiResponse[string], error) {
	g.updateCalls++
	return &webirr.ApiResponse[string]{Res: "OK"}, nil
}

func (g *fakeGateway) GetPaymentStatus(_ context.Context, _ string) (*webirr.ApiResponse[webirr.PaymentStatus], error) {
	g.statusCalls++
	return &webirr.ApiResponse[webirr.PaymentStatus]{Res: g.paymentStatus}, nil
}

func (g *fakeGateway) GetBillByReference(_ context.Context, _ string) (*webirr.ApiResponse[webirr.BillResponse], error) {
	if g.recoveredBill == nil {
		return &webirr.ApiResponse[webirr.BillResponse]{Error: "not found"}, nil
	}
	return &webirr.ApiResponse[webirr.BillResponse]{Res: *g.recoveredBill}, nil
}

func (g *fakeGateway) GetBillByPaymentCode(_ context.Context, _ string) (*webirr.ApiResponse[webirr.BillResponse], error) {
	if g.billByCode == nil {
		return &webirr.ApiResponse[webirr.BillResponse]{Error: "not found"}, nil
	}
	return &webirr.ApiResponse[webirr.BillResponse]{Res: *g.billByCode}, nil
}

func (g *fakeGateway) GetSupportedBanks(_ context.Context) (*webirr.ApiResponse[[]webirr.SupportedBank], error) {
	return &webirr.ApiResponse[[]webirr.SupportedBank]{Res: g.supportedBanks}, nil
}
