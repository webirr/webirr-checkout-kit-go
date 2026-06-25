package main

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	checkout "github.com/webirr/webirr-checkout-kit-go"

	_ "modernc.org/sqlite"
)

type SQLiteStore struct {
	db *sql.DB
}

func OpenSQLiteStore(path string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	store := &SQLiteStore{db: db}
	if err := store.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

func (s *SQLiteStore) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS webirr_checkouts (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  merchant_reference TEXT NOT NULL UNIQUE,
  customer_name TEXT NOT NULL,
  amount TEXT NOT NULL,
  description TEXT NOT NULL,
  webirr_payment_code TEXT NOT NULL DEFAULT '',
  webirr_payment_status INTEGER NOT NULL DEFAULT 0,
  webirr_payment_reference TEXT NOT NULL DEFAULT '',
  webirr_paid_via TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  paid_at TEXT,
  reversed_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_webirr_checkouts_status ON webirr_checkouts(webirr_payment_status);
`)
	return err
}

func (s *SQLiteStore) UpsertOrder(order demoOrder) error {
	now := nowText()
	_, err := s.db.Exec(`
INSERT INTO webirr_checkouts (
  merchant_reference, customer_name, amount, description, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(merchant_reference) DO UPDATE SET
  customer_name = excluded.customer_name,
  amount = excluded.amount,
  description = excluded.description,
  updated_at = excluded.updated_at
`, order.MerchantReference, order.CustomerName, order.Amount, order.Description, now, now)
	return err
}

func (s *SQLiteStore) LoadPayable(_ context.Context, merchantReference string) (checkout.Payable, error) {
	row := s.db.QueryRow(`
SELECT merchant_reference, customer_name, amount, description, webirr_payment_code, webirr_payment_status
FROM webirr_checkouts
WHERE merchant_reference = ?
`, merchantReference)

	var payable checkout.Payable
	var paymentCode string
	var paymentStatus int
	if err := row.Scan(&payable.MerchantReference, &payable.CustomerName, &payable.Amount, &payable.Description, &paymentCode, &paymentStatus); err != nil {
		if err == sql.ErrNoRows {
			return checkout.Payable{}, fmt.Errorf("payable %q was not found", merchantReference)
		}
		return checkout.Payable{}, err
	}
	payable.Currency = "ETB"
	payable.CustomerCode = merchantReference
	payable.WebirrPaymentCode = paymentCode
	payable.PaymentStatus = &paymentStatus
	payable.SuccessURL = "/orders/" + merchantReference + "/success"
	payable.CancelURL = "/orders/" + merchantReference
	return payable, nil
}

func (s *SQLiteStore) SavePaymentCode(_ context.Context, merchantReference, paymentCode string) error {
	result, err := s.db.Exec(`
UPDATE webirr_checkouts
SET webirr_payment_code = ?, webirr_payment_status = 0, updated_at = ?
WHERE merchant_reference = ?
`, paymentCode, nowText(), merchantReference)
	if err != nil {
		return err
	}
	return requireUpdated(result, merchantReference)
}

func (s *SQLiteStore) MarkPaid(_ context.Context, merchantReference string, payment checkout.PaymentResult) error {
	paidAt := firstNonEmpty(payment.PaidAt, nowText())
	result, err := s.db.Exec(`
UPDATE webirr_checkouts
SET webirr_payment_code = COALESCE(NULLIF(?, ''), webirr_payment_code),
    webirr_payment_status = 2,
    webirr_payment_reference = ?,
    webirr_paid_via = ?,
    paid_at = ?,
    updated_at = ?
WHERE merchant_reference = ?
`, payment.PaymentCode, payment.PaymentReference, payment.PaymentIssuer, paidAt, nowText(), merchantReference)
	if err != nil {
		return err
	}
	return requireUpdated(result, merchantReference)
}

func (s *SQLiteStore) MarkReversed(merchantReference string) error {
	result, err := s.db.Exec(`
UPDATE webirr_checkouts
SET webirr_payment_status = 3, reversed_at = ?, updated_at = ?
WHERE merchant_reference = ?
`, nowText(), nowText(), merchantReference)
	if err != nil {
		return err
	}
	return requireUpdated(result, merchantReference)
}

func (s *SQLiteStore) LoadCheckoutStatus(_ context.Context, merchantReference string) (checkout.CheckoutStatusResult, error) {
	row := s.db.QueryRow(`
SELECT merchant_reference, customer_name, amount, description, webirr_payment_code,
       webirr_payment_status, webirr_payment_reference, webirr_paid_via, paid_at
FROM webirr_checkouts
WHERE merchant_reference = ?
`, merchantReference)

	var status checkout.CheckoutStatusResult
	var paymentStatus int
	var paidAt sql.NullString
	if err := row.Scan(
		&status.MerchantReference,
		&status.CustomerName,
		&status.Amount,
		&status.Description,
		&status.PaymentCode,
		&paymentStatus,
		&status.PaymentReference,
		&status.PaymentIssuer,
		&paidAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return checkout.CheckoutStatusResult{}, fmt.Errorf("payable %q was not found", merchantReference)
		}
		return checkout.CheckoutStatusResult{}, err
	}

	status.Currency = "ETB"
	status.PaymentStatus = &paymentStatus
	switch paymentStatus {
	case 2:
		status.Status = checkout.StatusPaid
		status.ReceiptURL = "/orders/" + merchantReference + "/success"
	case 3:
		status.Status = checkout.StatusFailed
	default:
		status.Status = checkout.StatusPending
	}
	if paidAt.Valid {
		status.PaidAt = paidAt.String
	}
	return status, nil
}

func requireUpdated(result sql.Result, merchantReference string) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("payable %q was not found", merchantReference)
	}
	return nil
}

func nowText() string {
	return time.Now().UTC().Format(time.RFC3339)
}
