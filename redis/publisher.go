package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

var client *redis.Client

// InitRedis initializes the Redis client
func InitRedis() error {
	client = redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "",
		DB:       0,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		fmt.Printf("⚠️  Failed to connect to Redis: %v (trade notifications disabled)\n", err)
		client = nil
		return err
	}

	fmt.Println("✅ Connected to Redis - trade executions will be published")
	return nil
}

// CloseRedis closes the Redis connection
func CloseRedis() {
	if client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		client.Shutdown(ctx)
		client.Close()
	}
}

// TradeExecution represents a single trade action
type TradeExecution struct {
	Exchange  string    `json:"exchange"`
	Pair      string    `json:"pair"`
	Side      string    `json:"side"`       // "spot_long", "futures_short", "close_spot_long", "close_futures_short"
	Action    string    `json:"action"`     // "open" or "close"
	Amount    float64   `json:"amount"`     // USDT amount
	Price     float64   `json:"price"`      // Entry/Exit price
	SpreadPct float64   `json:"spread_pct"` // Spread at execution
	Timestamp time.Time `json:"timestamp"`
}

// TradeSummary represents the final P&L after all 4 trades complete
type TradeSummary struct {
	Pair            string    `json:"pair"`
	SpotExchange    string    `json:"spot_exchange"`
	FuturesExchange string    `json:"futures_exchange"`
	EntrySpread     float64   `json:"entry_spread_pct"`
	ExitSpread      float64   `json:"exit_spread_pct"`
	SpotProfit      float64   `json:"spot_profit"`
	FuturesProfit   float64   `json:"futures_profit"`
	TotalProfit     float64   `json:"total_profit"`
	Amount          float64   `json:"amount"`
	Duration        float64   `json:"duration_seconds"`
	OpenTime        time.Time `json:"open_time"`
	CloseTime       time.Time `json:"close_time"`
}

// PublishTradeExecution publishes a single trade execution to Redis
func PublishTradeExecution(trade TradeExecution) {
	if client == nil {
		fmt.Println("⚠️  Redis client not initialized - skipping trade execution publish")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	jsonData, err := json.Marshal(trade)
	if err != nil {
		fmt.Printf("❌ Failed to marshal trade execution: %v\n", err)
		return
	}

	// Publish to trade-execution topic
	if err := client.Publish(ctx, "arbitrage-trade-execution", jsonData).Err(); err != nil {
		fmt.Printf("❌ Failed to publish trade execution to Redis: %v\n", err)
		return
	}

	fmt.Printf("📤 Published trade execution to Redis: %s %s %s on %s\n",
		trade.Action, trade.Side, trade.Pair, trade.Exchange)
}

// PublishTradeSummary publishes the final P&L summary to Redis
func PublishTradeSummary(summary TradeSummary) {
	if client == nil {
		fmt.Println("⚠️  Redis client not initialized - skipping trade summary publish")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	jsonData, err := json.Marshal(summary)
	if err != nil {
		fmt.Printf("❌ Failed to marshal trade summary: %v\n", err)
		return
	}

	// Publish to trade-summary topic
	if err := client.Publish(ctx, "arbitrage-trade-summary", jsonData).Err(); err != nil {
		fmt.Printf("❌ Failed to publish trade summary to Redis: %v\n", err)
		return
	}

	fmt.Printf("📤 Published trade summary to Redis: %s - %.4f USDT profit\n",
		summary.Pair, summary.TotalProfit)
}
