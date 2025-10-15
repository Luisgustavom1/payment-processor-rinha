package payment

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"net/http"
	"os"
	"sync"
	"time"

	json "github.com/json-iterator/go"
	models "github.com/payment-processor-rinha/internal/application/payment/models"
	tasks "github.com/payment-processor-rinha/internal/application/payment/tasks"
	"github.com/redis/go-redis/v9"
)

type PaymentProcessor struct {
	client      *http.Client
	cache       *redis.Client
	defaultURL  string
	fallbackURL string
	up          bool
	upMutex     sync.RWMutex
}

func NewPaymentProcessor(ctx context.Context, cache *redis.Client) *PaymentProcessor {
	upCached := cache.Get(ctx, HEALTH_CHECK_KEY)
	up, _ := upCached.Bool()

	fmt.Printf("initializing up with %t\n", up)

	return &PaymentProcessor{
		client:      &http.Client{},
		cache:       cache,
		defaultURL:  os.Getenv("PROCESSOR_DEFAULT_URL"),
		fallbackURL: os.Getenv("PROCESSOR_FALLBACK_URL"),
		up:          up,
	}
}

func (p *PaymentProcessor) IsUp() bool {
	p.upMutex.RLock()
	defer p.upMutex.RUnlock()
	return p.up
}

func (p *PaymentProcessor) SetUp(status bool) {
	p.upMutex.Lock()
	defer p.upMutex.Unlock()
	p.up = status
}

func (p *PaymentProcessor) ProcessTask(ctx context.Context, task tasks.ProcessPaymentTask) error {
	// fmt.Printf("processing payment cid %s\n", task.CorrelationId\)
	now := time.Now().UTC()
	task.RequestedAt = now.Format(time.RFC3339)

	jsonData, err := json.Marshal(task)

	if err != nil {
		fmt.Println("failed to marshal payment:", err)
		return err
	}

	res := &http.Response{}
	res, err = p.client.Post(p.baseURL()+"/payments", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Println("failed to send request:", err)
		return err
	}
	defer res.Body.Close()

	if p.isRetryableError(res.StatusCode) {
		err = fmt.Errorf("processing error status: %s %s", res.Status, res.Body)
		fmt.Println(err)
		p.SetUp(false)
		return err
	}

	if res.StatusCode == http.StatusOK {
		err := p.savePayment(ctx, now, &task)
		if err != nil {
			fmt.Println("failed to save payment:", err)
			return err
		}
		return nil
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

	res.Default.TotalAmount = math.Round(res.Default.TotalAmount*10) / 10
	res.Fallback.TotalAmount = math.Round(res.Fallback.TotalAmount*10) / 10

	return &res, nil
}

func (p *PaymentProcessor) baseURL() string {
	if p.IsUp() {
		return p.defaultURL
	}
	return p.fallbackURL
}

func (p *PaymentProcessor) getPaymentKey(correlationId string) string {
	return "payments:" + correlationId
}

func (p *PaymentProcessor) getPaymentsIndexKey() string {
	return "payments:by-date"
}

func (p *PaymentProcessor) savePayment(ctx context.Context, now time.Time, payload *tasks.ProcessPaymentTask) error {
	payload.OnDefault = p.IsUp()
	j, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("error on marshalling processed payment: %w", err)
	}

	k := p.getPaymentKey(payload.CorrelationId)
	pipe := p.cache.TxPipeline()
	pipe.Set(ctx, k, j, 0)
	pipe.ZAdd(ctx, p.getPaymentsIndexKey(), redis.Z{
		Score:  float64(now.UnixMilli()),
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
