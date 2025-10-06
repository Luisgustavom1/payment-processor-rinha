package worker

import (
	"context"
	"fmt"
	"time"

	json "github.com/json-iterator/go"
	paymentProcessor "github.com/payment-processor-rinha/internal/application/payment/processors"
	paymentTask "github.com/payment-processor-rinha/internal/application/payment/tasks"
)

type WorkerPool struct {
	pp          *paymentProcessor.PaymentProcessor
	concurrency int
	queue       chan []byte
	maxRetries  int
}

func NewWorker(pp *paymentProcessor.PaymentProcessor, queue chan []byte, concurrency int) *WorkerPool {
	return &WorkerPool{
		pp:          pp,
		concurrency: concurrency,
		queue:       queue,
		maxRetries:  3,
	}
}

func (wp *WorkerPool) StartWorker() {
	for i := range wp.concurrency {
		ctx := context.Background()
		ctx.Value(i)

		go func() {
			for buff := range wp.queue {
				task := paymentTask.ProcessPaymentTask{}
				err := json.Unmarshal(buff, &task)
				if err != nil {
					fmt.Printf("error when unmarshal task %s\n", err.Error())
					panic(err)
				}

				task.Tries++
				if err := wp.pp.ProcessTask(ctx, task); err != nil {
					toWait := time.Duration(task.Tries) * time.Second
					time.Sleep(toWait)
					if task.Tries < wp.maxRetries {
						newBuff, _ := json.Marshal(task)
						wp.queue <- newBuff
					} else {
						fmt.Printf("max retries reached for task %s\n", task.CorrelationId)
					}
				}
			}
		}()
	}
}
