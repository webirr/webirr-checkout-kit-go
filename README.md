# WeBirr Checkout Kit for Go

![WeBirr Go checkout flow](examples/nethttp-memory/screenshots/go-checkout-journey.png)

Go backend helpers for WeBirr online checkout integrations. This package gives
custom Go merchant applications the same WeBirr online checkout pattern used by
the Moodle, WooCommerce, and JavaScript checkout-kit examples: the merchant
backend creates or resumes the WeBirr bill, the browser displays the WeBirr
Payment Code, the browser polls only merchant-owned endpoints, and the merchant
backend completes the payable after server-side verification.

The kit depends on the official Go client SDK:

```bash
go get github.com/webirr/webirr-checkout-kit-go
```

## Repository Layout

| Area | Path | Status |
| --- | --- | --- |
| Go checkout kit | repository root | Public package source for merchant-owned checkout endpoints. |
| `net/http` example | `examples/nethttp-memory` | Runnable local checkout demo with mock mode by default and optional WeBirr TestEnv mode. |
| Unit tests | `checkout_test.go` | Tests for bill creation/recovery/update, status resolver modes, HTTP handlers, supported-bank instructions, and local completion. |

## What Is A Payable?

In this package, a `Payable` is the merchant-side record being paid. Depending on
the merchant application, it can be:

- a WooCommerce-style order;
- a Moodle paid enrollment;
- a utility invoice;
- a customer bill;
- a booking, subscription, or service charge;
- any other merchant-owned record identified by `merchantReference`.

The WeBirr gateway creates a bill/payment code. The merchant application owns the
payable and resolves its amount, customer, description, and completion behavior.

## How The WeBirr Integration Works

The browser never calls WeBirr merchant APIs and never receives merchant API
credentials. The Go backend package is responsible for loading the merchant
payable, creating or recovering the WeBirr payment code, returning
merchant-supported banks, resolving payment status, and marking the payable paid
idempotently.

| Checkout role | Go package entry point | WeBirr call |
| --- | --- | --- |
| Load merchant payable | `Store.LoadPayable` | No WeBirr call; merchant database lookup |
| Create or resume payment code | `Checkout.CreateCheckout` / `Handler.CreateCheckout` | Create bill, recover bill by merchant reference, update unpaid bill when needed |
| Return supported banks | `Checkout.CreateCheckout` | `GET /einvoice/api/banks` through the official Go SDK |
| Poll payment status | `Checkout.GetStatus` / `Handler.GetStatus` | Default: server-side WeBirr payment-status check |
| Complete paid payable | `Store.MarkPaid` | Runs only after server-side paid verification |

The durable checkout key is `merchantReference`. No browser-facing checkout ID is
required for the baseline flow.

## Install

```bash
go get github.com/webirr/webirr-checkout-kit-go
go get github.com/webirr/webirr-api-go-client
```

## Configure

Keep merchant credentials on the server side:

```bash
export WEBIRR_TEST_ENV_MERCHANT_ID=your-test-merchant-id
export WEBIRR_TEST_ENV_API_KEY=your-test-env-api-key
```

## Basic Usage

```go
package main

import (
	"net/http"
	"os"

	checkout "github.com/webirr/webirr-checkout-kit-go"
	webirr "github.com/webirr/webirr-api-go-client"
)

func main() {
	client := webirr.NewClient(
		os.Getenv("WEBIRR_TEST_ENV_MERCHANT_ID"),
		os.Getenv("WEBIRR_TEST_ENV_API_KEY"),
		true,
	)

	store := checkout.NewMemoryStore()
	store.PutPayable(checkout.Payable{
		MerchantReference: "ord_2026_06_24_10033",
		Amount:            "640.00",
		Currency:          "ETB",
		CustomerName:      "Elias",
		CustomerCode:      "CUST-1001",
		CustomerPhone:     "0911000000",
		Description:       "Sample Audio Book",
		SuccessURL:        "/success",
		CancelURL:         "/cart",
	})

	handler := checkout.NewHandler(client, store)

	mux := http.NewServeMux()
	handler.Register(mux, "/webirr/checkout")
	http.ListenAndServe(":8080", mux)
}
```

The browser or mobile app calls only merchant-owned endpoints:

```http
POST /webirr/checkout
Content-Type: application/json

{"merchantReference":"ord_2026_06_24_10033"}
```

```http
GET /webirr/checkout/status?merchantReference=ord_2026_06_24_10033
```

The browser must not send the amount, API key, merchant ID, or WeBirr endpoint.
Those values are resolved by the merchant backend.

## Merchant Store

Production applications should implement `checkout.Store` using their own
database:

```go
type Store interface {
	LoadPayable(context.Context, string) (checkout.Payable, error)
	SavePaymentCode(context.Context, string, string) error
	MarkPaid(context.Context, string, checkout.PaymentResult) error
}
```

