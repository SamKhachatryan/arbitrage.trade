package main

import (
	"context"
	"log"
	"time"

	"arbitrage.trade/clients"
	"github.com/joho/godotenv"
)

var wsURL = "ws://185.7.81.99:4010"

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
	"pepe-usdt":  1.2,
	"floki-usdt": 1.3,
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
}

const riskCoef = 4.0

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
	// Load environment variables from .env file
	err := godotenv.Load()
	if err != nil {
		log.Println("⚠️  No .env file found, using default values")
	}

	/////// BINANCE

	// apiKey := os.Getenv("BINANCE_API_KEY")
	// apiSecret := os.Getenv("BINANCE_API_SECRET")

	// if apiKey == "" || apiSecret == "" {
	// 	log.Println("⚠️  WARNING: BINANCE_API_KEY or BINANCE_API_SECRET not set in environment")
	// 	log.Println("⚠️  Using placeholder credentials - API calls will fail")
	// 	apiKey = "your-api-key"
	// 	apiSecret = "your-api-secret"
	// }

	// Initialize Binance client
	ctx := context.Background()
	// binanceClient := clients.NewBinanceClient(apiKey, apiSecret)

	// Test parameters
	// pairName := "arb-usdt"
	// amountUSDT := 10.0

	// Step 1: Open spot long position
	// log.Println("[BINANCE] ▶️  Step 1: Opening Spot Long Position...")
	// _, err = binanceClient.PutSpotLong(ctx, pairName, amountUSDT)
	// if err != nil {
	// 	log.Printf("❌ Failed to open spot long: %v", err)
	// 	log.Println("💡 Make sure your API keys are correct and have trading permissions")
	// 	return
	// }

	ConsiderArbitrageOpportunity(ctx, clients.Binance, 0.236300, clients.Bitget, 0.236800, "doge-usdt", 0.21, 10.0)

	// var wg sync.WaitGroup
	// wg.Add(4)

	// go func() {
	// 	defer wg.Done()
	// 	clients.Execute(ctx, clients.Binance, clients.PutSpotLong, pairName, amountUSDT)
	// 	clients.Execute(ctx, clients.Binance, clients.CloseSpotLong, pairName, amountUSDT)
	// }()

	// go func() {
	// 	defer wg.Done()
	// 	clients.Execute(ctx, clients.Binance, clients.PutFuturesShort, pairName, amountUSDT)
	// 	clients.Execute(ctx, clients.Binance, clients.CloseFuturesShort, pairName, amountUSDT)
	// }()

	// go func() {
	// 	defer wg.Done()
	// 	clients.Execute(ctx, clients.Bitget, clients.PutSpotLong, pairName, amountUSDT)
	// 	clients.Execute(ctx, clients.Bitget, clients.CloseSpotLong, pairName, amountUSDT)
	// }()

	// go func() {
	// 	defer wg.Done()
	// 	clients.Execute(ctx, clients.Bitget, clients.PutFuturesShort, pairName, amountUSDT)
	// 	clients.Execute(ctx, clients.Bitget, clients.CloseFuturesShort, pairName, amountUSDT)
	// }()

	// wg.Wait()

	// Note: The websocket arbitrage detection code is commented out below

	// conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	// if err != nil {
	// 	log.Fatal("WebSocket dial error:", err)
	// }
	// defer conn.Close()

	// conn.SetReadLimit(1 << 20)

	// for {
	// 	_, data, err := conn.ReadMessage()
	// 	if err != nil {
	// 		log.Println("Read error:", err)
	// 		break
	// 	}

	// 	var parsed map[string]interface{}
	// 	if err := msgpack.NewDecoder(bytes.NewReader(data)).Decode(&parsed); err != nil {
	// 		log.Println("Decode error:", err)
	// 		continue
	// 	}

	// 	for pairName, val := range parsed {
	// 		if len(pairName) > 5 && pairName[len(pairName)-5:] == "-perp" {
	// 			continue
	// 		}

	// 		spotMap := val.(map[string]interface{})
	// 		perpMapRaw, ok := parsed[pairName+"-perp"]
	// 		if !ok {
	// 			continue
	// 		}
	// 		perpMap := perpMapRaw.(map[string]interface{})

	// 		for ex1, v1 := range spotMap {
	// 			p1 := toPairExchange(v1.([]interface{}))
	// 			for ex2, v2 := range perpMap {
	// 				if ex1 == ex2 {
	// 					continue
	// 				}
	// 				p2 := toPairExchange(v2.([]interface{}))

	// 				high := math.Max(p1.Price, p2.Price)
	// 				low := math.Min(p1.Price, p2.Price)
	// 				if low == 0 {
	// 					continue
	// 				}
	// 				diff := ((high - low) / low) * 100.0
	// 				threshold := arbitrageThresholds[pairName] / riskCoef

	// 				if diff >= threshold {
	// 					r1 := getReliability(p1)
	// 					r2 := getReliability(p2)
	// 					if r1 > Low && r2 > Low {
	// 						buyEx := ex1
	// 						sellEx := ex2
	// 						if p1.Price > p2.Price {
	// 							buyEx, sellEx = ex2, ex1
	// 						}

	// 						fmt.Println("---------------------")
	// 						fmt.Printf("Short on - %s (%f)\n", sellEx, low)
	// 						fmt.Printf("Buy on   - %s (%f)\n", buyEx, high)
	// 						fmt.Printf("Pair     - %s \n", pairName)
	// 						fmt.Printf("Diff     - %.2f%% \n", diff)
	// 						// if (buyEx == "whitebit" && sellEx == "bitget") || (buyEx == "bitget" && sellEx == "whitebit") {
	// 						// fmt.Printf("Arbitrage opportunity (%s): Buy on %s at %f, Sell on %s at %f, Diff: %.2f%% %s \n",
	// 						// pairName, buyEx, low, sellEx, high, diff, time.Now().Format("20060102150405"))
	// 						// }
	// 					}
	// 				}
	// 			}
	// 		}
	// 	}
	// }
}
