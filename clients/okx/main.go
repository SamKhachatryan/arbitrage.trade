package okx

import (
	"net/http"
	"time"

	"arbitrage.trade/clients/common"
)

func NewOkxClient(apiKey, apiSecret, passphrase string) *OkxClient {
	return &OkxClient{
		apiKey:     apiKey,
		apiSecret:  apiSecret,
		passphrase: passphrase,
		baseURL:    "https://www.okx.com",
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		positions: make(map[string]*common.Position),
	}
}

func (o *OkxClient) GetName() string {
	return "okx"
}
