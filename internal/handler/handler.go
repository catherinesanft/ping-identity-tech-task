// Package handler exposes stockservice over HTTP.
package handler

import (
	"encoding/json"
	"net/http"

	"stockticker/internal/cache"
	"stockticker/internal/stockservice"
)

// Handler serves the average closing price for a single, fixed symbol
// and day count, checking a TTL cache before falling back to the
// underlying Service.
type Handler struct {
	service *stockservice.Service
	cache   *cache.TTLCache[*stockservice.Result]
	symbol  string
	days    int
}

// NewHandler creates a Handler that computes averages for symbol over
// the given number of days and caches the result.
func NewHandler(service *stockservice.Service, cache *cache.TTLCache[*stockservice.Result], symbol string, days int) *Handler {
	return &Handler{service: service, cache: cache, symbol: symbol, days: days}
}

type errorResponse struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if cached, ok := h.cache.Get(h.symbol); ok {
		writeJSON(w, http.StatusOK, cached)
		return
	}

	result, err := h.service.Compute(h.symbol, h.days)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.cache.Set(h.symbol, result)
	writeJSON(w, http.StatusOK, result)
}
