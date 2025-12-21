package orderbook

// Example usage of the orderbook system
// Uncomment this code in main.go to enable periodic analysis

/*
import (
	"time"
	"arbitrage.trade/orderbook"
)

func runOrderbookAnalyzer(obManager *orderbook.GlobalManager) {
	// Create analyzer that runs every 500ms
	analyzer := orderbook.NewAnalyzer(obManager, 500*time.Millisecond)

	// Start the analyzer
	analyzer.Start()

	// Cleanup on exit
	defer analyzer.Stop()

	log.Println("🔍 Orderbook analyzer started (500ms interval)")
}

// In main():
// go runOrderbookAnalyzer(obManager)
*/

/*
// Example: Query orderbooks manually
func exampleQueryOrderbooks(obManager *orderbook.GlobalManager) {
	pm, exists := obManager.GetPairManager("btc-usdt")
	if !exists {
		return
	}

	// Get spot orderbook for OKX
	spotOB, ok := pm.GetSpotOrderBook("okx")
	if ok {
		bestBid, bidQty, _ := spotOB.GetBestBid()
		bestAsk, askQty, _ := spotOB.GetBestAsk()

		log.Printf("[OKX SPOT] BTC-USDT: Bid=%.2f (%.4f), Ask=%.2f (%.4f), Spread=%.4f%%",
			bestBid, bidQty, bestAsk, askQty,
			((bestAsk-bestBid)/bestBid)*100)
	}

	// Get perp orderbook for Binance
	perpOB, ok := pm.GetPerpOrderBook("binance")
	if ok {
		bestBid, bidQty, _ := perpOB.GetBestBid()
		bestAsk, askQty, _ := perpOB.GetBestAsk()

		log.Printf("[BINANCE PERP] BTC-USDT: Bid=%.2f (%.4f), Ask=%.2f (%.4f), Spread=%.4f%%",
			bestBid, bidQty, bestAsk, askQty,
			((bestAsk-bestBid)/bestBid)*100)
	}

	// Get full sorted orderbook snapshot
	if spotOB != nil {
		bids, asks, timestamp := spotOB.GetSnapshot()
		log.Printf("Snapshot at %s: %d bids, %d asks",
			timestamp.Format("15:04:05.000"), len(bids), len(asks))

		// Print top 5 bids
		for i := 0; i < 5 && i < len(bids); i++ {
			log.Printf("  Bid[%d]: %.2f @ %.4f", i, bids[i].Price, bids[i].Quantity)
		}

		// Print top 5 asks
		for i := 0; i < 5 && i < len(asks); i++ {
			log.Printf("  Ask[%d]: %.2f @ %.4f", i, asks[i].Price, asks[i].Quantity)
		}
	}
}
*/

/*
// Example: Calculate cross-exchange arbitrage
func exampleCrossExchangeArbitrage(obManager *orderbook.GlobalManager) {
	pm, exists := obManager.GetPairManager("btc-usdt")
	if !exists {
		return
	}

	exchanges := []string{"okx", "binance", "bitget", "gate"}

	for _, spotEx := range exchanges {
		spotOB, spotOk := pm.GetSpotOrderBook(spotEx)
		if !spotOk {
			continue
		}

		spotBid, _, bidOk := spotOB.GetBestBid()
		spotAsk, _, askOk := spotOB.GetBestAsk()
		if !bidOk || !askOk {
			continue
		}

		for _, perpEx := range exchanges {
			if perpEx == spotEx {
				continue // Skip same exchange
			}

			perpOB, perpOk := pm.GetPerpOrderBook(perpEx)
			if !perpOk {
				continue
			}

			perpBid, _, perpBidOk := perpOB.GetBestBid()
			perpAsk, _, perpAskOk := perpOB.GetBestAsk()
			if !perpBidOk || !perpAskOk {
				continue
			}

			// Opportunity 1: Buy spot, sell perp
			spread1 := ((perpBid - spotAsk) / spotAsk) * 100
			if spread1 > 0.5 { // Threshold
				log.Printf("[ARB] Buy %s spot @ %.2f, Sell %s perp @ %.2f, Spread: %.2f%%",
					spotEx, spotAsk, perpEx, perpBid, spread1)
			}

			// Opportunity 2: Sell spot, buy perp
			spread2 := ((spotBid - perpAsk) / perpAsk) * 100
			if spread2 > 0.5 { // Threshold
				log.Printf("[ARB] Sell %s spot @ %.2f, Buy %s perp @ %.2f, Spread: %.2f%%",
					spotEx, spotBid, perpEx, perpAsk, spread2)
			}
		}
	}
}
*/
