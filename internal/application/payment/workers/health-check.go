package worker

import (
	"context"
	"time"

	paymentProcessor "github.com/payment-processor-rinha/internal/application/payment/processors"
)

type HealthCheckPool struct {
	pp *paymentProcessor.PaymentProcessor
}

func NewHealthCheckPool(pp *paymentProcessor.PaymentProcessor) *HealthCheckPool {
	return &HealthCheckPool{
		pp: pp,
	}
}

func (wp *HealthCheckPool) StartHealthCheckWorker(masterInst bool) {
	ctx := context.Background()
	go func() {
		for {
			time.Sleep(time.Second * 5)
			wp.pp.HealthCheck(ctx, masterInst)
		}
	}()
}
