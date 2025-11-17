package main

import (
	"context"
	"log"
	"sync"
	"time"
)

// ArbitrageExecutor manages the execution of arbitrage trades
type ArbitrageExecutor struct {
	clients        map[string]ExchangeTradeClient
	activeTradesMu sync.RWMutex
	activeTrades   map[string]*ArbitrageTrade // key: pairName
	minProfitPct   float64                    // minimum profit percentage to execute
	maxTradeAmount float64                    // max USDT per trade
}

// ArbitrageTrade represents an active arbitrage position
type ArbitrageTrade struct {
	PairName        string
	SpotExchange    string
	FuturesExchange string
	EntrySpotPrice  float64
	EntryFutPrice   float64
	AmountUSDT      float64
	OpenedAt        time.Time
	SpotResult      *TradeResult
	FuturesResult   *TradeResult
}

// NewArbitrageExecutor creates a new arbitrage executor
func NewArbitrageExecutor(minProfitPct, maxTradeAmount float64) *ArbitrageExecutor {
	return &ArbitrageExecutor{
		clients:        make(map[string]ExchangeTradeClient),
		activeTrades:   make(map[string]*ArbitrageTrade),
		minProfitPct:   minProfitPct,
		maxTradeAmount: maxTradeAmount,
	}
}

// RegisterClient adds an exchange client
func (ae *ArbitrageExecutor) RegisterClient(client ExchangeTradeClient) {
	ae.clients[client.GetName()] = client
	log.Printf("[EXECUTOR] Registered exchange client: %s", client.GetName())
}

// ExecuteArbitrage executes an arbitrage opportunity
func (ae *ArbitrageExecutor) ExecuteArbitrage(
	ctx context.Context,
	pairName string,
	spotExchange string,
	futuresExchange string,
	spotPrice float64,
	futPrice float64,
	profitPct float64,
) error {
	// Check if we already have an active trade for this pair
	ae.activeTradesMu.RLock()
	_, exists := ae.activeTrades[pairName]
	ae.activeTradesMu.RUnlock()

	if exists {
		log.Printf("[EXECUTOR] Skipping %s - already have active trade", pairName)
		return nil
	}

	// Check if profit meets threshold
	if profitPct < ae.minProfitPct {
		log.Printf("[EXECUTOR] Skipping %s - profit %.2f%% below threshold %.2f%%",
			pairName, profitPct, ae.minProfitPct)
		return nil
	}

	log.Printf("[EXECUTOR] ============================================")
	log.Printf("[EXECUTOR] 🎯 Executing arbitrage for %s", pairName)
	log.Printf("[EXECUTOR]    Spot: %s @ %.8f", spotExchange, spotPrice)
	log.Printf("[EXECUTOR]    Futures: %s @ %.2f", futuresExchange, futPrice)
	log.Printf("[EXECUTOR]    Expected Profit: %.2f%%", profitPct)
	log.Printf("[EXECUTOR] ============================================")

	// Get the appropriate client (for now we assume same client for both markets)
	client, ok := ae.clients[spotExchange]
	if !ok {
		log.Printf("[EXECUTOR] ❌ No client found for exchange: %s", spotExchange)
		return nil
	}

	trade := &ArbitrageTrade{
		PairName:        pairName,
		SpotExchange:    spotExchange,
		FuturesExchange: futuresExchange,
		EntrySpotPrice:  spotPrice,
		EntryFutPrice:   futPrice,
		AmountUSDT:      ae.maxTradeAmount,
		OpenedAt:        time.Now(),
	}

	// Execute spot long
	log.Printf("[EXECUTOR] Opening spot long on %s...", spotExchange)
	spotResult, err := client.PutSpotLong(ctx, pairName, ae.maxTradeAmount)
	if err != nil {
		log.Printf("[EXECUTOR] ❌ Failed to open spot long: %v", err)
		return err
	}
	trade.SpotResult = spotResult
	log.Printf("[EXECUTOR] ✅ Spot long opened: %s", spotResult.Message)

	// Execute futures short
	log.Printf("[EXECUTOR] Opening futures short on %s...", futuresExchange)
	futResult, err := client.PutFuturesShort(ctx, pairName, ae.maxTradeAmount)
	if err != nil {
		log.Printf("[EXECUTOR] ❌ Failed to open futures short: %v", err)
		// Try to close spot position
		log.Printf("[EXECUTOR] ⚠️  Attempting to close spot position...")
		if closeResult, closeErr := client.CloseSpotLong(ctx, pairName); closeErr != nil {
			log.Printf("[EXECUTOR] ❌ Failed to close spot: %v", closeErr)
		} else {
			log.Printf("[EXECUTOR] ✅ Emergency spot close: %s", closeResult.Message)
		}
		return err
	}
	trade.FuturesResult = futResult
	log.Printf("[EXECUTOR] ✅ Futures short opened: %s", futResult.Message)

	// Store active trade
	ae.activeTradesMu.Lock()
	ae.activeTrades[pairName] = trade
	ae.activeTradesMu.Unlock()

	log.Printf("[EXECUTOR] ✅ Arbitrage positions opened for %s", pairName)
	return nil
}

