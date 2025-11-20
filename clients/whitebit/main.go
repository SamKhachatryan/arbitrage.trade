package whitebit

import (
	"net/http"
	"time"

	"arbitrage.trade/clients/common"
)

func NewWhitebitClient(apiKey, apiSecret string) *WhitebitClient {
	rateLimiter := make(chan struct{}, 1)
	rateLimiter <- struct{}{}

	return &WhitebitClient{
		apiKey:    apiKey,
		apiSecret: apiSecret,
		baseURL:   "https://whitebit.com",
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		positions:   make(map[string]*common.Position),
		rateLimiter: rateLimiter,
	}
}

func (w *WhitebitClient) GetName() string {
	return "whitebit"
}
