package gate

import (
	"net/http"
	"sync"

	"arbitrage.trade/clients/common"
)

type GateClient struct {
	apiKey     string
	apiSecret  string
	baseURL    string
	httpClient *http.Client

	positions map[string]*common.Position
	mu        sync.RWMutex
}

type SpotBalance struct {
	Currency  string `json:"currency"`
	Available string `json:"available"`
	Locked    string `json:"locked"`
}

type SpotOrderResponse struct {
	ID           string `json:"id"`
	Text         string `json:"text"`
	CurrencyPair string `json:"currency_pair"`
	Status       string `json:"status"`
	Type         string `json:"type"`
	Side         string `json:"side"`
	Amount       string `json:"amount"`
	Price        string `json:"price"`
	FilledTotal  string `json:"filled_total"`
	AvgDealPrice string `json:"avg_deal_price"`
	Fee          string `json:"fee"`
	FeeCurrency  string `json:"fee_currency"`
	CreateTime   string `json:"create_time"`
	CreateTimeMs string `json:"create_time_ms"`
}

type FuturesBalance struct {
	Currency  string `json:"currency"`
	Available string `json:"available"`
	Total     string `json:"total"`
}

type FuturesPosition struct {
	Contract      string `json:"contract"`
	Size          int64  `json:"size"`
	Leverage      string `json:"leverage"`
	EntryPrice    string `json:"entry_price"`
	LiqPrice      string `json:"liq_price"`
	MarkPrice     string `json:"mark_price"`
	UnrealisedPnl string `json:"unrealised_pnl"`
	RealisedPnl   string `json:"realised_pnl"`
	Mode          string `json:"mode"`
}

type FuturesOrderResponse struct {
	ID         int64  `json:"id"`
	Contract   string `json:"contract"`
	Size       int64  `json:"size"`
	Price      string `json:"price"`
	Status     string `json:"status"`
	FillPrice  string `json:"fill_price"`
	Left       int64  `json:"left"`
	TkfFee     string `json:"tkf_fee"`
	CreateTime string `json:"create_time"`
	FinishTime string `json:"finish_time"`
}