// CloseArbitrage closes an arbitrage position
func (ae *ArbitrageExecutor) CloseArbitrage(ctx context.Context, pairName string) error {
	ae.activeTradesMu.RLock()
	trade, exists := ae.activeTrades[pairName]
	ae.activeTradesMu.RUnlock()

	if !exists {
		log.Printf("[EXECUTOR] No active trade found for %s", pairName)
		return nil
	}

	log.Printf("[EXECUTOR] ============================================")
	log.Printf("[EXECUTOR] 🔄 Closing arbitrage for %s", pairName)
	log.Printf("[EXECUTOR]    Position age: %v", time.Since(trade.OpenedAt))
	log.Printf("[EXECUTOR] ============================================")

	client, ok := ae.clients[trade.SpotExchange]
	if !ok {
		log.Printf("[EXECUTOR] ❌ No client found for exchange: %s", trade.SpotExchange)
		return nil
	}

	// Close spot long
	log.Printf("[EXECUTOR] Closing spot long...")
	spotCloseResult, err := client.CloseSpotLong(ctx, pairName)
	if err != nil {
		log.Printf("[EXECUTOR] ❌ Failed to close spot: %v", err)
	} else {
		log.Printf("[EXECUTOR] ✅ Spot closed: %s", spotCloseResult.Message)
	}

	// Close futures short
	log.Printf("[EXECUTOR] Closing futures short...")
	futCloseResult, err := client.CloseFuturesShort(ctx, pairName)
	if err != nil {
		log.Printf("[EXECUTOR] ❌ Failed to close futures: %v", err)
	} else {
		log.Printf("[EXECUTOR] ✅ Futures closed: %s", futCloseResult.Message)
	}

	// Remove from active trades
	ae.activeTradesMu.Lock()
	delete(ae.activeTrades, pairName)
	ae.activeTradesMu.Unlock()

	log.Printf("[EXECUTOR] ✅ Arbitrage closed for %s", pairName)
	return nil
}

// MonitorAndClose monitors active trades and closes them based on conditions
func (ae *ArbitrageExecutor) MonitorAndClose(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ae.activeTradesMu.RLock()
			for pairName, trade := range ae.activeTrades {
				// Close trades older than 5 minutes (example condition)
				if time.Since(trade.OpenedAt) > 5*time.Minute {
					log.Printf("[EXECUTOR] Trade %s exceeded max duration, closing...", pairName)
					go ae.CloseArbitrage(ctx, pairName)
				}
			}
			ae.activeTradesMu.RUnlock()
		}
	}
}

// GetActiveTrades returns a copy of active trades
func (ae *ArbitrageExecutor) GetActiveTrades() map[string]*ArbitrageTrade {
	ae.activeTradesMu.RLock()
	defer ae.activeTradesMu.RUnlock()

	result := make(map[string]*ArbitrageTrade)
	for k, v := range ae.activeTrades {
		result[k] = v
	}
	return result
}

// Example integration with your existing websocket code:
/*
func integrateWithWebsocket() {
	executor := NewArbitrageExecutor(0.15, 100.0) // 0.15% min profit, $100 max per trade

	// Register exchange clients
	binanceClient := NewBinanceClient(apiKey, apiSecret)
	executor.RegisterClient(binanceClient)

	ctx := context.Background()

	// Start monitoring goroutine
	go executor.MonitorAndClose(ctx)

	// In your websocket loop where you detect arbitrage:
	for {
		// ... your existing code to detect arbitrage ...

		if diff >= threshold {
			r1 := getReliability(p1)
			r2 := getReliability(p2)
			if r1 > Low && r2 > Low {
				buyEx := ex1
				sellEx := ex2
				spotPrice := low
				futPrice := high

				if p1.Price > p2.Price {
					buyEx, sellEx = ex2, ex1
					spotPrice = high
					futPrice = low
				}

				// Execute the arbitrage
				err := executor.ExecuteArbitrage(
					ctx,
					pairName,
					buyEx,
					sellEx,
					spotPrice,
					futPrice,
					diff,
				)
				if err != nil {
					log.Printf("Failed to execute arbitrage: %v", err)
				}
			}
		}
	}
}
*/
