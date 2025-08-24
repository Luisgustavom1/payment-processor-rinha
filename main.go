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

type PaymentSummary struct {
	TotalRequests int     `json:"totalRequests"`
	TotalAmount   float64 `json:"totalAmount"`
}

type AllPaymentsSummary struct {
	Default  PaymentSummary `json:"default"`
	Fallback PaymentSummary `json:"fallback"`
}

var PAYMENTS_SUMMARY = AllPaymentsSummary{}

type PaymentProcessorClient struct {
	client  *http.Client
	baseURL string
}

func main() {
	client := initPaymentProcessorClient()

	http.HandleFunc("/payments", paymentHandler(client))
	http.HandleFunc("/payments-summary", paymentsSummaryHandler())

	healthCheckRes, err := client.HealthCheck()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(healthCheckRes)

	log.Fatal(http.ListenAndServe(":9999", nil))
}

func initPaymentProcessorClient() *PaymentProcessorClient {
	return &PaymentProcessorClient{
		client:  &http.Client{},
		baseURL: os.Getenv("PROCESSOR_DEFAULT_URL"),
	}
}

func (c *PaymentProcessorClient) HealthCheck() (*HealthCheckRes, error) {
	resp, err := c.client.Get(c.baseURL + "/payments/service-health")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var healthCheckRes HealthCheckRes
	if err := json.NewDecoder(resp.Body).Decode(&healthCheckRes); err != nil {
		return nil, err
	}
	return &healthCheckRes, nil
}

func (c *PaymentProcessorClient) ProcessPayment(req *PaymentRequest) (*http.Response, error) {
	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	res, err := c.client.Post(c.baseURL+"/payments", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusOK {
		PAYMENTS_SUMMARY.Default.TotalRequests++
		PAYMENTS_SUMMARY.Default.TotalAmount += req.Amount
		return res, nil
	}

	var er ErrorResponse
	if err := json.NewDecoder(res.Body).Decode(&er); err != nil {
		fmt.Println("Failed to decode error response:", err)
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

		var p PaymentRequest
		err := json.NewDecoder(r.Body).Decode(&p)
		if err != nil {
			http.Error(w, "Invalid request payload: "+err.Error(), http.StatusBadRequest)
			return
		}
		p.RequestedAt = time.Now().Format(time.RFC3339)

		_, err = c.ProcessPayment(&p)
		if err != nil {
			fmt.Println("Payment processing error:", err)
			http.Error(w, "Failed to process payment", http.StatusInternalServerError)
			return
		}

		fmt.Fprintf(w, "Payment request received: %+v", p.RequestedAt)
	}
}

func paymentsSummaryHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}

		json.NewEncoder(w).Encode(PAYMENTS_SUMMARY)
	}
}
