package stockservice

import (
	"context"
	"errors"
	"testing"
	"time"

	"stockticker/internal/alphavantage"
)

// fakeSource is an in-memory DailyPointSource for testing.
type fakeSource struct {
	points []alphavantage.DailyPoint
	err    error
}

func (f *fakeSource) GetDailyCloses(ctx context.Context, symbol string, limit int) ([]alphavantage.DailyPoint, error) {
	if f.err != nil {
		return nil, f.err
	}
	if limit < len(f.points) {
		return f.points[:limit], nil
	}
	return f.points, nil
}

func daily(closes ...float64) []alphavantage.DailyPoint {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	points := make([]alphavantage.DailyPoint, len(closes))
	for i, c := range closes {
		points[i] = alphavantage.DailyPoint{Date: base.AddDate(0, 0, i), Close: c}
	}
	return points
}

func TestCompute(t *testing.T) {
	sourceErr := errors.New("source unavailable")

	tests := []struct {
		name        string
		source      *fakeSource
		symbol      string
		days        int
		wantAverage float64
		wantLen     int
		wantErr     error // if set, require errors.Is match
		wantErrMsg  bool  // if true, just require an error
	}{
		{
			name:        "correct average calculation",
			source:      &fakeSource{points: daily(10, 20, 30)},
			symbol:      "IBM",
			days:        3,
			wantAverage: 20,
			wantLen:     3,
		},
		{
			name:        "limit smaller than available data",
			source:      &fakeSource{points: daily(10, 20, 30, 40, 50)},
			symbol:      "IBM",
			days:        2,
			wantAverage: 15,
			wantLen:     2,
		},
		{
			name:       "rejects zero days",
			source:     &fakeSource{points: daily(10, 20)},
			symbol:     "IBM",
			days:       0,
			wantErrMsg: true,
		},
		{
			name:       "rejects negative days",
			source:     &fakeSource{points: daily(10, 20)},
			symbol:     "IBM",
			days:       -5,
			wantErrMsg: true,
		},
		{
			name:    "propagates source error",
			source:  &fakeSource{err: sourceErr},
			symbol:  "IBM",
			days:    5,
			wantErr: sourceErr,
		},
		{
			name:       "no data returned",
			source:     &fakeSource{points: nil},
			symbol:     "IBM",
			days:       5,
			wantErrMsg: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := New(tt.source)
			result, err := svc.Compute(context.Background(), tt.symbol, tt.days)

			if tt.wantErr != nil {
				if err == nil || !errors.Is(err, tt.wantErr) {
					t.Fatalf("Compute() error = %v, want wrapping %v", err, tt.wantErr)
				}
				return
			}
			if tt.wantErrMsg {
				if err == nil {
					t.Fatalf("Compute() expected an error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Compute() unexpected error: %v", err)
			}
			if len(result.Points) != tt.wantLen {
				t.Errorf("Compute() len(Points) = %d, want %d", len(result.Points), tt.wantLen)
			}
			if result.Average != tt.wantAverage {
				t.Errorf("Compute() Average = %v, want %v", result.Average, tt.wantAverage)
			}
		})
	}
}
