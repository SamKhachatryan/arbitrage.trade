package gate

import (
	"net/http"
	"time"

	"arbitrage.trade/clients/common"
)

func NewGateClient(apiKey, apiSecret string) *GateClient {
	return &GateClient{
		apiKey:    apiKey,
		apiSecret: apiSecret,
		baseURL:   "https://api.gateio.ws",
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		positions: make(map[string]*common.Position),
	}
}

func (g *GateClient) GetName() string {
	return "gate"
}
