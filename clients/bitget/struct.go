package bitget

import (
	"net/http"
	"sync"

	"arbitrage.trade/clients/common"
)

type BitgetClient struct {
	apiKey     string
	apiSecret  string
	passphrase string
	baseURL    string
	httpClient *http.Client
	positions  map[string]*common.Position
	mu         sync.RWMutex
}

type FuturesPositionInfo struct {
	Total    float64
	Entry    float64
	HoldSide string
}
