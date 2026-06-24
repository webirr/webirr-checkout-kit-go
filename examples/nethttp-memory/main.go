package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	webirr "github.com/webirr/webirr-api-go-client"
	checkout "github.com/webirr/webirr-checkout-kit-go"
)

func main() {
	merchantID := os.Getenv("WEBIRR_TEST_ENV_MERCHANT_ID")
	apiKey := os.Getenv("WEBIRR_TEST_ENV_API_KEY")
	mode := "mock"

	var gateway checkout.GatewayClient
	if merchantID != "" && apiKey != "" {
		mode = "testenv"
		gateway = webirr.NewClient(merchantID, apiKey, true)
	} else {
		gateway = newMockGateway()
	}

	merchantReference := os.Getenv("WEBIRR_DEMO_MERCHANT_REFERENCE")
	if merchantReference == "" {
		merchantReference = "ord_" + time.Now().Format("2006_01_02_150405")
	}

	store := checkout.NewMemoryStore()
	store.PutPayable(checkout.Payable{
		MerchantReference: merchantReference,
		Amount:            "640.00",
		Currency:          "ETB",
		CustomerName:      "Elias",
		CustomerCode:      "CUST-1001",
		CustomerPhone:     "0911000000",
		Description:       "Sample Audio Book",
		SuccessURL:        "/success",
		CancelURL:         "/cart",
	})

	handler := checkout.NewHandler(gateway, store)
	mux := http.NewServeMux()
	handler.Register(mux, "/webirr/checkout")
	mux.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir("examples/nethttp-memory/assets"))))
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(strings.ReplaceAll(page, "{{merchantReference}}", merchantReference)))
	})

	log.Printf("listening on http://localhost:8080 (%s mode)", mode)
	log.Fatal(http.ListenAndServe(":8080", mux))
}

type mockGateway struct {
	mu          sync.Mutex
	statusCalls map[string]int
}

func newMockGateway() *mockGateway {
	return &mockGateway{statusCalls: make(map[string]int)}
}

func (g *mockGateway) CreateBill(_ context.Context, bill *webirr.Bill) (*webirr.ApiResponse[string], error) {
	return &webirr.ApiResponse[string]{Res: "806214"}, nil
}

func (g *mockGateway) UpdateBill(_ context.Context, bill *webirr.Bill) (*webirr.ApiResponse[string], error) {
	return &webirr.ApiResponse[string]{Res: "OK"}, nil
}

