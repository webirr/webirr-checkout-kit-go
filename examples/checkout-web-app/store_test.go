package main

import (
	"context"
	"path/filepath"
	"testing"

	checkout "github.com/webirr/webirr-checkout-kit-go"
)

func TestSQLiteStorePersistsPaymentCodeAndRecoversAfterReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "checkout.sqlite3")
	store, err := OpenSQLiteStore(path)
	if err != nil {
		t.Fatalf("OpenSQLiteStore returned error: %v", err)
	}
	order := demoOrder{
		MerchantReference: "ord_2026_06_24_10033",
		ItemID:            "audio-book-001",
		ItemTitle:         "Modern Business Audio Book",
		CustomerName:      "Elias",
		Amount:            "640.00",
		Currency:          "ETB",
		Description:       "Sample Audio Book",
	}
	if err := store.UpsertOrder(order); err != nil {
		t.Fatalf("UpsertOrder returned error: %v", err)
	}
	if err := store.SavePaymentCode(context.Background(), order.MerchantReference, "451 728 230"); err != nil {
		t.Fatalf("SavePaymentCode returned error: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	reopened, err := OpenSQLiteStore(path)
	if err != nil {
		t.Fatalf("reopen returned error: %v", err)
	}
	defer reopened.Close()

	payable, err := reopened.LoadPayable(context.Background(), order.MerchantReference)
	if err != nil {
		t.Fatalf("LoadPayable returned error: %v", err)
	}
	if payable.WebirrPaymentCode != "451 728 230" {
		t.Fatalf("payment code = %q, want 451 728 230", payable.WebirrPaymentCode)
	}
	if payable.PaymentStatus == nil || *payable.PaymentStatus != 0 {
		t.Fatalf("payment status = %#v, want 0", payable.PaymentStatus)
	}
}

func TestSQLiteStoreUpdatesDetailsWithoutClearingPaymentCode(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "checkout.sqlite3"))
	if err != nil {
		t.Fatalf("OpenSQLiteStore returned error: %v", err)
	}
	defer store.Close()

	order := demoOrder{MerchantReference: "ord_update", ItemID: "audio-book-001", ItemTitle: "Modern Business Audio Book", CustomerName: "Elias", Amount: "640.00", Currency: "ETB", Description: "Sample Audio Book"}
	if err := store.UpsertOrder(order); err != nil {
		t.Fatalf("UpsertOrder returned error: %v", err)
	}
	if err := store.SavePaymentCode(context.Background(), order.MerchantReference, "451 728 230"); err != nil {
		t.Fatalf("SavePaymentCode returned error: %v", err)
	}
	order.Amount = "645.00"
	order.ItemTitle = "Updated Audio Book"
	order.Description = "Digital audio book purchase"
	if err := store.UpsertOrder(order); err != nil {
		t.Fatalf("second UpsertOrder returned error: %v", err)
	}

	payable, err := store.LoadPayable(context.Background(), order.MerchantReference)
	if err != nil {
		t.Fatalf("LoadPayable returned error: %v", err)
	}
	if payable.Amount != "645.00" || payable.Description != "Updated Audio Book - Digital audio book purchase" {
		t.Fatalf("payable details = %#v", payable)
	}
	if payable.WebirrPaymentCode != "451 728 230" {
		t.Fatalf("payment code was cleared: %#v", payable)
	}
}

func TestSQLiteStoreMarksPaidAndReversed(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "checkout.sqlite3"))
	if err != nil {
		t.Fatalf("OpenSQLiteStore returned error: %v", err)
	}
	defer store.Close()

	order := demoOrder{MerchantReference: "ord_paid", ItemID: "audio-book-001", ItemTitle: "Modern Business Audio Book", CustomerName: "Elias", Amount: "640.00", Currency: "ETB", Description: "Digital audio book purchase"}
	if err := store.UpsertOrder(order); err != nil {
		t.Fatalf("UpsertOrder returned error: %v", err)
	}
	err = store.MarkPaid(context.Background(), order.MerchantReference, checkout.PaymentResult{
		PaymentCode:      "451 728 230",
		PaymentReference: "TX123",
		PaymentIssuer:    "CBE Mobile",
		PaidAt:           "2026-06-24T10:30:00Z",
	})
	if err != nil {
		t.Fatalf("MarkPaid returned error: %v", err)
	}

	status, err := store.LoadCheckoutStatus(context.Background(), order.MerchantReference)
	if err != nil {
		t.Fatalf("LoadCheckoutStatus returned error: %v", err)
	}
	if status.Status != checkout.StatusPaid || status.PaymentReference != "TX123" || status.PaymentIssuer != "CBE Mobile" {
		t.Fatalf("paid status = %#v", status)
	}
	receipt, err := store.LoadReceipt(context.Background(), order.MerchantReference)
	if err != nil {
		t.Fatalf("LoadReceipt returned error: %v", err)
	}
	if receipt.ItemTitle != "Modern Business Audio Book" || receipt.PaymentReference != "TX123" {
		t.Fatalf("receipt = %#v", receipt)
	}

	if err := store.MarkReversed(order.MerchantReference); err != nil {
		t.Fatalf("MarkReversed returned error: %v", err)
	}
	status, err = store.LoadCheckoutStatus(context.Background(), order.MerchantReference)
	if err != nil {
		t.Fatalf("LoadCheckoutStatus after reversal returned error: %v", err)
	}
	if status.PaymentStatus == nil || *status.PaymentStatus != 3 || status.Status != checkout.StatusFailed {
		t.Fatalf("reversed status = %#v", status)
	}
}
