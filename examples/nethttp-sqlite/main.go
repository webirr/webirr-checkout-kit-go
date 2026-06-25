package main

import (
	"errors"
	"html/template"
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

	order := defaultOrder()
	if err := store.UpsertOrder(order); err != nil {
		log.Fatal(err)
	}

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
		http.Redirect(w, r, order.OrderURL(), http.StatusFound)
	})
	mux.HandleFunc("/orders/", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case order.OrderURL():
			render(w, pages, "order.html", order)
		case order.SuccessURL():
			render(w, pages, "success.html", order)
		default:
			http.NotFound(w, r)
		}
	})
	mux.HandleFunc("/checkout", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("merchantReference") != order.MerchantReference {
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

func defaultOrder() demoOrder {
	merchantReference := strings.TrimSpace(os.Getenv("WEBIRR_DEMO_MERCHANT_REFERENCE"))
	if merchantReference == "" {
		merchantReference = "ord_" + time.Now().Format("2006_01_02_150405")
	}
	amount := firstNonEmpty(os.Getenv("WEBIRR_DEMO_AMOUNT"), "640.00")
	description := firstNonEmpty(os.Getenv("WEBIRR_DEMO_DESCRIPTION"), "Sample Audio Book")
	customerName := firstNonEmpty(os.Getenv("WEBIRR_DEMO_CUSTOMER_NAME"), "Elias")
	return demoOrder{
		MerchantReference: merchantReference,
		Amount:            amount,
		Currency:          "ETB",
		CustomerName:      customerName,
		CustomerCode:      "CUST-1001",
		CustomerPhone:     "",
		Description:       description,
	}
}

func sqlitePath() string {
	return firstNonEmpty(os.Getenv("WEBIRR_DEMO_SQLITE_PATH"), "webirr-checkout-demo.sqlite3")
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
	)
}

func render(w http.ResponseWriter, pages *template.Template, name string, order demoOrder) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.ExecuteTemplate(w, name, order); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