func (g *mockGateway) GetPaymentStatus(_ context.Context, paymentCode string) (*webirr.ApiResponse[webirr.PaymentStatus], error) {
	g.mu.Lock()
	g.statusCalls[paymentCode]++
	calls := g.statusCalls[paymentCode]
	g.mu.Unlock()

	if calls < 3 {
		return &webirr.ApiResponse[webirr.PaymentStatus]{Res: webirr.PaymentStatus{Status: 1}}, nil
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

func (g *mockGateway) GetBillByReference(_ context.Context, _ string) (*webirr.ApiResponse[webirr.BillResponse], error) {
	return &webirr.ApiResponse[webirr.BillResponse]{Error: "not found"}, nil
}

func (g *mockGateway) GetBillByPaymentCode(_ context.Context, paymentCode string) (*webirr.ApiResponse[webirr.BillResponse], error) {
	return &webirr.ApiResponse[webirr.BillResponse]{
		Res: webirr.BillResponse{
			Amount:        "640.00",
			CustomerName:  "Elias",
			Description:   "Sample Audio Book",
			BillReference: "ord_2026_06_24_10033",
			WbcCode:       paymentCode,
			PaymentStatus: 1,
		},
	}, nil
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

const page = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>WeBirr Go Checkout Example</title>
  <style>
    :root {
      --ink: #192028;
      --muted: #64707d;
      --line: #dce3e8;
      --soft: #f5f8fa;
      --accent: #0f7f53;
      --accent-strong: #0b6e46;
      --blue: #1167b1;
      --warn: #fff8e5;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      background: #eef3f6;
      color: var(--ink);
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      letter-spacing: 0;
    }
    main { max-width: 980px; margin: 32px auto; padding: 0 18px; }
    .checkout-shell {
      background: #fff;
      border: 1px solid var(--line);
      border-radius: 8px;
      box-shadow: 0 12px 32px rgba(32, 45, 58, .08);
      overflow: hidden;
    }
    .header {
      display: flex;
      align-items: center;
      gap: 12px;
      padding: 22px 26px;
      border-bottom: 1px solid var(--line);
    }
    .header img { width: 42px; height: 42px; }
    h1 { margin: 0; font-size: 26px; line-height: 1.2; }
    .mode { color: var(--muted); font-size: 13px; margin-top: 3px; }
    .content { padding: 24px 26px 28px; }
    .summary {
      display: grid;
      grid-template-columns: repeat(2, minmax(0, 1fr));
      gap: 0;
      border: 1px solid var(--line);
      border-radius: 6px;
      overflow: hidden;
      margin-bottom: 22px;
    }
    .summary div { padding: 13px 16px; border-bottom: 1px solid var(--line); }
    .summary div:nth-child(odd) { color: var(--muted); background: #fbfcfd; font-weight: 600; }
    .summary div:nth-last-child(-n + 2) { border-bottom: 0; }
    .actions { display: flex; gap: 12px; align-items: center; }
    button, a.button {
      min-width: 112px;
      min-height: 42px;
      border: 0;
      border-radius: 4px;
      padding: 10px 16px;
      font-size: 15px;
      cursor: pointer;
      text-align: center;
      text-decoration: none;
    }
    button.primary { background: var(--blue); color: #fff; }
    a.button { background: #edf2f6; color: var(--ink); }
    button:disabled { opacity: .55; cursor: default; }
    .status-line {
      display: flex;
      align-items: center;
      gap: 10px;
      background: var(--warn);
      border: 1px solid #f0dfaa;
      color: #67530c;
      padding: 12px 14px;
      border-radius: 6px;
      margin-bottom: 18px;
    }
    .spinner {
      width: 18px;
      height: 18px;
      border: 3px solid #dfc766;
      border-top-color: transparent;
      border-radius: 50%;
      animation: spin .9s linear infinite;
    }
    @keyframes spin { to { transform: rotate(360deg); } }
    .payment-code-title { color: var(--muted); font-weight: 700; margin-bottom: 8px; }
    .payment-code {
      color: var(--blue);
      font-size: 56px;
      line-height: 1;
      font-weight: 800;
      letter-spacing: 0;
      margin-bottom: 18px;
    }
    .record {
      display: grid;
      grid-template-columns: minmax(160px, 1fr) minmax(160px, 1.4fr);
      border: 1px solid var(--line);
      border-radius: 6px;
      overflow: hidden;
      margin: 0 0 20px;
    }
    .record dt, .record dd {
      padding: 13px 16px;
      border-bottom: 1px solid var(--line);
      margin: 0;
    }
    .record dt { color: var(--muted); font-weight: 700; background: #fbfcfd; }
    .record dt:last-of-type, .record dd:last-of-type { border-bottom: 0; }
    .instructions h2, .confirmed h2 { font-size: 20px; margin: 0 0 12px; }
    .instructions ul { margin: 0; padding: 0; list-style: none; border: 1px solid var(--line); border-radius: 6px; overflow: hidden; }
    .instructions li { padding: 11px 14px; border-bottom: 1px solid var(--line); }
    .instructions li:last-child { border-bottom: 0; }
    .confirmed {
      border: 1px solid #cde7d8;
      background: #f1fbf5;
      border-radius: 7px;
      padding: 22px;
      max-width: 680px;
    }
    .check {
      display: inline-flex;
      width: 42px;
      height: 42px;
      align-items: center;
      justify-content: center;
      border-radius: 50%;
      background: var(--accent);
      color: white;
      font-weight: 800;
      margin-bottom: 14px;
    }
    .hidden { display: none; }
    @media (max-width: 640px) {
      main { margin: 0; padding: 0; }
      .checkout-shell { border-radius: 0; }
      .summary, .record { grid-template-columns: 1fr; }
      .summary div:nth-last-child(-n + 2), .record dt:last-of-type { border-bottom: 1px solid var(--line); }
      .payment-code { font-size: 44px; }
    }
  </style>
</head>
<body>
  <main>
    <section class="checkout-shell">
      <header class="header">
        <img src="/assets/webirr-icon.png" alt="">
        <div>
          <h1>WeBirr Online Checkout</h1>
          <div class="mode">Go checkout kit example</div>
        </div>
      </header>
      <div class="content">
        <section id="review">
          <div class="summary">
            <div>Customer</div><div>Elias</div>
            <div>Amount</div><div>ETB 640.00</div>
            <div>Description</div><div>Sample Audio Book</div>
            <div>Merchant reference</div><div>{{merchantReference}}</div>
          </div>
          <div class="actions">
            <button class="primary" id="checkout">Checkout</button>
            <a class="button" href="/">Cancel</a>
          </div>
        </section>

        <section id="waiting" class="hidden">
          <div class="payment-code-title">WeBirr Payment Code</div>
          <div class="payment-code" id="payment-code"></div>
          <div class="status-line"><span class="spinner"></span><span>Payment not received yet.</span></div>
          <dl class="record">
            <dt>Customer</dt><dd id="waiting-customer"></dd>
            <dt>Amount</dt><dd id="waiting-amount"></dd>
            <dt>Merchant reference</dt><dd id="waiting-reference"></dd>
            <dt>Payment Status</dt><dd>pending</dd>
          </dl>
          <div class="instructions">
            <h2>Payment Instruction</h2>
            <ul id="instructions"></ul>
          </div>
        </section>

        <section id="paid" class="hidden">
          <div class="confirmed">
            <div class="check">OK</div>
            <h2>Payment Confirmed</h2>
            <dl class="record">
              <dt>Customer</dt><dd id="paid-customer"></dd>
              <dt>Amount</dt><dd id="paid-amount"></dd>
              <dt>Payment Reference</dt><dd id="paid-reference"></dd>
              <dt>Paid Via</dt><dd id="paid-via"></dd>
            </dl>
          </div>
        </section>
      </div>
    </section>
  </main>
  <script>
    const merchantReference = "{{merchantReference}}";
    const review = document.getElementById("review");
    const waiting = document.getElementById("waiting");
    const paid = document.getElementById("paid");
    let checkoutData = null;
    let pollTimer = null;
    async function requestJSON(url, options) {
      const response = await fetch(url, options);
      const data = await response.json();
      if (!response.ok || data.error) throw new Error(data.error || "Request failed");
      return data;
    }
    function show(section) {
      review.classList.toggle("hidden", section !== "review");
      waiting.classList.toggle("hidden", section !== "waiting");
      paid.classList.toggle("hidden", section !== "paid");
    }
    function money(amount, currency) {
      return (currency || "ETB") + " " + amount;
    }
    function renderWaiting(data) {
      document.getElementById("payment-code").textContent = data.paymentCode;
      document.getElementById("waiting-customer").textContent = data.customerName || "";
      document.getElementById("waiting-amount").textContent = money(data.amount, data.currency);
      document.getElementById("waiting-reference").textContent = data.merchantReference;
      const list = document.getElementById("instructions");
      list.innerHTML = "";
      (data.supportedBanks || []).forEach((bank) => {
        const item = document.createElement("li");
        item.textContent = bank.name + " -> WeBirr -> Payment Code";
        list.appendChild(item);
      });
      show("waiting");
    }
    function renderPaid(status) {
      document.getElementById("paid-customer").textContent = status.customerName || checkoutData.customerName || "";
      document.getElementById("paid-amount").textContent = money(status.amount || checkoutData.amount, status.currency || checkoutData.currency);
      document.getElementById("paid-reference").textContent = status.paymentReference || "";
      document.getElementById("paid-via").textContent = status.paymentIssuer || "";
      show("paid");
    }
    async function poll() {
      const status = await requestJSON("/webirr/checkout/status?merchantReference=" + encodeURIComponent(merchantReference));
      if (status.status === "Paid") {
        clearInterval(pollTimer);
        renderPaid(status);
      }
    }
    document.getElementById("checkout").addEventListener("click", async () => {
      document.getElementById("checkout").disabled = true;
      checkoutData = await requestJSON("/webirr/checkout", {
        method: "POST",
        headers: {"content-type": "application/json"},
        body: JSON.stringify({merchantReference})
      });
      renderWaiting(checkoutData);
      pollTimer = setInterval(() => { void poll(); }, 1400);
    });
  </script>
</body>
</html>`
