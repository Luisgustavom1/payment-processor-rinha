package payment

type ProcessPaymentPayload struct {
	CorrelationId string  `json:"correlationId"`
	RequestedAt   string  `json:"requestedAt"`
	Amount        float64 `json:"amount"`
}

type PaymentProcessed struct {
	ProcessPaymentPayload
	OnDefault bool `json:"onDefault"`
}
