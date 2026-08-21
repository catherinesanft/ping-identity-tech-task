// Package alphavantage provides a client for the Alpha Vantage
// TIME_SERIES_DAILY endpoint.
package alphavantage

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"time"
)

const (
	baseURL    = "https://www.alphavantage.co/query"
	dateLayout = "2006-01-02"
)

// DailyPoint is a single day's closing price for a symbol.
//
// Date is a time.Time (parsed from Alpha Vantage's "YYYY-MM-DD" keys)
// rather than a raw string so callers can sort, compare, and format it
// without reparsing.
type DailyPoint struct {
	Date  time.Time
	Close float64
}

// Client is an Alpha Vantage API client.
type Client struct {
	APIKey     string
	HTTPClient *http.Client
}

// NewClient creates a Client with the given API key and a sane default
// HTTP timeout.
func NewClient(apiKey string) *Client {
	return &Client{
		APIKey: apiKey,
		HTTPClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// dailyResponse mirrors the relevant parts of the TIME_SERIES_DAILY
// response. Alpha Vantage returns HTTP 200 even on rate limiting or an
// invalid symbol, signaling the failure inside the JSON body instead.
type dailyResponse struct {
	ErrorMessage string                    `json:"Error Message"`
	Note         string                    `json:"Note"`
	Information  string                    `json:"Information"`
	TimeSeries   map[string]dailyPointJSON `json:"Time Series (Daily)"`
}

type dailyPointJSON struct {
	Close string `json:"4. close"`
}

// GetDailyCloses fetches daily closing prices for symbol, most recent
// first, capped at limit entries. It respects ctx cancellation and
// deadlines for the underlying HTTP call.
func (c *Client) GetDailyCloses(ctx context.Context, symbol string, limit int) ([]DailyPoint, error) {
	outputSize := "compact" // covers the most recent 100 daily points
	if limit > 100 {
		outputSize = "full"
	}

	reqURL := baseURL + "?" + url.Values{
		"function":   {"TIME_SERIES_DAILY"},
		"symbol":     {symbol},
		"outputsize": {outputSize},
		"apikey":     {c.APIKey},
	}.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("alphavantage: build request: %w", err)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("alphavantage: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("alphavantage: unexpected status %d", resp.StatusCode)
	}

	var parsed dailyResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("alphavantage: decode response: %w", err)
	}

	if parsed.ErrorMessage != "" {
		return nil, fmt.Errorf("alphavantage: %s", parsed.ErrorMessage)
	}
	if parsed.Note != "" {
		return nil, fmt.Errorf("alphavantage: %s", parsed.Note)
	}
	if parsed.Information != "" {
		return nil, fmt.Errorf("alphavantage: %s", parsed.Information)
	}
	if len(parsed.TimeSeries) == 0 {
		return nil, fmt.Errorf("alphavantage: no time series data for %s", symbol)
	}

	points := make([]DailyPoint, 0, len(parsed.TimeSeries))
	for dateStr, raw := range parsed.TimeSeries {
		date, err := time.Parse(dateLayout, dateStr)
		if err != nil {
			return nil, fmt.Errorf("alphavantage: parse date %q: %w", dateStr, err)
		}
		closePrice, err := strconv.ParseFloat(raw.Close, 64)
		if err != nil {
			return nil, fmt.Errorf("alphavantage: parse close %q: %w", raw.Close, err)
		}
		points = append(points, DailyPoint{Date: date, Close: closePrice})
	}

	sort.Slice(points, func(i, j int) bool {
		return points[i].Date.After(points[j].Date)
	})

	if limit > 0 && limit < len(points) {
		points = points[:limit]
	}

	return points, nil
}
