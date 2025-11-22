package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"arbitrage.trade/clients"
	"arbitrage.trade/clients/common"
)

type ArbitragePosition struct {
	PairName        string
	ShortExchange   common.ExchangeType
	LongExchange    common.ExchangeType
	EntryShortPrice float64
	EntryLongPrice  float64
	EntrySpread     float64
	AmountUSDT      float64
	EntryTime       time.Time
	IsOpen          bool
	mu              sync.RWMutex
}

var (
	activePositions = make(map[string]*ArbitragePosition)
	positionsMutex  sync.RWMutex
)

// UpdatePrices is called from main WebSocket loop to track current prices
func UpdatePrices(pairName string, shortExchange string, shortPrice float64, longExchange string, longPrice float64) {
	positionsMutex.RLock()
	position, exists := activePositions[pairName]
	positionsMutex.RUnlock()

	if !exists || !position.IsOpen {
		return
	}

	// Check if this price update matches our position
	if string(position.ShortExchange) != shortExchange || string(position.LongExchange) != longExchange {
		return
	}

	position.mu.Lock()
	defer position.mu.Unlock()

	// Calculate current spread
	currentSpread := ((shortPrice - longPrice) / longPrice) * 100.0

	// Calculate spread convergence percentage
	spreadConvergence := ((position.EntrySpread - currentSpread) / position.EntrySpread) * 100.0

	// Calculate estimated profit based on spread convergence
	// Entry spread was X%, current spread is Y%
	// We profit from (X - Y)% of position size, minus fees (~0.15% total)
	estimatedProfit := ((position.EntrySpread - currentSpread) / 100.0) * position.AmountUSDT
	estimatedProfitAfterFees := estimatedProfit - (position.AmountUSDT * 0.0015) // 0.15% total fees

	elapsedTime := time.Since(position.EntryTime).Seconds()

	log.Printf("[TRACK %s] Entry: %.2f%% | Current: %.2f%% | Convergence: %.1f%% | Est.Profit: $%.4f | Time: %.0fs",
		pairName, position.EntrySpread, currentSpread, spreadConvergence, estimatedProfitAfterFees, elapsedTime)

	// Exit conditions optimized for 0.2%+ entry spreads:
	// Focus on taking small profits quickly rather than waiting
	// 1. ANY positive profit after fees - take it (don't be greedy)
	// 2. Spread reversed (prices crossed) - emergency exit
	// 3. Spread widened significantly (>30% from entry) - cut loss
	// 4. Time-based safety:
	//    - After 15s: close if profit > $0.01
	//    - After 30s: close if profit > $0
	//    - After 60s: close if profit > -$0.05 (small loss acceptable)
	//    - After 90s: force close
	shouldClose := false
	reason := ""

	// Take profit aggressively - any profit after 5 seconds is good
	if elapsedTime >= 5 && estimatedProfitAfterFees > 0.01 {
		shouldClose = true
		reason = fmt.Sprintf("Quick profit: $%.4f", estimatedProfitAfterFees)
	} else if elapsedTime >= 15 && estimatedProfitAfterFees > 0.005 {
		shouldClose = true
		reason = fmt.Sprintf("Small profit after 15s: $%.4f", estimatedProfitAfterFees)
	} else if elapsedTime >= 30 && estimatedProfitAfterFees > 0 {
		shouldClose = true
		reason = fmt.Sprintf("Any profit after 30s: $%.4f", estimatedProfitAfterFees)
	} else if currentSpread <= 0 {
		shouldClose = true
		reason = "Spread reversed (emergency exit)"
	} else if currentSpread > position.EntrySpread*1.3 {
		shouldClose = true
		reason = "Spread widened 30%+ (cut loss)"
	} else if elapsedTime >= 60 && estimatedProfitAfterFees > -0.05 {
		shouldClose = true
		reason = fmt.Sprintf("60s timeout, P/L: $%.4f", estimatedProfitAfterFees)
	} else if elapsedTime >= 90 {
		shouldClose = true
		reason = fmt.Sprintf("90s force close, P/L: $%.4f", estimatedProfitAfterFees)
	}

	if shouldClose {
		log.Printf("[CLOSE %s] Reason: %s | Held for: %.0fs", pairName, reason, elapsedTime)
		go closePosition(position)
	}
}

