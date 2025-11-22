package common

import (
	"context"
	"errors"
)

// ExchangeTradeClient defines the interface for executing arbitrage trades
type ExchangeTradeClient interface {
	// PutSpotLong opens a long position in the spot market (buys the asset with USDT)
	PutSpotLong(ctx context.Context, pairName string, amountUSDT float64) (*TradeResult, error)

	// PutFuturesShort opens a short position in the futures/perpetual market
	PutFuturesShort(ctx context.Context, pairName string, amountUSDT float64) (*TradeResult, error)

	// CloseSpotLong closes the long spot position (converts asset back to USDT)
	CloseSpotLong(ctx context.Context, pairName string, amoountUSDT float64) (*TradeResult, float64, error)

	// CloseFuturesShort closes the short futures position
	CloseFuturesShort(ctx context.Context, pairName string) (*TradeResult, float64, error)

	// GetName returns the exchange name
	GetName() string
}

// TradeResult contains the result of a trade operation
type TradeResult struct {
	OrderID       string  // Exchange's order ID
	ExecutedPrice float64 // Actual execution price
	ExecutedQty   float64 // Quantity executed
	Fee           float64 // Trading fee paid
	Success       bool    // Whether the trade was successful
}

// Position tracks an open position
type Position struct {
	PairName     string
	Side         string // "long" or "short"
	Market       string // "spot" or "futures"
	EntryPrice   float64
	Quantity     float64
	AmountUSDT   float64
	OrderID      string
	ExchangeName string
}

type ExchangeType string

const (
	Binance  ExchangeType = "binance"
	Bitget   ExchangeType = "bitget"
	Whitebit ExchangeType = "whitebit"
	Gate     ExchangeType = "gate"
	Okx      ExchangeType = "okx"
)

type OrderType string

const (
	PutSpotLong       OrderType = "PutSpotLong"
	CloseSpotLong     OrderType = "CloseSpotLong"
	PutFuturesShort   OrderType = "PutFuturesShort"
	CloseFuturesShort OrderType = "CloseFuturesShort"
)

var (
	ErrInsufficientBalance = errors.New("insufficient balance")
	ErrInvalidPair         = errors.New("invalid trading pair")
	ErrOrderFailed         = errors.New("order execution failed")
	ErrPositionNotFound    = errors.New("position not found")
	ErrConnectionFailed    = errors.New("exchange connection failed")
)
