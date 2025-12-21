package orderbook

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"arbitrage.trade/clients/common"
	"github.com/gorilla/websocket"
	"github.com/vmihailenco/msgpack/v5"
)

// SignalUpdate represents the raw update from the signal sender
type SignalUpdate struct {
	ExchangeName string
	Bids         map[float64]float64
	Asks         map[float64]float64
	Latency      float64
	LastUpdateTs int64
}

// PairManager manages orderbooks and WebSocket connections for a trading pair
type PairManager struct {
	pairName    string
	perpName    string
	signalURL   string
	spotBooks   *ExchangeOrderBooks
	perpBooks   *ExchangeOrderBooks
	spotConn    *websocket.Conn
	perpConn    *websocket.Conn
	mu          sync.Mutex
	ctx         context.Context
	cancel      context.CancelFunc
	reconnectMu sync.Mutex
	analyzer    *Analyzer // Analyzer to trigger on updates
}

// NewPairManager creates a new manager for a trading pair
func NewPairManager(pairName, signalURL string) *PairManager {
	ctx, cancel := context.WithCancel(context.Background())
	perpName := pairName + "-perp"

	return &PairManager{
		pairName:  pairName,
		perpName:  perpName,
		signalURL: signalURL,
		spotBooks: NewExchangeOrderBooks(),
		perpBooks: NewExchangeOrderBooks(),
		ctx:       ctx,
		cancel:    cancel,
	}
}

// SetAnalyzer sets the analyzer for this pair manager
func (pm *PairManager) SetAnalyzer(analyzer *Analyzer) {
	pm.analyzer = analyzer
}

// Start begins listening to orderbook updates for both spot and perpetual
func (pm *PairManager) Start() error {
	log.Printf("[ORDERBOOK] Starting pair manager for %s", pm.pairName)

	// Start spot connection
	go pm.maintainConnection(pm.pairName, true)

	// Start perpetual connection
	go pm.maintainConnection(pm.perpName, false)

	// Start periodic orderbook printer (every 10 seconds)
	go pm.printOrderbookPeriodically(10 * time.Second)

	return nil
}

// Stop closes all connections and stops the manager
func (pm *PairManager) Stop() {
	log.Printf("[ORDERBOOK] Stopping pair manager for %s", pm.pairName)
	pm.cancel()

	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pm.spotConn != nil {
		pm.spotConn.Close()
	}
	if pm.perpConn != nil {
		pm.perpConn.Close()
	}
}

// maintainConnection maintains a WebSocket connection with auto-reconnect
func (pm *PairManager) maintainConnection(topic string, isSpot bool) {
	for {
		select {
		case <-pm.ctx.Done():
			return
		default:
			err := pm.connectAndListen(topic, isSpot)
			if err != nil {
				log.Printf("[ORDERBOOK] Connection error for %s: %v. Reconnecting in 5s...", topic, err)
				time.Sleep(5 * time.Second)
			}
		}
	}
}

// connectAndListen establishes connection and listens for updates
func (pm *PairManager) connectAndListen(topic string, isSpot bool) error {
	conn, _, err := websocket.DefaultDialer.Dial(pm.signalURL, nil)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer conn.Close()

	// Store connection reference
	pm.mu.Lock()
	if isSpot {
		pm.spotConn = conn
	} else {
		pm.perpConn = conn
	}
	pm.mu.Unlock()

	// Subscribe to topic
	subscribeMsg := map[string]string{"topic": topic}
	if err := conn.WriteJSON(subscribeMsg); err != nil {
		return fmt.Errorf("failed to subscribe: %w", err)
	}

	log.Printf("[ORDERBOOK] Subscribed to %s", topic)

	// Listen for updates
	for {
		select {
		case <-pm.ctx.Done():
			return nil
		default:
			_, message, err := conn.ReadMessage()
			if err != nil {
				return fmt.Errorf("read error: %w", err)
			}

			if err := pm.processMessage(message, isSpot); err != nil {
				log.Printf("[ORDERBOOK] Error processing message for %s: %v", topic, err)
			}
		}
	}
}

