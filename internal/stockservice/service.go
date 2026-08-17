package stockservice

import (
	"errors"
	"fmt"

	"stockticker/internal/alphavantage"
)

// DailyPointSource supplies daily closing prices for a symbol.
type DailyPointSource interface {
	GetDailyCloses(symbol string, limit int) ([]alphavantage.DailyPoint, error)
}

// Service computes statistics over daily closing prices.
type Service struct {
	source DailyPointSource
}

// New creates a Service backed by the given DailyPointSource.
func New(source DailyPointSource) *Service {
	return &Service{source: source}
}

// Result holds the daily points used and their average closing price.
type Result struct {
	Points  []alphavantage.DailyPoint
	Average float64
}

// Compute fetches up to days daily closes for symbol and returns them
// along with their average closing price.
func (s *Service) Compute(symbol string, days int) (*Result, error) {
	if days <= 0 {
		return nil, fmt.Errorf("days must be positive, got %d", days)
	}

	points, err := s.source.GetDailyCloses(symbol, days)
	if err != nil {
		return nil, fmt.Errorf("get daily closes for %s: %w", symbol, err)
	}
	if len(points) == 0 {
		return nil, errors.New("no data returned")
	}

	var sum float64
	for _, p := range points {
		sum += p.Close
	}

	return &Result{
		Points:  points,
		Average: sum / float64(len(points)),
	}, nil
}
