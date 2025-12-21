package orderbook

import (
	"sync"
	"time"
)

// PriceLevel represents a single price level in the orderbook
type PriceLevel struct {
	Price    float64
	Quantity float64
}

// OrderBook represents the current state of bids and asks for an exchange
type OrderBook struct {
	mu           sync.RWMutex
	Bids         map[float64]float64 // price -> quantity
	Asks         map[float64]float64 // price -> quantity
	Latency      float64
	LastUpdateTs int64
}

// NewOrderBook creates a new empty orderbook
func NewOrderBook() *OrderBook {
	return &OrderBook{
		Bids: make(map[float64]float64),
		Asks: make(map[float64]float64),
	}
}

// Update merges new data into the orderbook
func (ob *OrderBook) Update(bids, asks map[float64]float64, latency float64, lastUpdateTs int64) {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	// Update bids - remove if quantity is 0, otherwise update
	for price, qty := range bids {
		if qty == 0 {
			delete(ob.Bids, price)
		} else {
			ob.Bids[price] = qty
		}
	}

	// Update asks - remove if quantity is 0, otherwise update
	for price, qty := range asks {
		if qty == 0 {
			delete(ob.Asks, price)
		} else {
			ob.Asks[price] = qty
		}
	}

	ob.Latency = latency
	ob.LastUpdateTs = lastUpdateTs
}

// GetBestBid returns the highest bid price
func (ob *OrderBook) GetBestBid() (float64, float64, bool) {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	if len(ob.Bids) == 0 {
		return 0, 0, false
	}

	bestPrice := 0.0
	bestQty := 0.0
	for price, qty := range ob.Bids {
		if price > bestPrice {
			bestPrice = price
			bestQty = qty
		}
	}
	return bestPrice, bestQty, true
}

// GetBestAsk returns the lowest ask price
func (ob *OrderBook) GetBestAsk() (float64, float64, bool) {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	if len(ob.Asks) == 0 {
		return 0, 0, false
	}

	bestPrice := -1.0
	bestQty := 0.0
	for price, qty := range ob.Asks {
		if bestPrice < 0 || price < bestPrice {
			bestPrice = price
			bestQty = qty
		}
	}
	return bestPrice, bestQty, true
}

// GetSnapshot returns sorted bids and asks
func (ob *OrderBook) GetSnapshot() ([]PriceLevel, []PriceLevel, time.Time) {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	// Convert to slices
	bids := make([]PriceLevel, 0, len(ob.Bids))
	for price, qty := range ob.Bids {
		bids = append(bids, PriceLevel{Price: price, Quantity: qty})
	}

	asks := make([]PriceLevel, 0, len(ob.Asks))
	for price, qty := range ob.Asks {
		asks = append(asks, PriceLevel{Price: price, Quantity: qty})
	}

	// Sort bids (highest first)
	for i := 0; i < len(bids); i++ {
		for j := i + 1; j < len(bids); j++ {
			if bids[j].Price > bids[i].Price {
				bids[i], bids[j] = bids[j], bids[i]
			}
		}
	}

	// Sort asks (lowest first)
	for i := 0; i < len(asks); i++ {
		for j := i + 1; j < len(asks); j++ {
			if asks[j].Price < asks[i].Price {
				asks[i], asks[j] = asks[j], asks[i]
			}
		}
	}

	timestamp := time.UnixMilli(ob.LastUpdateTs)
	return bids, asks, timestamp
}

// ExchangeOrderBooks holds orderbooks for all exchanges for a single pair
type ExchangeOrderBooks struct {
	mu         sync.RWMutex
	OrderBooks map[string]*OrderBook // exchange name -> orderbook
}

// NewExchangeOrderBooks creates a new container for exchange orderbooks
func NewExchangeOrderBooks() *ExchangeOrderBooks {
	return &ExchangeOrderBooks{
		OrderBooks: make(map[string]*OrderBook),
	}
}

// GetOrCreate returns the orderbook for an exchange, creating if needed
func (eob *ExchangeOrderBooks) GetOrCreate(exchangeName string) *OrderBook {
	eob.mu.Lock()
	defer eob.mu.Unlock()

	if ob, exists := eob.OrderBooks[exchangeName]; exists {
		return ob
	}

	ob := NewOrderBook()
	eob.OrderBooks[exchangeName] = ob
	return ob
}

// GetOrderBook returns the orderbook for an exchange if it exists
func (eob *ExchangeOrderBooks) GetOrderBook(exchangeName string) (*OrderBook, bool) {
	eob.mu.RLock()
	defer eob.mu.RUnlock()

	ob, exists := eob.OrderBooks[exchangeName]
	return ob, exists
}
