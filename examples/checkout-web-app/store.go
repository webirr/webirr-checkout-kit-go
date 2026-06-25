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
  demo_type TEXT NOT NULL DEFAULT 'audiobook',
  item_id TEXT NOT NULL DEFAULT '',
  item_title TEXT NOT NULL DEFAULT '',
  customer_name TEXT NOT NULL,
  amount TEXT NOT NULL,
  currency TEXT NOT NULL DEFAULT 'ETB',
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
	if err != nil {
		return err
	}
	for _, column := range []struct {
		name       string
		definition string
	}{
		{"demo_type", "TEXT NOT NULL DEFAULT 'audiobook'"},
		{"item_id", "TEXT NOT NULL DEFAULT ''"},
		{"item_title", "TEXT NOT NULL DEFAULT ''"},
		{"currency", "TEXT NOT NULL DEFAULT 'ETB'"},
	} {
		if err := s.ensureColumn(column.name, column.definition); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLiteStore) UpsertOrder(order demoOrder) error {
	now := nowText()
	_, err := s.db.Exec(`
INSERT INTO webirr_checkouts (
  merchant_reference, demo_type, item_id, item_title, customer_name, amount,
  currency, description, created_at, updated_at
) VALUES (?, 'audiobook', ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(merchant_reference) DO UPDATE SET
  item_id = excluded.item_id,
  item_title = excluded.item_title,
  customer_name = excluded.customer_name,
  amount = excluded.amount,
  currency = excluded.currency,
  description = excluded.description,
  updated_at = excluded.updated_at
`, order.MerchantReference, order.ItemID, order.ItemTitle, order.CustomerName, order.Amount, order.Currency, order.Description, now, now)
	return err
}

func (s *SQLiteStore) LoadOrder(_ context.Context, merchantReference string) (demoOrder, error) {
	row := s.db.QueryRow(`
SELECT merchant_reference, item_id, item_title, customer_name, amount, currency, description
FROM webirr_checkouts
WHERE merchant_reference = ?
`, merchantReference)

	var order demoOrder
	if err := row.Scan(&order.MerchantReference, &order.ItemID, &order.ItemTitle, &order.CustomerName, &order.Amount, &order.Currency, &order.Description); err != nil {
		if err == sql.ErrNoRows {
			return demoOrder{}, fmt.Errorf("order %q was not found", merchantReference)
		}
		return demoOrder{}, err
	}
	order.CustomerCode = order.MerchantReference
	return order, nil
}

func (s *SQLiteStore) LoadPayable(_ context.Context, merchantReference string) (checkout.Payable, error) {
	row := s.db.QueryRow(`
SELECT merchant_reference, item_title, customer_name, amount, currency, description, webirr_payment_code, webirr_payment_status
FROM webirr_checkouts
WHERE merchant_reference = ?
`, merchantReference)

	var payable checkout.Payable
	var itemTitle string
	var paymentCode string
	var paymentStatus int
	if err := row.Scan(&payable.MerchantReference, &itemTitle, &payable.CustomerName, &payable.Amount, &payable.Currency, &payable.Description, &paymentCode, &paymentStatus); err != nil {
		if err == sql.ErrNoRows {
			return checkout.Payable{}, fmt.Errorf("payable %q was not found", merchantReference)
		}
		return checkout.Payable{}, err
	}
	payable.CustomerCode = merchantReference
	if itemTitle != "" {
		payable.Description = itemTitle + " - " + payable.Description
	}
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

func (s *SQLiteStore) LoadReceipt(_ context.Context, merchantReference string) (receiptData, error) {
	row := s.db.QueryRow(`
SELECT merchant_reference, item_id, item_title, customer_name, amount, currency, description,
       webirr_payment_code, webirr_payment_reference, webirr_paid_via, paid_at
FROM webirr_checkouts
WHERE merchant_reference = ? AND webirr_payment_status = 2
`, merchantReference)

	var receipt receiptData
	var paidAt sql.NullString
	if err := row.Scan(
		&receipt.MerchantReference,
		&receipt.ItemID,
		&receipt.ItemTitle,
		&receipt.CustomerName,
		&receipt.Amount,
		&receipt.Currency,
		&receipt.Description,
		&receipt.PaymentCode,
		&receipt.PaymentReference,
		&receipt.PaidVia,
		&paidAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return receiptData{}, fmt.Errorf("paid order %q was not found", merchantReference)
		}
		return receiptData{}, err
	}
	receipt.CustomerCode = receipt.MerchantReference
	if paidAt.Valid {
		receipt.PaidAt = paidAt.String
	}
	return receipt, nil
}

func (s *SQLiteStore) ensureColumn(name, definition string) error {
	rows, err := s.db.Query(`PRAGMA table_info(webirr_checkouts)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var columnName, columnType string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &columnName, &columnType, &notNull, &defaultValue, &pk); err != nil {
			return err
		}
		if columnName == name {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = s.db.Exec(`ALTER TABLE webirr_checkouts ADD COLUMN ` + name + ` ` + definition)
	return err
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
