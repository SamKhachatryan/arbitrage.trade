package main

import (
	"bytes"
	"context"
	"log"
	"os"
	"time"

	"arbitrage.trade/clients/common"
	"arbitrage.trade/orderbook"
	"arbitrage.trade/redis"
	"github.com/gorilla/websocket"
	"github.com/joho/godotenv"
	"github.com/vmihailenco/msgpack/v5"
)

var wsURL string
var orderbookSignalURL string

type PairExchange struct {
	Price        float64
	Latency      float64
	LastUpdateTs int64
}

var arbitrageThresholds = map[string]float64{
	"btc-usdt":   0.5,
	"eth-usdt":   0.6,
	"sol-usdt":   0.7,
	"doge-usdt":  0.8,
	"xrp-usdt":   0.7,
	"ton-usdt":   0.9,
	"ada-usdt":   0.6,
	"link-usdt":  0.7,
	"arb-usdt":   0.8,
	"op-usdt":    0.8,
	"ltc-usdt":   0.6,
	"bch-usdt":   0.7,
	"uni-usdt":   0.8,
	"avax-usdt":  0.8,
	"apt-usdt":   0.3,
	"near-usdt":  0.8,
	"matic-usdt": 0.7,
	"sui-usdt":   0.9,
	"icp-usdt":   0.9,
	"xvs-usdt":   1.0,
	"ach-usdt":   1.1,
	"fet-usdt":   0.9,
	"rndr-usdt":  0.8,
	"enj-usdt":   0.9,
	"cfx-usdt":   0.5,
	"kas-usdt":   0.6,
	"mina-usdt":  1.0,
	"gala-usdt":  1.1,
	"blur-usdt":  1.2,
	"wojak-usdt": 1.3,
	"bnb-usdt":   0.5,
	"mon-usdt":   1.0,
}

const riskCoef = 10.0

var supportedExchanges = map[string]bool{
	"binance":  true,
	"bitget":   true,
	"whitebit": true,
	// "gate":     true,
	"okx": true,
}

func getReliability(p PairExchange) Reliability {
	age := float64(time.Now().UnixMilli() - p.LastUpdateTs)
	switch {
	case age < 70 && p.Latency < 50:
		return UltraHigh
	case age < 120 && p.Latency < 100:
		return High
	case age < 220 && p.Latency < 200:
		return Medium
	case age < 320 && p.Latency < 300:
		return Low
	case age < 1020 && p.Latency < 1000:
		return UltraLow
	default:
		return NotReliableAtAll
	}
}

