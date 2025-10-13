package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	json "github.com/json-iterator/go"
	paymentProcessor "github.com/payment-processor-rinha/internal/application/payment/processors"
)

func Setup(pp *paymentProcessor.PaymentProcessor, queue chan []byte) *http.Server {
	http.HandleFunc("/payments", paymentHandler(queue))
	http.HandleFunc("/payments-summary", paymentsSummaryHandler(pp))

	fmt.Println("starting server running on port 9999")
	return &http.Server{
		Addr:    ":9999",
		Handler: nil,
	}
}

func paymentHandler(queue chan []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		defer r.Body.Close()

		task, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read request body", http.StatusInternalServerError)
			return
		}
		select {
		case queue <- task:
			// fmt.Println("task enqueued")
		default:
			http.Error(w, "Queue is full", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}
}

func paymentsSummaryHandler(p *paymentProcessor.PaymentProcessor) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		q := r.URL.Query()

		from := parseRequestedAt(q.Get("from")).UTC().UnixMilli()
		to := parseRequestedAt(q.Get("to")).UTC().UnixMilli()

		res, err := p.SummaryPayments(ctx, from, to)
		if err != nil {
			http.Error(w, "failed to get payments summary", http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(res)
	}
}

func parseRequestedAt(reqAt string) time.Time {
	parsedTime, err := time.Parse(time.RFC3339, reqAt)
	if err != nil {
		fmt.Printf("invalid 'from' date format: %v\n", err)
		return time.Time{}
	}
	return parsedTime
}
