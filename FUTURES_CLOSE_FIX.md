# Futures Position Close Fix - Documentation

## Problem
The error `binance API error -4003: Quantity less than or equal to zero` occurred when trying to close futures short positions because:
1. The quantity was being stored in memory but may not match the actual position on Binance
2. The quantity format wasn't respecting Binance's LOT_SIZE step size requirements
3. No validation against actual exchange position state

## Solution Implemented

### 1. **Added `getFuturesPositionRisk()` Method**
Fetches the actual position from Binance API using `/fapi/v2/positionRisk` endpoint.

**What it does:**
- Queries Binance for the actual open position
- Returns position amount, entry price, unrealized PnL, and other details
- Handles cases where no position exists on the exchange

**Example Response:**
```go
PositionRisk{
    Symbol: "BTCUSDT",
    PositionAmt: -0.001,      // Negative = short position
    EntryPrice: 45000.50,
    UnrealizedProfit: -5.23,
    MarkPrice: 45523.45,
}
```

### 2. **Added `getFuturesStepSize()` Method**
Fetches the LOT_SIZE filter from Binance exchange info.

**What it does:**
- Calls `/fapi/v1/exchangeInfo` endpoint
- Parses the LOT_SIZE filter for the symbol
- Returns the minimum step size for quantity (e.g., 0.001)

**Example Step Sizes:**
- BTCUSDT: 0.001
- ETHUSDT: 0.001
- DOGEUSDT: 1.0

### 3. **Added `roundToStepSize()` Method**
Rounds quantities to comply with Binance's step size requirements.

**What it does:**
- Takes a quantity and step size
- Rounds down to the nearest valid step
- Formats to remove floating-point errors

**Example:**
```go
quantity: 0.0234567
stepSize: 0.001
result:   0.023     // Rounded down to nearest 0.001
```

### 4. **Updated `CloseFuturesShort()` Method**

**New Flow:**
1. ✅ Fetch actual position from Binance API
2. ✅ Validate position exists and amount > 0
3. ✅ Get step size from exchange info
4. ✅ Calculate absolute value of position (for shorts, it's negative)
5. ✅ Round quantity to step size
6. ✅ Validate quantity > 0
7. ✅ Place order with correct quantity
8. ✅ Clean up local position tracking

**Log Output Example:**
```
[BINANCE] CloseFuturesShort - Attempting to close futures position for btc-usdt
[BINANCE] getFuturesPositionRisk - Fetching position risk for: BTCUSDT
[BINANCE] getFuturesPositionRisk - Found position: Amt=-0.00222222, Entry=45000.50, UnrealizedPnL=-5.23
[BINANCE] CloseFuturesShort - Position from API: Amt=-0.00222222, EntryPrice=45000.50, UnrealizedPnL=-5.23
[BINANCE] getFuturesStepSize - Fetching exchange info for: BTCUSDT
[BINANCE] getFuturesStepSize - Step size for BTCUSDT: 0.00100000
[BINANCE] CloseFuturesShort - Step size for BTCUSDT: 0.00100000
[BINANCE] roundToStepSize - Original: 0.00222222, StepSize: 0.00100000, Rounded: 0.00200000
[BINANCE] CloseFuturesShort - Rounded close quantity: 0.00200000
[BINANCE] CloseFuturesShort - Placing market buy order to close short position
[BINANCE] signedRequest - POST https://fapi.binance.com/fapi/v1/order
[BINANCE] signedRequest - Response status: 200
[BINANCE] CloseFuturesShort - Order executed: OrderID=12345, Status=FILLED
[BINANCE] CloseFuturesShort - SUCCESS: Closed at 45523.45 (Entry: 45000.50, PnL: -1.16%)
```

### 5. **Updated `PutFuturesShort()` Method**

Also updated to use step size when opening positions for consistency:
- Fetches step size before placing order
- Rounds quantity to step size
- Uses `.8f` format for quantity parameter

## Benefits

✅ **Reliability**: Uses actual exchange data instead of in-memory tracking  
✅ **Compliance**: Respects Binance's quantity rules (step size)  
✅ **Error Prevention**: Validates quantity before placing order  
✅ **Debugging**: Comprehensive logging at every step  
✅ **Accuracy**: Handles floating-point precision correctly  

## API Endpoints Used

### GET /fapi/v2/positionRisk (Signed)
**Purpose**: Get current futures positions  
**Auth**: Requires API key and signature  
**Returns**: Array of all positions (filters for non-zero amounts)

### GET /fapi/v1/exchangeInfo
**Purpose**: Get trading rules and symbol information  
**Auth**: Public endpoint (no signature needed)  
**Returns**: Symbol filters including LOT_SIZE (step size)

## Error Handling

The code now handles these scenarios:

1. **No position on exchange**
   - Logs warning
   - Cleans up local tracking
   - Returns error

2. **Invalid step size**
   - Falls back to 0.001 default
   - Logs warning

3. **Quantity rounds to zero**
   - Validates before placing order
   - Returns error with details

4. **API failures**
   - Detailed error logging
   - Returns descriptive error messages

## Testing

To test the fix:

```bash
# 1. Ensure you have an open futures short position
# 2. Run the program
go run .

# The logs will show:
# - Position fetch from API
# - Step size retrieval
# - Quantity rounding
# - Successful order placement
```

## Future Improvements

Consider adding:
- [ ] Cache exchange info to reduce API calls
- [ ] Handle MIN_NOTIONAL filter (minimum order value)
- [ ] Support for position side (LONG/SHORT/BOTH in hedge mode)
- [ ] Retry logic for transient API failures
- [ ] Partial close support (close portion of position)

## Related Files Modified

- `binance_client.go` - Added helper methods and updated close logic
  - `getFuturesPositionRisk()`
  - `getFuturesStepSize()`
  - `roundToStepSize()`
  - Updated `CloseFuturesShort()`
  - Updated `PutFuturesShort()`

---

**Result**: Futures short positions can now be closed correctly with proper quantity formatting and exchange validation! 🎉
