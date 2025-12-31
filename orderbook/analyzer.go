package orderbook

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"arbitrage.trade/clients/common"
)

// OpportunityCallback is called when a valid arbitrage opportunity is found
type OpportunityCallback func(ctx context.Context, opp *Opportunity) bool

// PriceUpdateCallback is called on each price update for position tracking
type PriceUpdateCallback func(pairName string, shortExchange string, shortPrice float64, longExchange string, longPrice float64)

// Analyzer performs arbitrage analysis on orderbook updates
type Analyzer struct {
	globalManager       *GlobalManager
	logFile             *os.File
	logMu               sync.Mutex
	executionCallback   OpportunityCallback
	priceUpdateCallback PriceUpdateCallback
	executionMu         sync.Mutex
	isExecuting         bool
	supportedExchanges  map[string]bool
}

// Opportunity represents a detected arbitrage opportunity
type Opportunity struct {
	Pair            string
	SpotExchange    string
	PerpExchange    string
	SpotAskPrice    float64
	SpotAskVolume   float64
	PerpBidPrice    float64
	PerpBidVolume   float64
	SpreadPct       float64
	UsableVolumeUSD float64 // Minimum volume that can be used on both sides
	Timestamp       time.Time
}

// NewAnalyzer creates a new orderbook analyzer
func NewAnalyzer(gm *GlobalManager, supportedExchanges map[string]bool) *Analyzer {
	// Create/open log file for opportunities
	logFile, err := os.OpenFile("opportunities.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("⚠️  Failed to open opportunities log file: %v\n", err)
		logFile = nil
	} else {
		// Write header if file is new
		stat, _ := logFile.Stat()
		if stat.Size() == 0 {
			header := "=== ARBITRAGE OPPORTUNITIES LOG ===\n"
			header += "Format: [TIMESTAMP] Pair | Spot Exchange @ Price (Volume) | Perp Exchange @ Price (Volume) | Spread % | Potential Profit\n\n"
			logFile.WriteString(header)
		}
	}

	return &Analyzer{
		globalManager:      gm,
		logFile:            logFile,
		supportedExchanges: supportedExchanges,
	}
}

// SetExecutionCallback sets the callback function to execute trades
func (a *Analyzer) SetExecutionCallback(callback OpportunityCallback) {
	a.executionCallback = callback
}

// SetPriceUpdateCallback sets the callback function for position tracking price updates
func (a *Analyzer) SetPriceUpdateCallback(callback PriceUpdateCallback) {
	a.priceUpdateCallback = callback
}

// ResetExecutionFlag resets the execution flag to allow new trades
// This should be called after a position is closed
func (a *Analyzer) ResetExecutionFlag() {
	a.executionMu.Lock()
	a.isExecuting = false
	a.executionMu.Unlock()
	fmt.Println("🔓 Execution flag reset - ready for next trade")
}

// Close closes the log file
func (a *Analyzer) Close() {
	if a.logFile != nil {
		a.logFile.Close()
	}
}

// AnalyzePair analyzes a specific pair for arbitrage opportunities
// This is called whenever a pair receives an update from the signal
func (a *Analyzer) AnalyzePair(pairName string) {
	pm, exists := a.globalManager.GetPairManager(pairName)
	if !exists {
		return
	}

	opportunity := a.analyzeSignal(pm)
	if opportunity != nil {
		// Check if both exchanges are supported
		spotSupported := a.supportedExchanges[opportunity.SpotExchange]
		perpSupported := a.supportedExchanges[opportunity.PerpExchange]

		// Check if exchanges are different
		differentExchanges := opportunity.SpotExchange != opportunity.PerpExchange

		// Call price update callback for position tracking (if set)
		if a.priceUpdateCallback != nil && spotSupported && perpSupported && differentExchanges {
			a.priceUpdateCallback(pairName, opportunity.PerpExchange, opportunity.PerpBidPrice, opportunity.SpotExchange, opportunity.SpotAskPrice)
		}

		// TESTING: Execute trade if both exchanges are supported, different, and spread >= 0.0001% (Redis testing)
		if spotSupported && perpSupported && differentExchanges && common.GreaterThanOrEqual(opportunity.SpreadPct, 0.0001) {
			a.executeOpportunity(opportunity)
		}
	}
}

// executeOpportunity attempts to execute a trade for the given opportunity
func (a *Analyzer) executeOpportunity(opp *Opportunity) {
	// Check if already executing
	a.executionMu.Lock()
	if a.isExecuting {
		a.executionMu.Unlock()
		return
	}
	a.isExecuting = true
	a.executionMu.Unlock()

	// Call the execution callback if set
	if a.executionCallback != nil {
		ctx := context.Background()
		success := a.executionCallback(ctx, opp)

		if success {
			// Position opened successfully, DO NOT EXIT - let position tracking close it
			fmt.Println("✅ Trade opened successfully. Monitoring position for exit...")
			// Keep running to allow position tracking to work
			return
		}
	}

	// Reset execution flag if trade didn't succeed
	a.executionMu.Lock()
	a.isExecuting = false
	a.executionMu.Unlock()
}

