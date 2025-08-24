package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
)

type HealthCheckResponse struct {
	Failing         bool `json:"failing"`
	MinResponseTime int  `json:"minResponseTime"`
}

type ProcessPaymentPayload struct {
	CorrelationId string  `json:"correlationId"`
	RequestedAt   string  `json:"requestedAt"`
	Amount        float64 `json:"amount"`
}

type ProcessPaymentInput struct {
	CorrelationId string  `json:"correlationId"`
	Amount        float64 `json:"amount"`
}

type PaymentProcessed struct {
	Payment   ProcessPaymentPayload `json:"payment"`
	OnDefault bool                  `json:"onDefault"`
}

type PaymentsSummary struct {
	TotalRequests int     `json:"totalRequests"`
	TotalAmount   float64 `json:"totalAmount"`
}

type PaymentsSummaryResponse struct {
	Default  PaymentsSummary `json:"default"`
	Fallback PaymentsSummary `json:"fallback"`
}

func isRetryableError(statusCode int) bool {
	return statusCode/100 == 5
}

type PaymentProcessorClient struct {
	client      *http.Client
	cache       *redis.Client
	defaultURL  string
	fallbackURL string
	defaultUp   bool
}

func main() {
	redisC := redis.NewClient(&redis.Options{
		Addr:     "redis:6379",
		Password: "",
		DB:       0,
		Protocol: 2,
	})

	paymentProcessorC := initPaymentProcessorClient(redisC)

	http.HandleFunc("/payments", paymentHandler(paymentProcessorC))
	http.HandleFunc("/payments-summary", paymentsSummaryHandler(paymentProcessorC))
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		healthCheckRes, err := paymentProcessorC.HealthCheck()
		w.Header().Set("Content-Type", "application/json")

		if err != nil {
			http.Error(w, "health check failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if healthCheckRes.Failing {
			http.Error(w, "health check failed payment processor down", http.StatusServiceUnavailable)
			return
		}
		json.NewEncoder(w).Encode(healthCheckRes)
	})

	log.Fatal(http.ListenAndServe(":9999", nil))
}

func initPaymentProcessorClient(cache *redis.Client) *PaymentProcessorClient {
	return &PaymentProcessorClient{
		client:      &http.Client{},
		cache:       cache,
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

func (c *PaymentProcessorClient) HealthCheck() (*HealthCheckResponse, error) {
	resp, err := c.client.Get(c.baseURL() + "/payments/service-health")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	healthCheckRes := HealthCheckResponse{}
	if err := json.NewDecoder(resp.Body).Decode(&healthCheckRes); err != nil {
		return nil, err
	}
	return &healthCheckRes, nil
}

func (c *PaymentProcessorClient) getPaymentKey(correlationId *string) string {
	if correlationId == nil {
		return "payments:*"
	}
	return "payments:" + *correlationId
}

func (c *PaymentProcessorClient) getPaymentsIndexKey() string {
	return "payments:by-date"
}

func (c *PaymentProcessorClient) ProcessPayment(ctx context.Context, input *ProcessPaymentInput) (*ProcessPaymentPayload, error) {
	tries := 0
	now := time.Now()
	payload := ProcessPaymentPayload{
		RequestedAt:   now.Format(time.RFC3339),
		CorrelationId: input.CorrelationId,
		Amount:        input.Amount,
	}
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	c.defaultUp = true
	res := &http.Response{}
	for tries <= 3 { // 2 tries to default + 2 tries do fallback
		res, err = c.client.Post(c.baseURL()+"/payments", "application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			return nil, err
		}
		defer res.Body.Close()

		if res.StatusCode == http.StatusOK {
			result, err := c.savePayment(ctx, now, payload)
			if err != nil {
				return result, err
			}
			return &payload, nil
		}

		if !isRetryableError(res.StatusCode) {
			return nil, fmt.Errorf("processing error status: %s", res.Status)
		}

		if !c.defaultUp && tries == 1 {
			c.defaultUp = false
		}

		tries++
	}

	return nil, fmt.Errorf("processing error status: %s", res.Status)
}

func (c *PaymentProcessorClient) savePayment(ctx context.Context, now time.Time, payload ProcessPaymentPayload) (*ProcessPaymentPayload, error) {
	// TODO: try not marshal payload two time, here and previous when send to the api
	j, err := json.Marshal(PaymentProcessed{
		Payment:   payload,
		OnDefault: c.defaultUp,
	})
	if err != nil {
		return nil, fmt.Errorf("error on marshalling processed payment: %w", err)
	}

	k := c.getPaymentKey(&payload.CorrelationId)
	pipe := c.cache.TxPipeline()
	pipe.Set(ctx, k, j, 0)
	pipe.ZAdd(ctx, c.getPaymentsIndexKey(), redis.Z{
		Score:  float64(now.Unix()),
		Member: k,
	})
	_, err = pipe.Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("error on saving processed payments: %w", err)
	}
	return nil, nil
}

func paymentHandler(c *PaymentProcessorClient) http.HandlerFunc {
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

		input := ProcessPaymentInput{}
		err := json.NewDecoder(r.Body).Decode(&input)
		if err != nil {
			http.Error(w, "Invalid request payload: "+err.Error(), http.StatusBadRequest)
			return
		}

		payment, err := c.ProcessPayment(ctx, &input)
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

func paymentsSummaryHandler(c *PaymentProcessorClient) http.HandlerFunc {
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

		res := PaymentsSummaryResponse{}

		keys, err := c.cache.ZRangeByScore(ctx, c.getPaymentsIndexKey(), &redis.ZRangeBy{
			Min: fmt.Sprint(from),
			Max: fmt.Sprint(to),
		}).Result()
		if err != nil {
			fmt.Println(err)
			http.Error(w, "failed to get payments to summarize", http.StatusInternalServerError)
			return
		}

		if len(keys) == 0 {
			json.NewEncoder(w).Encode(res)
			return
		}

		results, err := c.cache.MGet(ctx, keys...).Result()
		if err != nil {
			fmt.Println(err)
			http.Error(w, "failed to get payments", http.StatusInternalServerError)
			return
		}

		for _, result := range results {
			if result == nil {
				continue
			}
			payment := PaymentProcessed{}
			err := json.Unmarshal([]byte(result.(string)), &payment)
			if err != nil {
				continue
			}

			if payment.OnDefault {
				res.Default.TotalRequests++
				res.Default.TotalAmount += payment.Payment.Amount
				continue
			}

			res.Fallback.TotalRequests++
			res.Fallback.TotalAmount += payment.Payment.Amount
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
