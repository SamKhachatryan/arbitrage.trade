package bitget

import (
	"net/http"
	"time"

	"arbitrage.trade/clients/common"
)

func NewBitgetClient(apiKey, apiSecret, passphrase string) *BitgetClient {
	return &BitgetClient{
		apiKey:     apiKey,
		apiSecret:  apiSecret,
		passphrase: passphrase,
		baseURL:    "https://api.bitget.com",
		httpClient: &http.Client{Timeout: 30 * time.Second},
		positions:  make(map[string]*common.Position),
	}
}

func (b *BitgetClient) GetName() string { return "bitget" }
