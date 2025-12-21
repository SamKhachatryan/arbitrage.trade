# Orderbook System

## Overview

The orderbook system connects to the signal sender via WebSocket to receive real-time orderbook updates for multiple trading pairs. Each pair has **separate WebSocket connections** for spot and perpetual markets.

## Architecture

### Key Components

1. **OrderBook** (`orderbook/types.go`)
   - Maintains bid/ask maps with price levels
   - Thread-safe with RWMutex
   - Supports merging updates (0 quantity removes level)
   - Provides best bid/ask queries and sorted snapshots

2. **PairManager** (`orderbook/manager.go`)
   - Manages orderbooks for one trading pair (spot + perp)
   - Maintains 2 WebSocket connections per pair:
     - One for `{topic: "btc-usdt"}` (spot)
     - One for `{topic: "btc-usdt-perp"}` (perpetual)
   - Auto-reconnects on connection loss
   - Decodes MessagePack binary format

3. **GlobalManager** (`orderbook/global.go`)
   - Manages all PairManager instances
   - Add/remove pairs dynamically
   - Centralized shutdown

4. **Analyzer** (`orderbook/analyzer.go`)
   - Placeholder for arbitrage detection logic
   - TODO: Port existing arbitrage analysis here
   - Currently just demonstrates data access patterns

## WebSocket Protocol

### Subscription
On connect, send JSON message:
```json
{"topic": "btc-usdt"}
```
or
```json
{"topic": "btc-usdt-perp"}
```

### Update Format (MessagePack → Decoded)

The signal sends binary MessagePack. When decoded, structure is:

```javascript
{
  "okx": [[bids_map, asks_map], latency, lastUpdateTs],
  "binance": [[bids_map, asks_map], latency, lastUpdateTs],
  // ... other exchanges
}
```

**Example (JSON representation):**
```json
{
  "okx": {
    "bids": {"2961.65": 47564.099, "2961.68": 61721.4112, ...},
    "asks": {"2961.76": 9691145.2784, "2961.77": 59.2354, ...},
    "latency": 56,
    "lastUpdateTs": 1766355904500
  }
}
```

The actual binary format uses arrays: `[[bids, asks], latency, timestamp]`

## Usage

### Initialize
```go
obManager := orderbook.NewGlobalManager("ws://185.7.81.99:4010")
```

### Add Trading Pairs
```go
err := obManager.AddPair("btc-usdt")  // Creates spot + perp connections
err := obManager.AddPair("eth-usdt")
```

### Query Orderbooks
```go
pm, exists := obManager.GetPairManager("btc-usdt")
if exists {
    spotOB, _ := pm.GetSpotOrderBook("okx")
    perpOB, _ := pm.GetPerpOrderBook("binance")
    
    bestBid, qty, ok := spotOB.GetBestBid()
    bestAsk, qty, ok := spotOB.GetBestAsk()
}
```

### Cleanup
```go
defer obManager.StopAll()  // Closes all WebSocket connections
```

## Data Flow

```
Signal Sender (ws://185.7.81.99:4010)
         │
         ├─── WS: {topic: "btc-usdt"} ────┐
         │                                  │
         └─── WS: {topic: "btc-usdt-perp"} ┤
                                            │
                                     PairManager
                                            │
                         ┌──────────────────┴──────────────────┐
                         ↓                                      ↓
                  spotBooks (map)                        perpBooks (map)
                    {"okx": OrderBook,                     {"okx": OrderBook,
                     "binance": OrderBook,                  "binance": OrderBook,
                     ...}                                   ...}
```

## Implementation Notes

### Why Separate Connections?
Each pair+market combination runs in its own goroutine with dedicated WebSocket. This provides:
- **Isolation**: One pair's connection issue doesn't affect others
- **Performance**: No shared lock contention between pairs
- **Scalability**: Easy to distribute pairs across workers

### OrderBook Updates
- Updates are **incremental**: Only changed price levels are sent
- **Quantity = 0**: Remove that price level
- **Quantity > 0**: Update/add that price level
- Maps are **unsorted** for fast updates
- Call `GetSnapshot()` for sorted bids/asks when needed

### MessagePack Parsing
The `parseExchangeData()` method handles the array format:
```go
// Input: [[bids_map, asks_map], latency, lastUpdateTs]
dataArray[0]    // [bids_map, asks_map]
dataArray[1]    // latency (float)
dataArray[2]    // lastUpdateTs (int64)
```

## TODO: Arbitrage Analysis

The current `Analyzer` component is a placeholder. To implement:

1. **Get best prices from orderbooks**
   ```go
   spotBestBid, _, _ := spotOB.GetBestBid()
   perpBestAsk, _, _ := perpOB.GetBestAsk()
   ```

2. **Calculate spreads**
   ```go
   spreadPercent := ((spotBestBid - perpBestAsk) / perpBestAsk) * 100
   ```

3. **Check thresholds** (fees, minimum profit, etc.)

4. **Trigger trades** if opportunity exists

5. **Port existing logic** from `arbitrage.go` to use orderbook data

## Testing

Monitor logs to see connections:
```
[ORDERBOOK] Starting pair manager for btc-usdt
[ORDERBOOK] Subscribed to btc-usdt
[ORDERBOOK] Subscribed to btc-usdt-perp
```

## Future Enhancements

- [ ] Add depth-of-book analysis (not just best bid/ask)
- [ ] Calculate executable volume at different price levels
- [ ] Track orderbook imbalance for prediction
- [ ] Add historical orderbook snapshots
- [ ] Implement BTreeMap for sorted price levels (or sort on-demand)
- [ ] Add metrics (update frequency, spread history, etc.)
