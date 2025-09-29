package payment

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	models "github.com/payment-processor-rinha/internal/application/payment/models"
	tasks "github.com/payment-processor-rinha/internal/application/payment/tasks"
	"github.com/redis/go-redis/v9"
)

type HealthCheckResponse struct {
	Failing         bool `json:"failing"`
	MinResponseTime int  `json:"minResponseTime"`
}

type PaymentProcessor struct {
	client      *http.Client
	cache       *redis.Client
	defaultURL  string
	fallbackURL string
	defaultUp   bool
}

func NewPaymentProcessor(cache *redis.Client) *PaymentProcessor {
	return &PaymentProcessor{
		client:      &http.Client{},
		cache:       cache,
		defaultURL:  os.Getenv("PROCESSOR_DEFAULT_URL"),
		fallbackURL: os.Getenv("PROCESSOR_FALLBACK_URL"),
		defaultUp:   true,
	}
}

func (p *PaymentProcessor) ProcessTask(ctx context.Context, task tasks.ProcessPaymentTask) error {
	log.Printf("processing payment cid %s\n", task.CorrelationId)

	now := time.Now()
	task.RequestedAt = now.Format(time.RFC3339)

	jsonData, err := json.Marshal(task)
	if err != nil {
		fmt.Println("failed to marshal payment:", err)
		return err
	}

	p.defaultUp = true
	res := &http.Response{}
	res, err = p.client.Post(p.baseURL()+"/payments", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Println("failed to send request:", err)
		return err
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusOK {
		err := p.savePayment(ctx, now, &task)
		if err != nil {
			fmt.Println("failed to save payment:", err)
			return err
		}
		fmt.Printf("payment saved %s\n", task.CorrelationId)
		return nil
	}

	if p.isRetryableError(res.StatusCode) {
		err = fmt.Errorf("processing error status: %s", res.Status)
		fmt.Println(err)
		return err
	}

	e := res.StatusCode != http.StatusOK
	if e {
		p.defaultUp = false

		res, err = p.client.Post(p.baseURL()+"/payments", "application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			fmt.Println("failed to send request:", err)
			return err
		}
		defer res.Body.Close()

		if res.StatusCode == http.StatusOK {
			err := p.savePayment(ctx, now, &task)
			if err != nil {
				fmt.Println("failed to save payment:", err)
				return err
			}
			fmt.Printf("payment saved %s\n", task.CorrelationId)
			return nil
		}

		if p.isRetryableError(res.StatusCode) {
			err = fmt.Errorf("processing error status: %s", res.Status)
			fmt.Println(err)
			return err
		}
	}

	return nil
}

func (p *PaymentProcessor) SummaryPayments(ctx context.Context, from, to int64) (*models.PaymentsSummaryResponse, error) {
	res := models.PaymentsSummaryResponse{}

	keys, err := p.cache.ZRangeByScore(ctx, p.getPaymentsIndexKey(), &redis.ZRangeBy{
		Min: fmt.Sprint(from),
		Max: fmt.Sprint(to),
	}).Result()
	if err != nil {
		fmt.Println(err)
		return nil, fmt.Errorf("failed to get payments to summarize")
	}

	fmt.Println("found payment keys len:", len(keys))
	if len(keys) == 0 {
		return &res, nil
	}

	results, err := p.cache.MGet(ctx, keys...).Result()
	if err != nil {
		fmt.Println(err)
		return nil, fmt.Errorf("failed to get payments")
	}

	for _, result := range results {
		if result == nil {
			continue
		}
		payment := tasks.ProcessPaymentTask{}
		err := json.Unmarshal([]byte(result.(string)), &payment)
		if err != nil {
			continue
		}

		if payment.OnDefault {
			res.Default.TotalRequests++
			res.Default.TotalAmount += payment.Amount
			continue
		}

		res.Fallback.TotalRequests++
		res.Fallback.TotalAmount += payment.Amount
	}

	return &res, nil
}

func (p *PaymentProcessor) HealthCheck() (*HealthCheckResponse, error) {
	resp, err := p.client.Get(p.baseURL() + "/payments/service-health")
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

func (p *PaymentProcessor) baseURL() string {
	if p.defaultUp {
		return p.defaultURL
	}
	return p.fallbackURL
}

func (p *PaymentProcessor) getPaymentKey(correlationId *string) string {
	if correlationId == nil {
		return "payments:*"
	}
	return "payments:" + *correlationId
}

func (p *PaymentProcessor) getPaymentsIndexKey() string {
	return "payments:by-date"
}

func (p *PaymentProcessor) savePayment(ctx context.Context, now time.Time, payload *tasks.ProcessPaymentTask) error {
	payload.OnDefault = p.defaultUp
	j, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("error on marshalling processed payment: %w", err)
	}

	k := p.getPaymentKey(&payload.CorrelationId)
	pipe := p.cache.TxPipeline()
	pipe.Set(ctx, k, j, 0)
	pipe.ZAdd(ctx, p.getPaymentsIndexKey(), redis.Z{
		Score:  float64(now.Unix()),
		Member: k,
	})
	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("error on saving processed payments: %w", err)
	}
	return nil
}

func (p *PaymentProcessor) isRetryableError(statusCode int) bool {
	return statusCode/100 == 5
}
