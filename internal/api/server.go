package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"

	paymentProcessor "github.com/payment-processor-rinha/internal/application/payment"
	payment "github.com/payment-processor-rinha/internal/application/payment/models"
)

func Setup(redisClient *redis.Client) error {
	paymentProcessorC := paymentProcessor.InitPaymentProcessor(redisClient)

	http.HandleFunc("/payments", paymentHandler(paymentProcessorC))
	http.HandleFunc("/payments-summary", paymentsSummaryHandler(paymentProcessorC))

	fmt.Println("starting server running on port 9999")
	return http.ListenAndServe(":9999", nil)
}

func paymentHandler(p *paymentProcessor.PaymentProcessor) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		defer r.Body.Close()

		input := payment.PaymentProcessed{}
		err := json.NewDecoder(r.Body).Decode(&input)
		if err != nil {
			http.Error(w, "Invalid request payload: "+err.Error(), http.StatusBadRequest)
			return
		}

		payment, err := p.ProcessPayment(ctx, &input)
		if err != nil {
			fmt.Println(err)
			http.Error(w, "failed to process payment", http.StatusInternalServerError)
			return
		}

		res := map[string]interface{}{
			"message":     "payment request received",
			"requestedAt": payment.RequestedAt,
		}
		json.NewEncoder(w).Encode(res)
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

		from := parseRequestedAt(q.Get("from")).Unix()
		to := parseRequestedAt(q.Get("to")).Unix()

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
