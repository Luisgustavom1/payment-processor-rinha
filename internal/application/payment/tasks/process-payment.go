package payment

import (
	"encoding/json"

	"github.com/hibiken/asynq"
)

type ProcessPaymentPayload struct {
	CorrelationId string  `json:"correlationId"`
	RequestedAt   string  `json:"requestedAt"`
	Amount        float64 `json:"amount"`
}

type ProcessPaymentTask struct {
	ProcessPaymentPayload
	OnDefault bool `json:"onDefault"`
}

const (
	ProcessPayment = "payment:process"
)

func NewProcessPaymentTask(task ProcessPaymentTask) (*asynq.Task, error) {
	payload, err := json.Marshal(task)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(ProcessPayment, payload, asynq.MaxRetry(3)), nil
}