// processMessage decodes and processes a MessagePack update
func (pm *PairManager) processMessage(message []byte, isSpot bool) error {
	// Decode MessagePack - always comes in unified state format:
	// {
	//   "pair-name": {
	//     "exchange1": [[bids, asks], latency, timestamp],
	//     "exchange2": [[bids, asks], latency, timestamp]
	//   }
	// }
	// This structure is used for scalability - signal can send 1 pair or 100 pairs
	// using the same format, and we just deep merge into our state

	var rawData map[string]interface{}
	dec := msgpack.NewDecoder(bytes.NewReader(message))
	if err := dec.Decode(&rawData); err != nil {
		return fmt.Errorf("failed to decode msgpack: %w", err)
	}

	// Iterate through pairs in the update (usually just one for single subscription)
	for _, pairValue := range rawData {
		exchangesData, ok := pairValue.(map[string]interface{})
		if !ok {
			continue
		}

		// Process each exchange in this pair's data
		for exchangeName, exchangeData := range exchangesData {
			update, err := pm.parseExchangeData(exchangeName, exchangeData)
			if err != nil {
				continue
			}

			// Update the appropriate orderbook (spot or perp)
			books := pm.spotBooks
			if !isSpot {
				books = pm.perpBooks
			}

			ob := books.GetOrCreate(exchangeName)
			ob.Update(update.Bids, update.Asks, update.Latency, update.LastUpdateTs)
		}
	}

	// Trigger analysis after processing updates
	if pm.analyzer != nil {
		pm.analyzer.AnalyzePair(pm.pairName)
	}

	return nil
} // parseExchangeData converts the array format to SignalUpdate
func (pm *PairManager) parseExchangeData(exchangeName string, data interface{}) (*SignalUpdate, error) {
	// Data format: [[bids_map, asks_map], latency, lastUpdateTs]
	dataArray, ok := data.([]interface{})
	if !ok || len(dataArray) < 3 {
		return nil, fmt.Errorf("invalid data format")
	}

	// Parse orderbook data [bids, asks]
	obData, ok := dataArray[0].([]interface{})
	if !ok || len(obData) < 2 {
		return nil, fmt.Errorf("invalid orderbook format")
	}

	// Parse bids
	bids, err := pm.parseOrderBookSide(obData[0])
	if err != nil {
		return nil, fmt.Errorf("failed to parse bids: %w", err)
	}

	// Parse asks
	asks, err := pm.parseOrderBookSide(obData[1])
	if err != nil {
		return nil, fmt.Errorf("failed to parse asks: %w", err)
	}

	// Parse latency
	latency := common.ToFloat64(dataArray[1])

	// Parse lastUpdateTs
	lastUpdateTs := common.ToInt64(dataArray[2])

	return &SignalUpdate{
		ExchangeName: exchangeName,
		Bids:         bids,
		Asks:         asks,
		Latency:      latency,
		LastUpdateTs: lastUpdateTs,
	}, nil
}

// parseOrderBookSide converts map[string]interface{} to map[float64]float64
func (pm *PairManager) parseOrderBookSide(data interface{}) (map[float64]float64, error) {
	result := make(map[float64]float64)

	// Try map[interface{}]interface{} first (MessagePack format)
	if dataMap, ok := data.(map[interface{}]interface{}); ok {
		for k, v := range dataMap {
			// Parse price key
			var price float64
			switch p := k.(type) {
			case string:
				price, _ = strconv.ParseFloat(p, 64)
			case float64:
				price = p
			case float32:
				price = float64(p)
			case int:
				price = float64(p)
			case int64:
				price = float64(p)
			default:
				// Try to convert to string and parse
				priceStr := fmt.Sprintf("%v", p)
				price, _ = strconv.ParseFloat(priceStr, 64)
			}

			// Parse quantity value
			qty := common.ToFloat64(v)
			if price > 0 { // Only add valid prices
				result[price] = qty
			}
		}
		return result, nil
	}

	// Try map[string]interface{} (alternative format)
	if dataMap, ok := data.(map[string]interface{}); ok {
		for k, v := range dataMap {
			price, _ := strconv.ParseFloat(k, 64)
			qty := common.ToFloat64(v)
			if price > 0 {
				result[price] = qty
			}
		}
		return result, nil
	}

	// Empty map is ok
	return result, nil
}

// GetSpotOrderBook returns the spot orderbook for an exchange
func (pm *PairManager) GetSpotOrderBook(exchangeName string) (*OrderBook, bool) {
	return pm.spotBooks.GetOrderBook(exchangeName)
}