func toPairExchange(arr []interface{}) PairExchange {
	return PairExchange{
		Price:        toFloat64(arr[0]),
		Latency:      toFloat64(arr[1]),
		LastUpdateTs: toInt64(arr[2]),
	}
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Println("⚠️  No .env file found, using default values")
	}

	// Get WebSocket URL from environment variable
	orderbookSignalURL = os.Getenv("SIGNAL_WS_URL")
	if orderbookSignalURL == "" {
		orderbookSignalURL = "ws://185.7.81.99:4010" // Default to original URL
		log.Println("⚠️  SIGNAL_WS_URL not set, using default:", orderbookSignalURL)
	} else {
		log.Println("📡 Using Signal WebSocket URL:", orderbookSignalURL)
	}

	// Use the same URL for both (backward compatibility)
	wsURL = orderbookSignalURL

	// Initialize Redis for trade notifications
	if err := redis.InitRedis(); err != nil {
		log.Println("⚠️  Redis unavailable - trade notifications disabled")
	}
	defer redis.CloseRedis()

	// Initialize global orderbook manager
	log.Println("📊 Initializing orderbook manager...")
	obManager := orderbook.NewGlobalManager(orderbookSignalURL)

	// Add trading pairs to monitor
	tradingPairs := []string{
		// "btc-usdt",
		// "eth-usdt",
		// "sol-usdt",
		// "doge-usdt",
		"xrp-usdt",
		"ton-usdt",
		"ada-usdt",
		// "bnb-usdt",
		"trx-usdt",
		"avax-usdt",
		// "mon-usdt",
	}

	for _, pair := range tradingPairs {
		log.Printf("📈 Adding pair: %s (spot + perp)", pair)
		if err := obManager.AddPair(pair); err != nil {
			log.Printf("❌ Failed to add pair %s: %v", pair, err)
		}
	}

	log.Println("✅ Orderbook manager started for all pairs")
	log.Println("💡 Each pair has separate WebSocket connections for spot and perpetual")

	// Initialize the arbitrage analyzer with supported exchanges
	log.Println("🔍 Initializing arbitrage analyzer...")
	analyzer := orderbook.NewAnalyzer(obManager, supportedExchanges)
	obManager.SetAnalyzer(analyzer)
	defer analyzer.Close()

	// Set global analyzer reference for resetting execution flag after trades
	globalAnalyzer = analyzer

	// Set up price update callback for position tracking
	analyzer.SetPriceUpdateCallback(func(pairName string, shortExchange string, shortPrice float64, longExchange string, longPrice float64) {
		UpdatePrices(pairName, shortExchange, shortPrice, longExchange, longPrice)
	})

	// Set up execution callback for live trading
	analyzer.SetExecutionCallback(func(ctx context.Context, opp *orderbook.Opportunity) bool {
		log.Printf("🚀 EXECUTING TRADE: %s | Spot: %s @ $%.6f | Perp: %s @ $%.6f | Spread: %.2f%% | Volume: $%.2f",
			opp.Pair, opp.SpotExchange, opp.SpotAskPrice, opp.PerpExchange, opp.PerpBidPrice, opp.SpreadPct, opp.UsableVolumeUSD)

		// Execute the arbitrage trade
		// Buy spot (long), sell perp (short)
		ConsiderArbitrageOpportunity(
			ctx,
			common.ExchangeType(opp.PerpExchange), // Short exchange (sell perp)
			opp.PerpBidPrice,                      // Short price
			common.ExchangeType(opp.SpotExchange), // Long exchange (buy spot)
			opp.SpotAskPrice,                      // Long price
			opp.Pair,
			opp.SpreadPct,
			opp.UsableVolumeUSD, // Use the synchronized volume from orderbook analysis
		)

		return true // Trade executed successfully
	})

	log.Println("✅ Analyzer enabled - will analyze on each signal update and execute trades (spread >= 0.5%)")
	log.Println("📝 Logging all opportunities to opportunities.log file")
	log.Println("⚠️  Program will terminate after executing one trade")

	// TODO: Add periodic analyzer that checks all orderbooks for arbitrage opportunities
	// For now, we'll keep the old WebSocket connection for backward compatibility

	// ctx := context.Context()

	// Safety flag to ensure only ONE arbitrage cycle is executed during testing
	// var executedOnce bool
	// var executionMutex sync.Mutex

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		log.Fatal("WebSocket dial error:", err)
	}
	defer conn.Close()

	conn.SetReadLimit(1 << 20)

	// Cleanup on exit
	defer obManager.StopAll()

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			log.Println("Read error:", err)
			break
		}

		var parsed map[string]interface{}

		if err := msgpack.NewDecoder(bytes.NewReader(data)).Decode(&parsed); err != nil {
			log.Println("Decode error:", err)
			continue
		}

		// TODO: Remove this old processing once we fully migrate to orderbook-based analysis
		// for pairName, val := range parsed {
		// 	if len(pairName) > 5 && pairName[len(pairName)-5:] == "-perp" {
		// 		continue
		// 	}

		// 	spotMap := val.(map[string]interface{})
		// 	perpMapRaw, ok := parsed[pairName+"-perp"]
		// 	if !ok {
		// 		continue
		// 	}
		// 	perpMap := perpMapRaw.(map[string]interface{})

		// 	for ex1, v1 := range spotMap {
		// 		longExchange := toPairExchange(v1.([]interface{}))
		// 		for ex2, v2 := range perpMap {
		// 			if ex1 == ex2 {
		// 				continue
		// 			}
		// 			shortExchange := toPairExchange(v2.([]interface{}))

		// 			UpdatePrices(pairName, ex2, shortExchange.Price, ex1, longExchange.Price)

		// 			if longExchange.Price > shortExchange.Price {
		// 				continue
		// 			}

		// 			high := shortExchange.Price
		// 			low := longExchange.Price
		// 			if common.IsZero(low) || common.IsZero(high) {
		// 				continue
		// 			}
		// 			diff := ((high - low) / low) * 100.0

		// 			// Update active positions with current prices
		// 			UpdatePrices(pairName, ex2, high, ex1, low)

		// 			threshold := arbitrageThresholds[pairName] / riskCoef

		// 			if common.GreaterThanOrEqual(diff, threshold) {
		// 				r1 := getReliability(longExchange)
		// 				r2 := getReliability(shortExchange)
		// 				if r1 >= NotReliableAtAll && r2 >= NotReliableAtAll {
		// 					buyEx := ex1
		// 					sellEx := ex2

		// 					fmt.Printf("%s %s %f\n", buyEx, sellEx, diff)

		// 					// Require minimum 0.5% spread to cover fees and make profit
		// 					// Typical fees: 0.1% x 2 legs x 2 trades = 0.4% minimum
		// 					// log.Printf("%.2f%% \n", diff)
		// 					if supportedExchanges[buyEx] && supportedExchanges[sellEx] && common.GreaterThanOrEqual(diff, 0.1) {
		// 						// executionMutex.Lock()
		// 						// if executedOnce {
		// 						// 	executionMutex.Unlock()
		// 						// 	continue
		// 						// }

		// 						// executedOnce = true
		// 						// executionMutex.Unlock()

		// 						fmt.Println("---------------------")
		// 						fmt.Printf("Cheaper   - %s (%f)\n", ex1, low)
		// 						fmt.Printf("Expensive - %s (%f)\n", ex2, high)
		// 						fmt.Printf("Pair      - %s \n", pairName)
		// 						fmt.Printf("Diff      - %.2f%% \n", diff)

		// 						// ConsiderArbitrageOpportunity(ctx, common.ExchangeType(ex2), high, common.ExchangeType(ex1), low, pairName, diff, 10.0)
		// 						// Don't return - keep monitoring for exit conditions
		// 						// return
		// 					} else if common.GreaterThan(diff, 0.1) {
		// 						// fmt.Println("---------------------")
		// 						// fmt.Printf("Short on - %s (%f)\n", ex2, high)
		// 						// fmt.Printf("Buy on   - %s (%f)\n", ex1, low)
		// 						// fmt.Printf("Pair     - %s \n", pairName)
		// 						// fmt.Printf("Diff     - %.2f%% \n", diff)
		// 					}
		// 				}
		// 			}
		// 		}
		// 	}
		// }
	}
}
