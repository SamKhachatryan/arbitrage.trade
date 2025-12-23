package main

import (
	"context"
	"log"
	"os"
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
	LastLogTime     time.Time // Track when we last logged to avoid spam
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
	// 3. Maximum hold time of 120 seconds (safety exit)
	shouldClose := false
	reason := ""

	if spreadConvergence >= 60.0 {
		shouldClose = true
		reason = "Spread converged 60%+"
	} else if currentSpread <= 0 {
		shouldClose = true
		reason = "Spread reversed (prices crossed)"
	} else if elapsedTime >= 60 {
		shouldClose = true
		reason = "Max hold time reached (60s)"
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
	log.Printf("[💰 RESULT %s] Total Profit: %.4f USDT | Spot: %.4f | Futures: %.4f",
		position.PairName, totalProfit, spotProfit, futuresProfit)

	// Remove from active positions
	positionsMutex.Lock()
	delete(activePositions, position.PairName)
	positionsMutex.Unlock()

	// Position closed, now exit the program
	log.Println("✅ Position closed successfully. Program will terminate.")
	time.Sleep(1 * time.Second)
	os.Exit(0)
}

func ConsiderArbitrageOpportunity(ctx context.Context, shortExchange common.ExchangeType, shortPrice float64, longExchange common.ExchangeType,
	longPrice float64, pairName string, diffPercent float64, amountUSDT float64) {

	if common.LessThan(diffPercent, 0.5) {
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
