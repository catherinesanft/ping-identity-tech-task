// Command server runs the stockticker HTTP server.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"stockticker/internal/alphavantage"
	"stockticker/internal/cache"
	"stockticker/internal/handler"
	"stockticker/internal/stockservice"
)

const defaultAPIKey = "C227WD9W3LUVKVV9" // sample key from the exercise spec

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	symbol := os.Getenv("SYMBOL")
	if symbol == "" {
		return errors.New("SYMBOL environment variable is required")
	}

	ndaysStr := os.Getenv("NDAYS")
	if ndaysStr == "" {
		return errors.New("NDAYS environment variable is required")
	}
	days, err := strconv.Atoi(ndaysStr)
	if err != nil || days <= 0 {
		return fmt.Errorf("NDAYS must be a positive integer, got %q", ndaysStr)
	}

	apiKey := os.Getenv("APIKEY")
	if apiKey == "" {
		apiKey = defaultAPIKey
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	cacheTTLStr := os.Getenv("CACHE_TTL")
	if cacheTTLStr == "" {
		cacheTTLStr = "5m"
	}
	cacheTTL, err := time.ParseDuration(cacheTTLStr)
	if err != nil {
		return fmt.Errorf("CACHE_TTL must be a valid duration, got %q: %w", cacheTTLStr, err)
	}

	client := alphavantage.NewClient(apiKey)
	svc := stockservice.New(client)
	resultCache := cache.New[*stockservice.Result](cacheTTL)
	h := handler.NewHandler(svc, resultCache, symbol, days)

	mux := http.NewServeMux()
	mux.Handle("/", h)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		log.Printf("listening on :%s (symbol=%s days=%d cache_ttl=%s)", port, symbol, days, cacheTTL)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
		close(serverErr)
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		if err != nil {
			return fmt.Errorf("server error: %w", err)
		}
	case sig := <-stop:
		log.Printf("received %s, shutting down", sig)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := srv.Shutdown(ctx); err != nil {
			return fmt.Errorf("graceful shutdown failed: %w", err)
		}
		log.Println("shutdown complete")
	}

	return nil
}
