package binance

import (
	"net/http"
	"time"

	"arbitrage.trade/clients/common"
)

func NewBinanceClient(apiKey, apiSecret string) *BinanceClient {
	return &BinanceClient{
		apiKey:      apiKey,
		apiSecret:   apiSecret,
		spotBaseURL: "https://api.binance.com",
		futsBaseURL: "https://fapi.binance.com",
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		positions: make(map[string]*common.Position),
	}
}

func (b *BinanceClient) GetName() string { return "binance" }
