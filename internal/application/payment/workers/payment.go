package worker

import (
	"context"
	"fmt"
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
		maxRetries:  3,
	}
}

func (wp *PaymentWorkerPool) StartPaymentWorker() {
	for i := range wp.concurrency {
		ctx := context.Background()
		ctx.Value(i)
		go func() {
			for buff := range wp.queue {
				if !wp.pp.IsUp() {
					// fmt.Println("payment processor is down")
					continue
				}

				task := paymentTask.ProcessPaymentTask{}
				err := json.Unmarshal(buff, &task)
				if err != nil {
					fmt.Printf("error when unmarshal task %s\n", err.Error())
					panic(err)
				}

				if err := wp.pp.ProcessTask(ctx, task); err != nil {
					time.Sleep(2 * time.Second)
					wp.queue <- buff
				}
			}
		}()
	}
}
