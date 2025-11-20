package whitebit

import (
	"net/http"
	"sync"

	"arbitrage.trade/clients/common"
)

type WhitebitClient struct {
	apiKey     string
	apiSecret  string
	baseURL    string
	httpClient *http.Client

	positions map[string]*common.Position
	mu        sync.RWMutex

	// Rate limiter - allows only one request at a time
	rateLimiter chan struct{}
}

type BalanceResponse struct {
	Available string `json:"available"`
	Freeze    string `json:"freeze"`
}

type MarketOrderResponse struct {
	OrderID       int64   `json:"orderId"`
	ClientOrderID string  `json:"clientOrderId"`
	Market        string  `json:"market"`
	Side          string  `json:"side"`
	Type          string  `json:"type"`
	Timestamp     float64 `json:"timestamp"`
	DealMoney     string  `json:"dealMoney"`
	DealStock     string  `json:"dealStock"`
	Amount        string  `json:"amount"`
	DealFee       string  `json:"dealFee"`
	Status        string  `json:"status"`
}

type CollateralPosition struct {
	PositionID   int    `json:"positionId"`
	Market       string `json:"market"`
	Amount       string `json:"amount"`
	BasePrice    string `json:"basePrice"`
	PNL          string `json:"pnl"`
	PositionSide string `json:"positionSide"`
}

type OpenPositionsResponse []CollateralPosition

type OrderStatusResponse struct {
	OrderID       int64   `json:"orderId"`
	ClientOrderID string  `json:"clientOrderId"`
	Market        string  `json:"market"`
	Side          string  `json:"side"`
	Type          string  `json:"type"`
	Timestamp     float64 `json:"timestamp"`
	DealMoney     string  `json:"dealMoney"`
	DealStock     string  `json:"dealStock"`
	Amount        string  `json:"amount"`
	TakerFee      string  `json:"takerFee"`
	MakerFee      string  `json:"makerFee"`
	Left          string  `json:"left"`
	DealFee       string  `json:"dealFee"`
	Status        string  `json:"status"` // "FILLED", "PENDING", "CANCELLED", etc.
}