func closePosition(position *ArbitragePosition) {
	position.mu.Lock()
	if !position.IsOpen {
		position.mu.Unlock()
		return
	}
	position.IsOpen = false
	position.mu.Unlock()

	ctx := context.Background()
	var wg sync.WaitGroup
	wg.Add(2)

	spotProfit := 0.00
	futuresProfit := 0.00

	go func() {
		defer wg.Done()
		var err error
		futuresProfit, err = clients.Execute(ctx, position.ShortExchange, common.CloseFuturesShort, position.PairName, position.AmountUSDT)
		if err != nil {
			log.Printf("[ERROR] Failed to close futures short: %v", err)
		}
	}()

	go func() {
		defer wg.Done()
		var err error
		spotProfit, err = clients.Execute(ctx, position.LongExchange, common.CloseSpotLong, position.PairName, position.AmountUSDT)
		if err != nil {
			log.Printf("[ERROR] Failed to close spot long: %v", err)
		}
	}()

	wg.Wait()

	totalProfit := spotProfit + futuresProfit
	log.Printf("[RESULT %s] Total Profit: %.4f USDT | Spot: %.4f | Futures: %.4f",
		position.PairName, totalProfit, spotProfit, futuresProfit)

	// Remove from active positions
	positionsMutex.Lock()
	delete(activePositions, position.PairName)
	positionsMutex.Unlock()
}

func ConsiderArbitrageOpportunity(ctx context.Context, shortExchange common.ExchangeType, shortPrice float64, longExchange common.ExchangeType,
	longPrice float64, pairName string, diffPercent float64, amountUSDT float64) {

	if diffPercent < 0.1 {
		return
	}

	// Check if already have an open position for this pair
	positionsMutex.RLock()
	_, exists := activePositions[pairName]
	positionsMutex.RUnlock()

	if exists {
		log.Printf("[SKIP %s] Position already open", pairName)
		return
	}

	log.Printf("[OPEN %s] Short: %s@%.6f | Long: %s@%.6f | Spread: %.2f%%",
		pairName, shortExchange, shortPrice, longExchange, longPrice, diffPercent)

	// Create position tracking
	position := &ArbitragePosition{
		PairName:        pairName,
		ShortExchange:   shortExchange,
		LongExchange:    longExchange,
		EntryShortPrice: shortPrice,
		EntryLongPrice:  longPrice,
		EntrySpread:     diffPercent,
		AmountUSDT:      amountUSDT,
		EntryTime:       time.Now(),
		IsOpen:          true,
	}

	positionsMutex.Lock()
	activePositions[pairName] = position
	positionsMutex.Unlock()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		_, err := clients.Execute(ctx, shortExchange, common.PutFuturesShort, pairName, amountUSDT)
		if err != nil {
			log.Printf("[ERROR] Failed to open futures short: %v", err)
			position.mu.Lock()
			position.IsOpen = false
			position.mu.Unlock()
		}
	}()

	go func() {
		defer wg.Done()
		_, err := clients.Execute(ctx, longExchange, common.PutSpotLong, pairName, amountUSDT)
		if err != nil {
			log.Printf("[ERROR] Failed to open spot long: %v", err)
			position.mu.Lock()
			position.IsOpen = false
			position.mu.Unlock()
		}
	}()

	wg.Wait()

	// If opening failed, clean up
	position.mu.RLock()
	isOpen := position.IsOpen
	position.mu.RUnlock()

	if !isOpen {
		positionsMutex.Lock()
		delete(activePositions, pairName)
		positionsMutex.Unlock()
		log.Printf("[FAILED %s] Could not open position", pairName)
	} else {
		log.Printf("[OPENED %s] Position opened successfully, monitoring for exit...", pairName)
	}
}
