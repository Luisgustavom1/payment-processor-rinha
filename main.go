package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
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

type PaymentProcessorClient struct {
	client  *http.Client
	baseURL string
}

func main() {
	client := initPaymentProcessorClient()

	http.HandleFunc("/payments", paymentHandler(client))

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

	println(string(jsonData))
	resp, err := c.client.Post(c.baseURL+"/payments", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	fmt.Println(string(body))

	return resp, nil
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

		resp, err := c.ProcessPayment(&p)
		if err != nil {
			http.Error(w, "Failed to process payment", http.StatusInternalServerError)
			return
		}
		defer resp.Body.Close()

		fmt.Fprintf(w, "Payment request received: %+v", p.RequestedAt)
	}
}