// GetPerpOrderBook returns the perpetual orderbook for an exchange
func (pm *PairManager) GetPerpOrderBook(exchangeName string) (*OrderBook, bool) {
	return pm.perpBooks.GetOrderBook(exchangeName)
}

// printOrderbookPeriodically prints the orderbook state as JSON every interval
func (pm *PairManager) printOrderbookPeriodically(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-pm.ctx.Done():
			return
		case <-ticker.C:
			pm.printOrderbookSnapshot()
		}
	}
}

// printOrderbookSnapshot prints current orderbook state as JSON
func (pm *PairManager) printOrderbookSnapshot() {
	type OrderbookSummary struct {
		BestBid   float64 `json:"best_bid"`
		BestAsk   float64 `json:"best_ask"`
		Spread    float64 `json:"spread_pct"`
		BidLevels int     `json:"bid_levels"`
		AskLevels int     `json:"ask_levels"`
		Latency   float64 `json:"latency"`
	}

	type OrderbookSnapshot struct {
		Pair      string                      `json:"pair"`
		Timestamp time.Time                   `json:"timestamp"`
		Spot      map[string]OrderbookSummary `json:"spot"`
		Perp      map[string]OrderbookSummary `json:"perp"`
	}

	snapshot := OrderbookSnapshot{
		Pair:      pm.pairName,
		Timestamp: time.Now(),
		Spot:      make(map[string]OrderbookSummary),
		Perp:      make(map[string]OrderbookSummary),
	}

	// Collect spot data
	pm.spotBooks.mu.RLock()
	for exName, ob := range pm.spotBooks.OrderBooks {
		bestBid, _, bidOk := ob.GetBestBid()
		bestAsk, _, askOk := ob.GetBestAsk()

		if bidOk && askOk {
			spread := ((bestAsk - bestBid) / bestBid) * 100
			snapshot.Spot[exName] = OrderbookSummary{
				BestBid:   bestBid,
				BestAsk:   bestAsk,
				Spread:    spread,
				BidLevels: len(ob.Bids),
				AskLevels: len(ob.Asks),
				Latency:   ob.Latency,
			}
		}
	}
	pm.spotBooks.mu.RUnlock()

	// Collect perp data
	pm.perpBooks.mu.RLock()
	for exName, ob := range pm.perpBooks.OrderBooks {
		bestBid, _, bidOk := ob.GetBestBid()
		bestAsk, _, askOk := ob.GetBestAsk()

		if bidOk && askOk {
			spread := ((bestAsk - bestBid) / bestBid) * 100
			snapshot.Perp[exName] = OrderbookSummary{
				BestBid:   bestBid,
				BestAsk:   bestAsk,
				Spread:    spread,
				BidLevels: len(ob.Bids),
				AskLevels: len(ob.Asks),
				Latency:   ob.Latency,
			}
		}
	}
	pm.perpBooks.mu.RUnlock()
}

// AnalyzeArbitrage performs arbitrage analysis on the orderbooks
// TODO: Implement arbitrage detection logic here
func (pm *PairManager) AnalyzeArbitrage() {
	// This is where we'll implement the arbitrage detection logic
	// For now, just log that we have data

	// Example: Get best bid/ask from each exchange for spot
	pm.spotBooks.mu.RLock()
	spotExchanges := make([]string, 0, len(pm.spotBooks.OrderBooks))
	for exName := range pm.spotBooks.OrderBooks {
		spotExchanges = append(spotExchanges, exName)
	}
	pm.spotBooks.mu.RUnlock()

	// Example: Get best bid/ask from each exchange for perp
	pm.perpBooks.mu.RLock()
	perpExchanges := make([]string, 0, len(pm.perpBooks.OrderBooks))
	for exName := range pm.perpBooks.OrderBooks {
		perpExchanges = append(perpExchanges, exName)
	}
	pm.perpBooks.mu.RUnlock()

	// TODO: Compare spot vs perp prices across exchanges
	// TODO: Calculate potential arbitrage opportunities
	// TODO: Check if spread is sufficient after fees
	// TODO: Trigger trades if opportunity exists

	_ = spotExchanges
	_ = perpExchanges
}
