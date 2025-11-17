# Trading Client Debug Guide

## Setup

1. Copy `.env.example` to `.env`:
   ```bash
   copy .env.example .env
   ```

2. Edit `.env` and add your Binance API credentials

3. Run the program:
   ```bash
   go run .
   ```

## Debug Logging

The system now includes comprehensive logging at every step:

### Log Prefixes:
- `[BINANCE]` - Binance client operations
- `▶️` - Starting a step
- `✅` - Success
- `❌` - Error
- `⚠️` - Warning
- `💡` - Helpful tip
- `📊` - Data/metrics
- `⏳` - Waiting/in progress

### What Gets Logged:

1. **Initialization**
   - Client creation
   - API key validation

2. **Spot Long (Buy)**
   - Symbol normalization
   - Current price fetch
   - Quantity calculation
   - Order placement
   - Order execution details
   - Position storage

3. **Futures Short (Sell)**
   - Symbol normalization
   - Current price fetch
   - Quantity calculation
   - Order placement
   - Order execution details
   - Position storage

4. **Close Spot Long**
   - Position retrieval
   - Order placement
   - Execution details
   - PnL calculation

5. **Close Futures Short**
   - Position retrieval
   - Order placement
   - Execution details
   - PnL calculation

### Example Log Output:

```
==========================================================
🚀 Starting Arbitrage Trading System
==========================================================
✅ Initialized binance client

----------------------------------------------------------
📊 Testing Trade Execution for btc-usdt with $100.00
----------------------------------------------------------

▶️  Step 1: Opening Spot Long Position...
[BINANCE] PutSpotLong - Starting spot buy for btc-usdt with $100.00 USDT
[BINANCE] PutSpotLong - Normalized symbol: BTCUSDT
[BINANCE] getSpotPrice - Fetching price from: https://api.binance.com/api/v3/ticker/price?symbol=BTCUSDT
[BINANCE] getSpotPrice - Successfully fetched price: 45000.50000000
[BINANCE] PutSpotLong - Current spot price: 45000.50000000
[BINANCE] PutSpotLong - Estimated quantity: 0.00222220
[BINANCE] PutSpotLong - Placing market buy order with params: ...
[BINANCE] signedRequest - POST https://api.binance.com/api/v3/order
...
```

## Common Issues

### API Key Not Set
```
⚠️  WARNING: BINANCE_API_KEY or BINANCE_API_SECRET not set
```
**Solution**: Create a `.env` file with your credentials

### API Error -2015: Invalid API-key
```
[BINANCE] signedRequest - ERROR: API error code=-2015
```
**Solution**: Check that your API key is correct in `.env`

### API Error -1021: Timestamp Issue
```
binance API error -1021: Timestamp for this request is outside of the recvWindow
```
**Solution**: Your system clock may be off. Sync your time.

### Insufficient Balance
```
binance API error -2010: Account has insufficient balance
```
**Solution**: Add funds to your Binance account or reduce `amountUSDT`

### Position Not Found
```
[BINANCE] CloseSpotLong - ERROR: Position not found
```
**Solution**: The open position tracking failed. Check logs for order execution issues.

## Testing Safely

1. **Start Small**: Use minimum amounts first (e.g., $10-20)
2. **Test on Testnet**: Binance has a testnet for futures trading
3. **Paper Trade**: Comment out the actual trade calls and just log what would happen
4. **Check Balances**: Always verify you have enough balance before trading

## Production Checklist

- [ ] Real API credentials in `.env`
- [ ] IP whitelist configured on Binance
- [ ] Error recovery mechanisms in place
- [ ] Position tracking database (instead of in-memory)
- [ ] Monitoring and alerts
- [ ] PnL tracking and reporting
- [ ] Circuit breakers for losses
- [ ] Rate limiting for API calls
