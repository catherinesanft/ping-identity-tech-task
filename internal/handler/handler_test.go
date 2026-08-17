package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"stockticker/internal/alphavantage"
	"stockticker/internal/cache"
	"stockticker/internal/stockservice"
)

// countingSource is a fake DailyPointSource that records how many times
// GetDailyCloses was called.
type countingSource struct {
	mu     sync.Mutex
	calls  int
	points []alphavantage.DailyPoint
	err    error
}

func (s *countingSource) GetDailyCloses(symbol string, limit int) ([]alphavantage.DailyPoint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return s.points, nil
}

func (s *countingSource) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func daily(closes ...float64) []alphavantage.DailyPoint {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	points := make([]alphavantage.DailyPoint, len(closes))
	for i, c := range closes {
		points[i] = alphavantage.DailyPoint{Date: base.AddDate(0, 0, i), Close: c}
	}
	return points
}

func TestHandler_RejectsNonGET(t *testing.T) {
	source := &countingSource{points: daily(10, 20)}
	h := NewHandler(stockservice.New(source), cache.New[*stockservice.Result](time.Minute), "IBM", 5)

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
	if source.callCount() != 0 {
		t.Fatalf("source called %d times, want 0", source.callCount())
	}
}

func TestHandler_PropagatesSourceError(t *testing.T) {
	source := &countingSource{err: errSource}
	h := NewHandler(stockservice.New(source), cache.New[*stockservice.Result](time.Minute), "IBM", 5)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestHandler_SecondRequestWithinTTLHitsCache(t *testing.T) {
	source := &countingSource{points: daily(10, 20, 30)}
	h := NewHandler(stockservice.New(source), cache.New[*stockservice.Result](time.Minute), "IBM", 3)

	// First request: cache miss, hits the source.
	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	rec1 := httptest.NewRecorder()
	h.ServeHTTP(rec1, req1)

	if rec1.Code != http.StatusOK {
		t.Fatalf("first request status = %d, want %d", rec1.Code, http.StatusOK)
	}
	if got := source.callCount(); got != 1 {
		t.Fatalf("after first request, source called %d times, want 1", got)
	}

	var first stockservice.Result
	if err := json.NewDecoder(rec1.Body).Decode(&first); err != nil {
		t.Fatalf("decode first response: %v", err)
	}

	// Second request within the TTL window: should be served from cache,
	// not call the source again.
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Fatalf("second request status = %d, want %d", rec2.Code, http.StatusOK)
	}
	if got := source.callCount(); got != 1 {
		t.Fatalf("after second request, source called %d times, want still 1 (expected cache hit)", got)
	}

	var second stockservice.Result
	if err := json.NewDecoder(rec2.Body).Decode(&second); err != nil {
		t.Fatalf("decode second response: %v", err)
	}
	if second.Average != first.Average {
		t.Fatalf("second response Average = %v, want %v (same as cached result)", second.Average, first.Average)
	}
}

func TestHandler_ExpiredCacheCallsSourceAgain(t *testing.T) {
	source := &countingSource{points: daily(10, 20, 30)}
	h := NewHandler(stockservice.New(source), cache.New[*stockservice.Result](20*time.Millisecond), "IBM", 3)

	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(httptest.NewRecorder(), req1)

	time.Sleep(40 * time.Millisecond)

	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(httptest.NewRecorder(), req2)

	if got := source.callCount(); got != 2 {
		t.Fatalf("after TTL expiry, source called %d times, want 2", got)
	}
}

var errSource = &staticErr{"source unavailable"}

type staticErr struct{ msg string }

func (e *staticErr) Error() string { return e.msg }
