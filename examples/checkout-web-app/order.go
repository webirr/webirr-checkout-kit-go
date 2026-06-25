package main

import checkout "github.com/webirr/webirr-checkout-kit-go"

type demoOrder struct {
	MerchantReference string
	ItemID            string
	ItemTitle         string
	Amount            string
	Currency          string
	CustomerName      string
	CustomerCode      string
	CustomerPhone     string
	Description       string
}

func (o demoOrder) OrderURL() string {
	return "/orders/" + o.MerchantReference
}

func (o demoOrder) CheckoutURL() string {
	return "/checkout?merchantReference=" + o.MerchantReference
}

func (o demoOrder) SuccessURL() string {
	return o.OrderURL() + "/success"
}

func (o demoOrder) ReceiptURL() string {
	return o.OrderURL() + "/receipt.txt"
}

func (o demoOrder) Payable() checkout.Payable {
	return checkout.Payable{
		MerchantReference: o.MerchantReference,
		Amount:            o.Amount,
		Currency:          o.Currency,
		CustomerName:      o.CustomerName,
		CustomerCode:      o.CustomerCode,
		CustomerPhone:     o.CustomerPhone,
		Description:       o.ItemTitle + " - " + o.Description,
		SuccessURL:        o.SuccessURL(),
		CancelURL:         o.OrderURL(),
	}
}

type receiptData struct {
	demoOrder
	PaymentCode      string
	PaymentReference string
	PaidVia          string
	PaidAt           string
}

type catalogPage struct {
	CustomerName string
	Books        []demoBook
	Error        string
}
