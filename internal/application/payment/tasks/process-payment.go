package payment

type ProcessPaymentPayload struct {
	CorrelationId string  `json:"correlationId"`
	RequestedAt   string  `json:"requestedAt"`
	Amount        float64 `json:"amount"`
}

type ProcessPaymentTask struct {
	ProcessPaymentPayload
	OnDefault bool `json:"onDefault"`
	Tries     int  `json:"tries"`
}

const (
	ProcessPayment = "payment:process"
)
