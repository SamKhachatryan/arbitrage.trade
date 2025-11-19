package binance

import (
	"net/http"
	"sync"

	"arbitrage.trade/clients/common"
)

type BinanceClient struct {
	apiKey      string
	apiSecret   string
	spotBaseURL string
	futsBaseURL string
	httpClient  *http.Client

	// Track open positions
	positions map[string]*common.Position
	posMutex  sync.RWMutex
}

type AccountBalance struct {
	Asset  string `json:"asset"`
	Free   string `json:"free"`
	Locked string `json:"locked"`
}

type AccountInfo struct {
	Balances []AccountBalance `json:"balances"`
}

type Fill struct {
	Price           string `json:"price"`
	Qty             string `json:"qty"`
	Commission      string `json:"commission"`
	CommissionAsset string `json:"commissionAsset"`
}

type PositionRisk struct {
	Symbol           string  `json:"symbol"`
	PositionAmt      float64 `json:"positionAmt,string"`
	EntryPrice       float64 `json:"entryPrice,string"`
	MarkPrice        float64 `json:"markPrice,string"`
	UnrealizedProfit float64 `json:"unRealizedProfit,string"`
	LiquidationPrice float64 `json:"liquidationPrice,string"`
	Leverage         float64 `json:"leverage,string"`
	PositionSide     string  `json:"positionSide"`
}
