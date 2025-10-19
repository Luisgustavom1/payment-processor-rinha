package worker

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	json "github.com/json-iterator/go"
	paymentProcessor "github.com/payment-processor-rinha/internal/application/payment/processors"
	paymentTask "github.com/payment-processor-rinha/internal/application/payment/tasks"
)

type PaymentWorkerPool struct {
	pp          *paymentProcessor.PaymentProcessor
	concurrency int
	queue       chan []byte
	maxRetries  int
}

func NewPaymentWorker(pp *paymentProcessor.PaymentProcessor, queue chan []byte, concurrency int) *PaymentWorkerPool {
	return &PaymentWorkerPool{
		pp:          pp,
		concurrency: concurrency,
		queue:       queue,
		maxRetries:  5,
	}
}

var lastQueueAnalysis = time.Now()

func (wp *PaymentWorkerPool) StartPaymentWorker(queueMaxSize int) {
	for i := range wp.concurrency {
		ctx := context.Background()
		ctx.Value(i)
		go func() {
			for buff := range wp.queue {
				ql := len(wp.queue)
				if time.Since(lastQueueAnalysis) > time.Second*3 && float64(ql) >= float64(queueMaxSize)*0.9 {
					fmt.Printf("queue is almost full %d\n", ql)
					lastQueueAnalysis = time.Now()
				}

				for !wp.pp.IsUp() {
					time.Sleep(time.Millisecond * 100)
				}

				task := paymentTask.ProcessPaymentTask{}
				err := json.Unmarshal(buff, &task)
				if err != nil {
					fmt.Printf("error when unmarshal task %s\n", err.Error())
					panic(err)
				}

				tries := 0
				for {
					tries++
					if tries > wp.maxRetries {
						fmt.Printf("max retries reached for task %s\n", task.CorrelationId)
						break
					}

					if err := wp.pp.ProcessTask(ctx, task); err == nil {
						break
					}

					performBackoffWithJitter(tries)
				}
			}
		}()
	}
}

const baseDelay = 1 * time.Second
const jitter = 250 * time.Millisecond

func performBackoffWithJitter(tries int) {
	if tries < 1 {
		tries = 1
	}

	// baseDelay * 2^(n-1)
	backoff := baseDelay * time.Duration(1<<(tries-1))

	// evict "thundering herd"
	randomJitter := time.Duration(rand.Intn(int(jitter)))
	totalWait := backoff + randomJitter
	time.Sleep(totalWait)
}
