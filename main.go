package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

type HealthCheckRes struct {
	Failing         bool `json:"failing"`
	MinResponseTime int  `json:"minResponseTime"`
}

type PaymentRequest struct {
	CorrelationId string  `json:"correlationId"`
	RequestedAt   string  `json:"requestedAt"`
	Amount        float64 `json:"amount"`
}

type ErrorResponse struct {
	Message string `json:"message"`
}

type PaymentsSummary struct {
	TotalRequests int     `json:"totalRequests"`
	TotalAmount   float64 `json:"totalAmount"`
}

type PaymentsSummaryResponse struct {
	Default  PaymentsSummary `json:"default"`
	Fallback PaymentsSummary `json:"fallback"`
}

type PaymentsLedger struct {
	Payments []PaymentLedger `json:"payments"`
}

type PaymentLedger struct {
	IsDefault      bool           `json:"isDefault"`
	PaymentRequest PaymentRequest `json:"paymentRequest"`
}

var PAYMENTS_SUMMARY = PaymentsLedger{}

func isRetryableError(statusCode int) bool {
	return statusCode/100 == 5
}

type PaymentProcessorClient struct {
	client      *http.Client
	defaultURL  string
	fallbackURL string
	defaultUp   bool
}

func main() {
	client := initPaymentProcessorClient()

	http.HandleFunc("/payments", paymentHandler(client))
	http.HandleFunc("/payments-summary", paymentsSummaryHandler())
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		healthCheckRes, err := client.HealthCheck()
		if err != nil {
			http.Error(w, "health check failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if healthCheckRes.Failing {
			http.Error(w, "health check failed payment processor down", http.StatusServiceUnavailable)
			return
		}
		fmt.Fprintf(w, "health check OK: %+v", healthCheckRes)
	})

	log.Fatal(http.ListenAndServe(":9999", nil))
}

func initPaymentProcessorClient() *PaymentProcessorClient {
	return &PaymentProcessorClient{
		client:      &http.Client{},
		defaultURL:  os.Getenv("PROCESSOR_DEFAULT_URL"),
		fallbackURL: os.Getenv("PROCESSOR_FALLBACK_URL"),
		defaultUp:   true,
	}
}

func (c *PaymentProcessorClient) baseURL() string {
	if c.defaultUp {
		return c.defaultURL
	}
	return c.fallbackURL
}

func (c *PaymentProcessorClient) HealthCheck() (*HealthCheckRes, error) {
	resp, err := c.client.Get(c.baseURL() + "/payments/service-health")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	healthCheckRes := HealthCheckRes{}
	if err := json.NewDecoder(resp.Body).Decode(&healthCheckRes); err != nil {
		return nil, err
	}
	return &healthCheckRes, nil
}

func (c *PaymentProcessorClient) ProcessPayment(req *PaymentRequest) (*http.Response, error) {
	tries := 0
	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	res := &http.Response{}
	for tries <= 3 { // 2 tries to default + 2 tries do fallback
		res, err = c.client.Post(c.baseURL()+"/payments", "application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			return nil, err
		}
		defer res.Body.Close()

		if res.StatusCode == http.StatusOK {
			PAYMENTS_SUMMARY.Payments = append(PAYMENTS_SUMMARY.Payments, PaymentLedger{
				IsDefault:      true,
				PaymentRequest: *req,
			})
			return res, nil
		}

		if isRetryableError(res.StatusCode) && tries == 1 {
			c.defaultUp = false
		}
		tries++
	}

	er := ErrorResponse{}
	if err := json.NewDecoder(res.Body).Decode(&er); err != nil {
		fmt.Println("failed to decode error response:", err)
		return nil, err
	}
	err = fmt.Errorf("%s", er.Message)
	return nil, err

}

func paymentHandler(c *PaymentProcessorClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		defer r.Body.Close()

		p := PaymentRequest{}
		err := json.NewDecoder(r.Body).Decode(&p)
		if err != nil {
			http.Error(w, "Invalid request payload: "+err.Error(), http.StatusBadRequest)
			return
		}
		p.RequestedAt = time.Now().Format(time.RFC3339)

		fmt.Printf("processing:%s\n", p.CorrelationId)
		_, err = c.ProcessPayment(&p)
		if err != nil {
			fmt.Println("payment processing error:", err)
			http.Error(w, "failed to process payment", http.StatusInternalServerError)
			return
		}

		fmt.Fprintf(w, "payment request received: %+v", p.RequestedAt)
	}
}

func paymentsSummaryHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		q := r.URL.Query()

		from := parseRequestedAt(q.Get("from"))
		to := parseRequestedAt(q.Get("to"))

		res := PaymentsSummaryResponse{}

		for _, payment := range PAYMENTS_SUMMARY.Payments {
			requestedAt := parseRequestedAt(payment.PaymentRequest.RequestedAt)
			if requestedAt.Before(from) || requestedAt.After(to) {
				continue
			}

			if payment.IsDefault {
				res.Default.TotalRequests++
				res.Default.TotalAmount += payment.PaymentRequest.Amount
				continue
			}

			res.Fallback.TotalRequests++
			res.Fallback.TotalAmount += payment.PaymentRequest.Amount
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
