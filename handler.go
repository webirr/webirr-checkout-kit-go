package checkout

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

// Handler exposes merchant-owned checkout HTTP endpoints.
type Handler struct {
	checkout *Checkout
}

// NewHandler creates HTTP handlers with default gateway status polling.
func NewHandler(gateway GatewayClient, store Store, options ...Option) *Handler {
	return &Handler{checkout: New(gateway, store, options...)}
}

// Checkout returns the underlying checkout service.
func (h *Handler) Checkout() *Checkout {
	return h.checkout
}

// Register mounts create and status endpoints under a base path.
func (h *Handler) Register(mux *http.ServeMux, basePath string) {
	basePath = "/" + strings.Trim(strings.TrimSpace(basePath), "/")
	mux.HandleFunc(basePath, h.CreateCheckout)
	mux.HandleFunc(basePath+"/status", h.GetStatus)
}

// CreateCheckout handles POST /webirr/checkout with body {"merchantReference":"..."}.
func (h *Handler) CreateCheckout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body struct {
		MerchantReference string `json:"merchantReference"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	result, err := h.checkout.CreateCheckout(r.Context(), body.MerchantReference)
	if err != nil {
		writeCheckoutError(w, err)
		return
	}
	jsonResponse(w, http.StatusOK, result)
}

// GetStatus handles GET /webirr/checkout/status?merchantReference=....
func (h *Handler) GetStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	result, err := h.checkout.GetStatus(r.Context(), r.URL.Query().Get("merchantReference"))
	if err != nil {
		writeCheckoutError(w, err)
		return
	}
	jsonResponse(w, http.StatusOK, result)
}

func writeCheckoutError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrMerchantReferenceRequired):
		jsonError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrUnsupportedLocalStatus):
		jsonError(w, http.StatusInternalServerError, err.Error())
	default:
		jsonError(w, http.StatusInternalServerError, err.Error())
	}
}

func jsonResponse(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func jsonError(w http.ResponseWriter, status int, message string) {
	jsonResponse(w, status, map[string]string{"error": message})
}