Responsibilities:

- `LoadPayable` resolves the order, invoice, enrollment, bill, booking, or other
  merchant-owned payable by `merchantReference`.
- `SavePaymentCode` stores the `merchantReference -> WeBirr Payment Code` mapping
  immediately after creation or recovery.
- `MarkPaid` completes the order, enrollment, receipt, service delivery, or access
  idempotently after server-side payment confirmation.

`checkout.NewMemoryStore()` is provided only for examples and tests.

## Default Payment Status

No status resolver is required for the common case:

```go
handler := checkout.NewHandler(client, store)
```

By default, the status endpoint calls WeBirr `GetPaymentStatus` through the
official Go SDK, updates the local store when paid, and returns safe status fields
to the checkout UI.

## Webhook Or Bulk Polling Status

If your application receives WeBirr webhook notifications or runs timestamp-based
bulk polling in the background, the checkout status endpoint can read from your
local payment table instead.

```go
handler := checkout.NewHandler(client, store, checkout.WithLocalStatus())
```

For local-first behavior with WeBirr fallback while still pending:

```go
handler := checkout.NewHandler(client, store, checkout.WithHybridStatus())
```

For full control:

```go
handler := checkout.NewHandler(
	client,
	store,
	checkout.WithStatusResolver(checkout.StatusResolverFunc(func(ctx context.Context, c *checkout.Checkout, payable checkout.Payable) (checkout.CheckoutStatusResult, error) {
		return checkout.CheckoutStatusResult{
			MerchantReference: payable.MerchantReference,
			Status:            checkout.StatusPending,
		}, nil
	})),
)
```

The status resolver answers only what the checkout UI should show now. Bill
creation and payment-code recovery stay in the create checkout flow.

## WeBirr Payment Flow

At a glance, the payment flow is:

### 1. Invoice Creation / Checkout On Purchase

- The customer starts a merchant checkout, invoice payment, paid enrollment, or
  similar payable flow.
- The merchant backend resolves the payable amount, customer, description, and
  stable `merchantReference`.
- The Go checkout kit creates or resumes the WeBirr bill and stores the WeBirr
  Payment Code through merchant callbacks.

### 2. Payment Code Display

- The browser displays the **WeBirr Payment Code**.
- Payment instructions are generated only from the merchant's `supportedBanks`
  response.
- The customer payment path is:
  `{Banking App} -> WeBirr menu -> Enter Payment Code -> Pay`.

### 3. Payment Status Monitoring

- Browser JavaScript polls the merchant backend status endpoint.
- The merchant backend resolves payment status using the default WeBirr status
  call, local webhook-updated state, local bulk-polling state, or a hybrid
  resolver.
- Manual refresh should appear only when polling fails; normal polling should be
  sequential.

### 4. Completion And Access

- Once WeBirr reports paid, or local verified status is paid, the merchant backend
  calls `MarkPaid` idempotently.
- The paid UI or success page shows Customer, Amount, Payment Reference, and Paid
  Via.

## Supported Banks

The create checkout response includes `supportedBanks`, loaded from the
merchant-scoped WeBirr supported banks endpoint. Checkout UI should render
instructions from that list, such as:

```text
CBE Mobile -> WeBirr -> Payment Code
Telebirr -> WeBirr -> Payment Code
```

Do not show a broad static bank list if the merchant's supported banks could not
be loaded.

## Screenshots

The `net/http` example screenshots show the same three-step online checkout flow
used by the Moodle, WooCommerce, and JavaScript checkout-kit examples.

![Go checkout review](examples/nethttp-memory/screenshots/go-checkout-01-review.png)

![Go payment code waiting](examples/nethttp-memory/screenshots/go-checkout-02-payment-code.png)

![Go payment confirmed](examples/nethttp-memory/screenshots/go-checkout-03-confirmed.png)

## Local Example

Run the example in mock mode:

```bash
go run ./examples/nethttp-memory
```

Mock mode requires no WeBirr credentials. It preserves the real architecture:
browser calls merchant-owned endpoints, the backend returns safe checkout fields,
and payment status changes through the backend.

Run the same example against WeBirr TestEnv:

```bash
WEBIRR_TEST_ENV_MERCHANT_ID=your-test-merchant-id \
WEBIRR_TEST_ENV_API_KEY=your-test-api-key \
go run ./examples/nethttp-memory
```

Then open `http://localhost:8080`.

## Run Tests

If Go is installed:

```bash
go test ./...
```

If Go is not installed locally, use Docker:

```bash
docker run --rm -v "$PWD":/src -w /src golang:1.22 \
  sh -lc "/usr/local/go/bin/gofmt -w . && /usr/local/go/bin/go test ./..."
```

## Release Status

This package is not released publicly yet. Before the first release, create the
GitHub repository, register it as a hub submodule, run package validation, tag a
reviewed version, and create a matching GitHub Release.
