package payment

import (
	"context"
	"encoding/json"
	"fmt"
)

const HEALTH_CHECK_KEY = "health_check"

type HealthCheckResponse struct {
	Failing         bool `json:"failing"`
	MinResponseTime int  `json:"minResponseTime"`
}

func (p *PaymentProcessor) HealthCheck(ctx context.Context, masterInstance bool) {
	if masterInstance {
		resp, err := p.client.Get(p.baseURL() + "/payments/service-health")
		if err != nil {
			fmt.Println(err)
			return
		}
		defer resp.Body.Close()

		healthCheckRes := HealthCheckResponse{}
		if err := json.NewDecoder(resp.Body).Decode(&healthCheckRes); err != nil {
			fmt.Println(err)
			return
		}

		fmt.Println("hc res", healthCheckRes)
		p.cache.Set(ctx, HEALTH_CHECK_KEY, !healthCheckRes.Failing, 0)
		p.SetUp(!healthCheckRes.Failing)
		return
	}

	upCached := p.cache.Get(ctx, HEALTH_CHECK_KEY)
	up, _ := upCached.Bool()
	fmt.Println("hc res", up)
	p.SetUp(up)
}
