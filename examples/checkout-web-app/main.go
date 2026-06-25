package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	webirr "github.com/webirr/webirr-api-go-client"
	checkout "github.com/webirr/webirr-checkout-kit-go"
)

func main() {
	mode, gateway, err := createGateway()
	if err != nil {
		log.Fatal(err)
	}

	baseDir, err := exampleDir()
	if err != nil {
		log.Fatal(err)
	}

	store, err := OpenSQLiteStore(sqlitePath())
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()

	pages, err := parseTemplates(baseDir)
	if err != nil {
		log.Fatal(err)
	}

	handler := checkout.NewHandler(gateway, store)
	mux := http.NewServeMux()
	handler.Register(mux, "/webirr/checkout")
	mux.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir(filepath.Join(baseDir, "assets")))))
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(filepath.Join(baseDir, "static")))))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		renderCatalog(w, pages, catalogPage{CustomerName: "Elias", Books: demoCatalog()})
	})
	mux.HandleFunc("/demo/orders", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		customerName := strings.TrimSpace(r.Form.Get("customerName"))
		book, ok := findBook(strings.TrimSpace(r.Form.Get("bookID")))
		if customerName == "" || !ok {
			message := "Choose a book and enter customer name."
			if customerName == "" {
				message = "Customer name is required."
			}
			renderCatalog(w, pages, catalogPage{CustomerName: firstNonEmpty(customerName, "Elias"), Books: demoCatalog(), Error: message})
			return
		}
		order := orderFromBook(book, customerName, newMerchantReference())
		if err := store.UpsertOrder(order); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, order.OrderURL(), http.StatusSeeOther)
	})
	mux.HandleFunc("/orders/", func(w http.ResponseWriter, r *http.Request) {
		merchantReference := strings.TrimPrefix(r.URL.Path, "/orders/")
		switch {
		case strings.HasSuffix(merchantReference, "/success"):
			merchantReference = strings.TrimSuffix(merchantReference, "/success")
			receipt, err := store.LoadReceipt(r.Context(), merchantReference)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			render(w, pages, "success.html", receipt)
		case strings.HasSuffix(merchantReference, "/receipt.txt"):
			merchantReference = strings.TrimSuffix(merchantReference, "/receipt.txt")
			receipt, err := store.LoadReceipt(r.Context(), merchantReference)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			serveReceipt(w, receipt)
		default:
			order, err := store.LoadOrder(r.Context(), merchantReference)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			render(w, pages, "order.html", order)
		}
	})
	mux.HandleFunc("/checkout", func(w http.ResponseWriter, r *http.Request) {
		order, err := store.LoadOrder(r.Context(), r.URL.Query().Get("merchantReference"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		render(w, pages, "checkout.html", order)
	})

	log.Printf("SQLite store: %s", sqlitePath())
	log.Printf("listening on http://localhost:8080 (%s mode)", mode)
	log.Fatal(http.ListenAndServe(":8080", mux))
}

func createGateway() (string, checkout.GatewayClient, error) {
	merchantID := strings.TrimSpace(os.Getenv("WEBIRR_MERCHANT_ID"))
	apiKey := strings.TrimSpace(os.Getenv("WEBIRR_API_KEY"))
	if merchantID == "" && apiKey == "" {
		return "mock", newMockGateway(), nil
	}
	if merchantID == "" || apiKey == "" {
		return "", nil, errors.New("real WeBirr gateway mode requires WEBIRR_MERCHANT_ID and WEBIRR_API_KEY")
	}

	isTestEnv, err := envBoolDefault("WEBIRR_TEST_MODE", true)
	if err != nil {
		return "", nil, err
	}
	mode := "testenv"
	if !isTestEnv {
		mode = "prod"
	}
	return mode, webirr.NewClient(merchantID, apiKey, isTestEnv), nil
}

func envBoolDefault(name string, defaultValue bool) (bool, error) {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	if value == "" {
		return defaultValue, nil
	}
	switch value {
	case "1", "true", "yes", "y", "on":
		return true, nil
	case "0", "false", "no", "n", "off":
		return false, nil
	default:
		return false, errors.New(name + " must be true or false")
	}
}

func sqlitePath() string {
	return "webirr-checkout-demo.sqlite3"
}

func exampleDir() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("could not resolve example directory")
	}
	return filepath.Dir(file), nil
}

func parseTemplates(baseDir string) (*template.Template, error) {
	return template.ParseFiles(
		filepath.Join(baseDir, "templates", "order.html"),
		filepath.Join(baseDir, "templates", "checkout.html"),
		filepath.Join(baseDir, "templates", "success.html"),
		filepath.Join(baseDir, "templates", "catalog.html"),
	)
}

func render(w http.ResponseWriter, pages *template.Template, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func renderCatalog(w http.ResponseWriter, pages *template.Template, page catalogPage) {
	render(w, pages, "catalog.html", page)
}

func newMerchantReference() string {
	var bytes [4]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "ord_" + time.Now().UTC().Format("20060102150405")
	}
	return "ord_" + hex.EncodeToString(bytes[:])
}

func serveReceipt(w http.ResponseWriter, receipt receiptData) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+receipt.MerchantReference+`-receipt.txt"`)
	_, _ = io.WriteString(w, receiptText(receipt))
}

func receiptText(receipt receiptData) string {
	return strings.Join([]string{
		"WeBirr Online Checkout Demo",
		"----------------------------",
		"Digital Audio Book Purchase Receipt",
		"",
		"Customer Name: " + receipt.CustomerName,
		"Audio Book Title: " + receipt.ItemTitle,
		"Amount: " + receipt.Amount + " " + receipt.Currency,
		"Merchant Reference: " + receipt.MerchantReference,
		"WeBirr Payment Code: " + receipt.PaymentCode,
		"Payment Reference: " + receipt.PaymentReference,
		"Paid Via: " + receipt.PaidVia,
		"Paid At: " + receipt.PaidAt,
		"Demo Download Access: " + receipt.ItemTitle,
		"",
	}, "\n")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