// logOpportunity logs an opportunity to console and file with detailed information
func (a *Analyzer) logOpportunity(opp *Opportunity) {
	// Calculate potential profit on the usable volume
	profitPerUnit := opp.PerpBidPrice - opp.SpotAskPrice
	profitPct := (profitPerUnit / opp.SpotAskPrice) * 100.0
	estimatedProfit := opp.UsableVolumeUSD * (profitPct / 100.0)

	// Format log message with comprehensive info
	timestamp := time.Now().Format("2006-01-02 15:04:05.000")
	logMsg := fmt.Sprintf("[%s] %s | Spot: %s @ $%.8f (vol: %.4f) | Perp: %s @ $%.8f (vol: %.4f) | Spread: %.5f%% | Usable: $%.2f | Profit: $%.6f\n",
		timestamp,
		opp.Pair,
		opp.SpotExchange,
		opp.SpotAskPrice,
		opp.SpotAskVolume,
		opp.PerpExchange,
		opp.PerpBidPrice,
		opp.PerpBidVolume,
		opp.SpreadPct,
		opp.UsableVolumeUSD,
		estimatedProfit,
	)

	// Print to console
	// fmt.Print(logMsg)

	// Write to file
	if a.logFile != nil {
		a.logMu.Lock()
		a.logFile.WriteString(logMsg)
		a.logMu.Unlock()
	}
}

// isReliable checks if an orderbook is reliable based on latency and freshness
func isReliable(ob *OrderBook) bool {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	latencyOk := common.LessThan(ob.Latency, 200.0)
	ageMs := float64(time.Now().UnixMilli() - ob.LastUpdateTs)
	freshnessOk := common.LessThan(ageMs, 5000.0)

	return latencyOk && freshnessOk
}

// analyzeSignal performs arbitrage analysis on a single pair
// Port of the JavaScript analyzeSignal function
func (a *Analyzer) analyzeSignal(pm *PairManager) *Opportunity {
	// We're analyzing a single pair (e.g., "btc-usdt")
	// Compare spot orderbooks vs perp orderbooks across all exchanges

	pm.spotBooks.mu.RLock()
	spotExchanges := make([]string, 0, len(pm.spotBooks.OrderBooks))
	for exName := range pm.spotBooks.OrderBooks {
		spotExchanges = append(spotExchanges, exName)
	}
	pm.spotBooks.mu.RUnlock()

	pm.perpBooks.mu.RLock()
	perpExchanges := make([]string, 0, len(pm.perpBooks.OrderBooks))
	for exName := range pm.perpBooks.OrderBooks {
		perpExchanges = append(perpExchanges, exName)
	}
	pm.perpBooks.mu.RUnlock()

	// Iterate through all spot exchanges
	for _, spotExchange := range spotExchanges {
		spotOB, spotExists := pm.GetSpotOrderBook(spotExchange)
		if !spotExists || !isReliable(spotOB) {
			continue
		}

		spotBestAsk, spotAskVol, spotAskOk := spotOB.GetBestAsk()
		if !spotAskOk {
			continue
		}

		// spotAskVol is already in USDT (quantity × price)

		// Compare against all perp exchanges
		for _, perpExchange := range perpExchanges {
			// Skip if same exchange (avoid self-comparison)
			if perpExchange == spotExchange {
				continue
			}

			perpOB, perpExists := pm.GetPerpOrderBook(perpExchange)
			if !perpExists || !isReliable(perpOB) {
				continue
			}

			perpBestBid, perpBidVol, perpBidOk := perpOB.GetBestBid()
			if !perpBidOk {
				continue
			}

			// perpBidVol is already in USDT (quantity × price)

			// Target notional USD (what we want to trade)
			targetNotionalUSD := 20.0

			// Check minimum achievable volume on each side based on quantity precision
			spotMinAchievable := common.CalculateMinAchievableVolume(spotBestAsk, pm.pairName)
			perpMinAchievable := common.CalculateMinAchievableVolume(perpBestBid, pm.pairName)

			// Take the minimum volume between:
			// 1. What orderbook offers on spot side (already in USDT)
			// 2. What orderbook offers on perp side (already in USDT)
			// 3. Our target notional
			minVolume := spotAskVol
			if common.LessThan(perpBidVol, minVolume) {
				minVolume = perpBidVol
			}
			if common.LessThan(targetNotionalUSD, minVolume) {
				minVolume = targetNotionalUSD
			}

			// Check if minimum volume is achievable with quantity precision on BOTH sides
			// If volume < target AND either side can't achieve it, skip this opportunity
			if common.LessThan(minVolume, targetNotionalUSD) {
				// Volume is less than target - check if it's at least achievable
				spotCanAchieve := common.CanAchieveVolume(minVolume, spotBestAsk, pm.pairName)
				perpCanAchieve := common.CanAchieveVolume(minVolume, perpBestBid, pm.pairName)

				if !spotCanAchieve || !perpCanAchieve {
					// Can't achieve even the available volume with precision - skip
					continue
				}
			}

			// Also ensure both sides can at least achieve their minimum
			if common.LessThan(spotAskVol, spotMinAchievable) || common.LessThan(perpBidVol, perpMinAchievable) {
				continue
			}

			// Check if arbitrage exists: perp bid > spot ask
			if common.GreaterThan(perpBestBid, spotBestAsk) {
				spreadPct := ((perpBestBid - spotBestAsk) / spotBestAsk) * 100.0

				return &Opportunity{
					Pair:            pm.pairName,
					SpotExchange:    spotExchange,
					PerpExchange:    perpExchange,
					SpotAskPrice:    spotBestAsk,
					SpotAskVolume:   spotAskVol,
					PerpBidPrice:    perpBestBid,
					PerpBidVolume:   perpBidVol,
					SpreadPct:       spreadPct,
					UsableVolumeUSD: minVolume, // This is the synchronized volume to use
					Timestamp:       time.Now(),
				}
			}
		}
	}

	return nil
}
