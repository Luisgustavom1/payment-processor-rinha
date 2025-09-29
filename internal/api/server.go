package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/hibiken/asynq"
	paymentProcessor "github.com/payment-processor-rinha/internal/application/payment/processors"
	paymentTask "github.com/payment-processor-rinha/internal/application/payment/tasks"
)

func Setup(pp *paymentProcessor.PaymentProcessor, asynqClient *asynq.Client) *http.Server {
	http.HandleFunc("/payments", paymentHandler(asynqClient))
	http.HandleFunc("/payments-summary", paymentsSummaryHandler(pp))

	fmt.Println("starting server running on port 9999")
	return &http.Server{
		Addr:    ":9999",
		Handler: nil,
	}
}

func paymentHandler(asynqClient *asynq.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		defer r.Body.Close()

		input := paymentTask.ProcessPaymentTask{}
		err := json.NewDecoder(r.Body).Decode(&input)
		if err != nil {
			http.Error(w, "Invalid request payload: "+err.Error(), http.StatusBadRequest)
			return
		}

		task, err := paymentTask.NewProcessPaymentTask(input)
		if err != nil {
			http.Error(w, "failed to create payment task: "+err.Error(), http.StatusInternalServerError)
			return
		}

		_, err = asynqClient.Enqueue(task)
		if err != nil {
			fmt.Println(err)
			http.Error(w, "failed to process payment", http.StatusInternalServerError)
			return
		}

		res := map[string]interface{}{
			"message": "payment scheduled to process",
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
