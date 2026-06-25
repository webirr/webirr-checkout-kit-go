package main

import (
	"context"
	"sync"

	webirr "github.com/webirr/webirr-api-go-client"
)

type mockGateway struct {
	mu          sync.Mutex
	statusCalls map[string]int
	bills       map[string]webirr.BillResponse
}

func newMockGateway() *mockGateway {
	return &mockGateway{
		statusCalls: make(map[string]int),
		bills:       make(map[string]webirr.BillResponse),
	}
}

func (g *mockGateway) CreateBill(_ context.Context, bill *webirr.Bill) (*webirr.ApiResponse[string], error) {
	paymentCode := "451 728 230"
	g.mu.Lock()
	g.bills[paymentCode] = billResponseFromBill(bill, paymentCode)
	g.mu.Unlock()
	return &webirr.ApiResponse[string]{Res: paymentCode}, nil
}

func (g *mockGateway) UpdateBill(_ context.Context, bill *webirr.Bill) (*webirr.ApiResponse[string], error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for paymentCode, existing := range g.bills {
		if existing.BillReference == bill.BillReference {
			g.bills[paymentCode] = billResponseFromBill(bill, paymentCode)
			break
		}
	}
	return &webirr.ApiResponse[string]{Res: "OK"}, nil
}

func (g *mockGateway) GetPaymentStatus(_ context.Context, paymentCode string) (*webirr.ApiResponse[webirr.PaymentStatus], error) {
	g.mu.Lock()
	g.statusCalls[paymentCode]++
	calls := g.statusCalls[paymentCode]
	g.mu.Unlock()

	if calls < 3 {
		return &webirr.ApiResponse[webirr.PaymentStatus]{Res: webirr.PaymentStatus{Status: 0}}, nil
	}

	return &webirr.ApiResponse[webirr.PaymentStatus]{
		Res: webirr.PaymentStatus{
			Status: 2,
			Data: &webirr.PaymentDetail{
				Status:           2,
				PaymentReference: "TX9f7eli77683004b489b9e99",
				BankID:           "cbe_mobile",
				PaymentDate:      "2026-06-24 10:30",
				WbcCode:          paymentCode,
				Amount:           "640.00",
			},
		},
	}, nil
}

func (g *mockGateway) GetBillByReference(_ context.Context, billReference string) (*webirr.ApiResponse[webirr.BillResponse], error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, bill := range g.bills {
		if bill.BillReference == billReference {
			return &webirr.ApiResponse[webirr.BillResponse]{Res: bill}, nil
		}
	}
	return &webirr.ApiResponse[webirr.BillResponse]{Error: "not found"}, nil
}

func (g *mockGateway) GetBillByPaymentCode(_ context.Context, paymentCode string) (*webirr.ApiResponse[webirr.BillResponse], error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	bill, ok := g.bills[paymentCode]
	if !ok {
		return &webirr.ApiResponse[webirr.BillResponse]{Error: "not found"}, nil
	}
	return &webirr.ApiResponse[webirr.BillResponse]{Res: bill}, nil
}

func (g *mockGateway) GetSupportedBanks(_ context.Context) (*webirr.ApiResponse[[]webirr.SupportedBank], error) {
	return &webirr.ApiResponse[[]webirr.SupportedBank]{
		Res: []webirr.SupportedBank{
			{BankID: "cbe_mobile", Name: "CBE Mobile"},
			{BankID: "cbe_birr", Name: "CBE Birr"},
			{BankID: "awash_birr", Name: "Awash Birr"},
			{BankID: "telebirr", Name: "Telebirr"},
			{BankID: "mpesa", Name: "M-Pesa"},
		},
	}, nil
}

func billResponseFromBill(bill *webirr.Bill, paymentCode string) webirr.BillResponse {
	return webirr.BillResponse{
		Amount:        bill.Amount,
		CustomerCode:  bill.CustomerCode,
		CustomerName:  bill.CustomerName,
		CustomerPhone: bill.CustomerPhone,
		Description:   bill.Description,
		BillReference: bill.BillReference,
		WbcCode:       paymentCode,
		PaymentStatus: 0,
	}
}
