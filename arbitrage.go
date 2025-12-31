package main

import (
	"context"
	"log"
	"sync"
	"time"

	"arbitrage.trade/clients/common"
	"arbitrage.trade/orderbook"
	"arbitrage.trade/redis"
)

var (
	activePositions = make(map[string]*ArbitragePosition)
	positionsMutex  sync.RWMutex
	globalAnalyzer  *orderbook.Analyzer // Reference to reset execution flag after trade closes
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
	LastLogTime     time.Time // Track when we last logged to avoid spam
	mu              sync.RWMutex
}

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

	elapsedTime := time.Since(position.EntryTime).Seconds()

	// Only log every 2 seconds to avoid spam
	timeSinceLastLog := time.Since(position.LastLogTime).Seconds()
	if timeSinceLastLog >= 2.0 {
		log.Printf("[TRACK %s] Entry: %.2f%% | Current: %.2f%% | Convergence: %.1f%% | Time: %.0fs",
			pairName, position.EntrySpread, currentSpread, spreadConvergence, elapsedTime)
		position.LastLogTime = time.Now()
	}

	// Exit conditions:
	// 1. Spread has converged by 60% or more (profit target)
	// 2. Spread has reversed (negative means prices crossed)
	// 3. Maximum hold time of 60 seconds (safety exit)
	shouldClose := false
	reason := ""

	if spreadConvergence >= 60.0 {
		shouldClose = true
		reason = "Spread converged 60%+"
	} else if currentSpread <= 0 {
		shouldClose = true
		reason = "Spread reversed (prices crossed)"
	} else if elapsedTime >= 58 {
		shouldClose = true
		reason = "Max hold time reached (58s+)"
		log.Printf("[DEBUG] Triggering close: elapsedTime=%.2f >= 58", elapsedTime)
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

	// TESTING: Simulate trade closes and Redis publishing
	spotProfit := 0.15
	futuresProfit := 0.12

	// Get simulated exit prices (slightly different from entry)
	exitShortPrice := position.EntryShortPrice * 0.995 // 0.5% lower
	exitLongPrice := position.EntryLongPrice * 1.005   // 0.5% higher
	exitSpread := ((exitShortPrice - exitLongPrice) / exitLongPrice) * 100.0

	log.Printf("[SIMULATED] Closing futures short on %s", position.ShortExchange)
	redis.PublishTradeExecution(redis.TradeExecution{
		Exchange:  string(position.ShortExchange),
		Pair:      position.PairName,
		Side:      "futures_short",
		Action:    "close",
		Amount:    position.AmountUSDT,
		Price:     exitShortPrice,
		SpreadPct: exitSpread,
		Timestamp: time.Now(),
	})

	log.Printf("[SIMULATED] Closing spot long on %s", position.LongExchange)
	redis.PublishTradeExecution(redis.TradeExecution{
		Exchange:  string(position.LongExchange),
		Pair:      position.PairName,
		Side:      "spot_long",
		Action:    "close",
		Amount:    position.AmountUSDT,
		Price:     exitLongPrice,
		SpreadPct: exitSpread,
		Timestamp: time.Now(),
	})

	// TESTING: Actual trades disabled, execution commented out
	/*
		ctx := context.Background()
		var wg sync.WaitGroup
		wg.Add(2)

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
	*/

	log.Printf("[SIMULATED] Trades closed (not executed, Redis testing mode)")

	totalProfit := spotProfit + futuresProfit
	duration := time.Since(position.EntryTime).Seconds()

	log.Printf("[💰 RESULT %s] Total Profit: %.4f USDT | Spot: %.4f | Futures: %.4f",
		position.PairName, totalProfit, spotProfit, futuresProfit)

	// Publish trade summary to Redis
	redis.PublishTradeSummary(redis.TradeSummary{
		Pair:            position.PairName,
		SpotExchange:    string(position.LongExchange),
		FuturesExchange: string(position.ShortExchange),
		EntrySpread:     position.EntrySpread,
		ExitSpread:      exitSpread,
		SpotProfit:      spotProfit,
		FuturesProfit:   futuresProfit,
		TotalProfit:     totalProfit,
		Amount:          position.AmountUSDT,
		Duration:        duration,
		OpenTime:        position.EntryTime,
		CloseTime:       time.Now(),
	})

	// Remove from active positions
	positionsMutex.Lock()
	delete(activePositions, position.PairName)
	positionsMutex.Unlock()

	// Reset execution flag to allow next trade
	if globalAnalyzer != nil {
		globalAnalyzer.ResetExecutionFlag()
	}

	// Position closed successfully - ready for next trade
	log.Printf("✅ Position closed successfully. Ready for next opportunity.")
}

func ConsiderArbitrageOpportunity(ctx context.Context, shortExchange common.ExchangeType, shortPrice float64, longExchange common.ExchangeType,
	longPrice float64, pairName string, diffPercent float64, amountUSDT float64) {

	// TESTING: Reduced threshold to 0.0001% for Redis testing
	if common.LessThan(diffPercent, 0.0001) {
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
		LastLogTime:     time.Now(),
		IsOpen:          true,
	}

	positionsMutex.Lock()
	activePositions[pairName] = position
	positionsMutex.Unlock()

	// Start a safety timer to force close after 65 seconds if UpdatePrices fails
	go func() {
		time.Sleep(65 * time.Second)
		position.mu.RLock()
		stillOpen := position.IsOpen
		position.mu.RUnlock()

		if stillOpen {
			log.Printf("[FORCE CLOSE %s] Safety timer triggered - position held too long", pairName)
			closePosition(position)
		}
	}()

	// TESTING: Simulate trade execution and Redis publishing
	log.Printf("[SIMULATED] Opening futures short on %s", shortExchange)
	redis.PublishTradeExecution(redis.TradeExecution{
		Exchange:  string(shortExchange),
		Pair:      pairName,
		Side:      "futures_short",
		Action:    "open",
		Amount:    amountUSDT,
		Price:     shortPrice,
		SpreadPct: diffPercent,
		Timestamp: time.Now(),
	})

	log.Printf("[SIMULATED] Opening spot long on %s", longExchange)
	redis.PublishTradeExecution(redis.TradeExecution{
		Exchange:  string(longExchange),
		Pair:      pairName,
		Side:      "spot_long",
		Action:    "open",
		Amount:    amountUSDT,
		Price:     longPrice,
		SpreadPct: diffPercent,
		Timestamp: time.Now(),
	})

	// TESTING: Trades disabled, actual execution commented out
	/*
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
	*/

	// Simulate successful trade execution for Redis testing
	log.Printf("[SIMULATED] Trades opened successfully (not executed, Redis testing mode)")

	// Simulate successful trade execution for Redis testing
	log.Printf("[SIMULATED] Trades opened successfully (not executed, Redis testing mode)")

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
