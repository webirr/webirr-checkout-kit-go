# WeBirr Checkout Web App Example

This is a runnable Go web app that demonstrates the merchant-owned WeBirr online
checkout pattern.

The app shows:

- an order review page;
- a checkout page that displays the WeBirr Payment Code;
- merchant-owned create and status endpoints;
- SQLite-backed retry and recovery state;
- mock mode for local UI checks;
- optional WeBirr TestEnv or ProdEnv mode.

## Run

Mock mode needs no WeBirr credentials:

```bash
cd examples/checkout-web-app
go run .
```

Open:

```text
http://localhost:8080
```

The example creates a local SQLite database named
`webirr-checkout-demo.sqlite3`. It is ignored by Git.

## TestEnv Mode

Keep merchant credentials on the server side:

```bash
cd examples/checkout-web-app
WEBIRR_MERCHANT_ID=your-test-merchant-id \
WEBIRR_API_KEY=your-test-api-key \
WEBIRR_TEST_MODE=true \
go run .
```

TestEnv mode creates a real WeBirr TestEnv bill and displays the real WeBirr
Payment Code format. Payment remains pending until the code is paid through an
approved TestEnv banking app or simulator.

## ProdEnv Mode

Use production credentials only from a merchant production deployment:

```bash
cd examples/checkout-web-app
WEBIRR_MERCHANT_ID=your-production-merchant-id \
WEBIRR_API_KEY=your-production-api-key \
WEBIRR_TEST_MODE=false \
go run .
```

## Demo Values

Optional local demo values:

```bash
WEBIRR_DEMO_MERCHANT_REFERENCE=ord_2026_06_24_10033
WEBIRR_DEMO_AMOUNT=640.00
WEBIRR_DEMO_DESCRIPTION="Sample Audio Book"
WEBIRR_DEMO_CUSTOMER_NAME=Elias
WEBIRR_DEMO_SQLITE_PATH=webirr-checkout-demo.sqlite3
```

## Endpoints

The browser calls only the merchant backend:

| Route | Purpose |
| --- | --- |
| `GET /orders/{merchantReference}` | Order review page. |
| `GET /checkout?merchantReference=...` | Checkout page. |
| `POST /webirr/checkout` | Create or resume the WeBirr payment code. |
| `GET /webirr/checkout/status?merchantReference=...` | Poll status and complete the local payable when paid. |
| `GET /orders/{merchantReference}/success` | Merchant success page. |

Create request:

```json
{"merchantReference":"ord_2026_06_24_10033"}
```

The browser never sends the amount, API key, merchant ID, or WeBirr endpoint.
Those are resolved server-side.

## SQLite Store

The example stores checkout/payment state in SQLite:

```text
id
merchant_reference
customer_name
amount
description
webirr_payment_code
webirr_payment_status
webirr_payment_reference
webirr_paid_via
created_at
updated_at
paid_at
reversed_at
```

`merchant_reference` is the merchant-owned durable key. Platform-specific data
such as cart items, booking details, course IDs, shipping addresses, or tax rows
should stay in the merchant application's own tables.

## Status Values

Use the WeBirr status model:

```text
0 pending/not paid
1 paid-unconfirmed/in progress
2 paid
3 reversed/canceled
```

## Validate

From the example directory:

```bash
go test ./...
```

If Go is not installed locally, run from the repository root:

```bash
docker run --rm -v "$PWD":/src -w /src/examples/checkout-web-app golang:1.22 \
  sh -lc "GOTOOLCHAIN=local /usr/local/go/bin/go test ./..."
```
