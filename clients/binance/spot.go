package binance

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"arbitrage.trade/clients/common"
)

func (b *BinanceClient) getSpotBalance(ctx context.Context, asset string) (float64, error) {
	params := url.Values{}
	params.Set("timestamp", strconv.FormatInt(time.Now().UnixMilli(), 10))

	var accountInfo AccountInfo
	err := b.signedRequest(ctx, "GET", b.spotBaseURL+"/api/v3/account", params, &accountInfo)
	if err != nil {
		log.Printf("[BINANCE] getSpotBalance - ERROR: Request failed: %v", err)
		return 0, err
	}

	// Find the balance for the asset
	for _, balance := range accountInfo.Balances {
		if balance.Asset == asset {
			free, _ := strconv.ParseFloat(balance.Free, 64)
			// Return free balance (available to sell)
			return free, nil
		}
	}

	return 0, nil
}

func (b *BinanceClient) getSpotPrice(symbol string) (float64, error) {
	url := fmt.Sprintf("%s/api/v3/ticker/price?symbol=%s", b.spotBaseURL, symbol)

	resp, err := http.Get(url)
	if err != nil {
		log.Printf("[BINANCE] getSpotPrice - ERROR: HTTP request failed: %v", err)
		return 0, err
	}
	defer resp.Body.Close()

	var result struct {
		Price string `json:"price"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("[BINANCE] getSpotPrice - ERROR: JSON decode failed: %v", err)
		return 0, err
	}

	price, err := strconv.ParseFloat(result.Price, 64)
	if err != nil {
		log.Printf("[BINANCE] getSpotPrice - ERROR: Price parse failed: %v", err)
		return 0, err
	}

	return price, nil
}

// BinanceClient implements ExchangeTradeClient for Binance
func (b *BinanceClient) PutSpotLong(ctx context.Context, pairName string, amountUSDT float64) (*common.TradeResult, error) {
	symbol := b.normalizePairName(pairName, false)
	_, err := b.getSpotPrice(symbol)
	if err != nil {
		log.Printf("[BINANCE] PutSpotLong - ERROR: Failed to get spot price: %v", err)
		return nil, fmt.Errorf("failed to get spot price: %w", err)
	}

	balance, err := b.getSpotBalance(ctx, "USDT")
	if err != nil {
		log.Printf("[BINANCE] PutSpotLong - ERROR: Failed to get USDT balance: %v", err)
		return nil, fmt.Errorf("failed to get USDT balance: %w", err)
	}

	common.SetBalance(b.GetName(), "spot", "USDT", balance)

	// Place market buy order using quoteOrderQty (USDT amount)
	params := url.Values{}
	params.Set("symbol", symbol)
	params.Set("side", "BUY")
	params.Set("type", "MARKET")
	params.Set("quoteOrderQty", fmt.Sprintf("%.8f", amountUSDT))
	params.Set("timestamp", strconv.FormatInt(time.Now().UnixMilli(), 10))

	var orderResp struct {
		OrderID             int64  `json:"orderId"`
		ExecutedQty         string `json:"executedQty"`
		CummulativeQuoteQty string `json:"cummulativeQuoteQty"`
		Status              string `json:"status"`
		Fills               []struct {
			Price           string `json:"price"`
			Qty             string `json:"qty"`
			Commission      string `json:"commission"`
			CommissionAsset string `json:"commissionAsset"`
		} `json:"fills"`
	}

	err = b.signedRequest(ctx, "POST", b.spotBaseURL+"/api/v3/order", params, &orderResp)
	if err != nil {
		log.Printf("[BINANCE] PutSpotLong - ERROR: Order failed: %v", err)
		return nil, fmt.Errorf("spot buy order failed: %w", err)
	}

	// CummulativeQuoteQty is the GROSS quote amount traded (before fee in quote asset)
	grossUSDTTraded, _ := strconv.ParseFloat(orderResp.CummulativeQuoteQty, 64)
	execQty, _ := strconv.ParseFloat(orderResp.ExecutedQty, 64)

	// Calculate total fees and convert to USDT equivalent
	var totalFeeInUSDT float64
	var totalFeeInOtherAsset float64

	for _, fill := range orderResp.Fills {
		fee, _ := strconv.ParseFloat(fill.Commission, 64)
		price, _ := strconv.ParseFloat(fill.Price, 64)

		if fill.CommissionAsset == "USDT" {
			totalFeeInUSDT += fee
		} else {
			// Fee is in base asset (e.g., DOGE), convert to USDT at fill price
			totalFeeInOtherAsset += fee
			totalFeeInUSDT += fee * price // Convert fee to USDT equivalent
		}
	}

	// Actual USDT cost = gross traded + all fees in USDT equivalent
	actualUSDTSpent := grossUSDTTraded + totalFeeInUSDT

	// Avg price is based on traded notional (gross), fees do not change price
	avgPrice := grossUSDTTraded / execQty

	// Store position with REAL USDT spent
	b.posMutex.Lock()
	b.positions[pairName+"_spot"] = &common.Position{
		PairName:     pairName,
		Side:         "long",
		Market:       "spot",
		EntryPrice:   avgPrice,
		Quantity:     execQty,
		AmountUSDT:   actualUSDTSpent, // <-- REAL USDT balance change on open
		OrderID:      strconv.FormatInt(orderResp.OrderID, 10),
		ExchangeName: b.GetName(),
	}
	b.posMutex.Unlock()

	// For TradeResult.Fee we return the fee in USDT equivalent
	return &common.TradeResult{
		OrderID:       strconv.FormatInt(orderResp.OrderID, 10),
		ExecutedPrice: avgPrice,
		ExecutedQty:   execQty,
		Fee:           totalFeeInUSDT,
		Success:       orderResp.Status == "FILLED",
	}, nil
}

func (b *BinanceClient) CloseSpotLong(ctx context.Context, pairName string, amountUSDT float64) (*common.TradeResult, float64, error) {
	symbol := b.normalizePairName(pairName, false)

	// Extract base asset from pair name (e.g., "btc-usdt" -> "BTC")
	baseAsset := b.getBaseAsset(pairName)

	// Get actual balance from Binance API
	balance, err := b.getSpotBalance(ctx, baseAsset)
	if err != nil {
		log.Printf("[BINANCE] CloseSpotLong - ERROR: Failed to get balance: %v", err)
		return nil, 0.00, fmt.Errorf("failed to get balance: %w", err)
	}

	if common.IsZero(balance) {
		log.Printf("[BINANCE] CloseSpotLong - No balance found on exchange for %s", baseAsset)
		// Clean up local position tracking
		b.posMutex.Lock()
		delete(b.positions, pairName+"_spot")
		b.posMutex.Unlock()
		return nil, 0.00, fmt.Errorf("no balance on exchange for %s", baseAsset)
	}

	closeQuantity := common.RoundQuantity(balance, pairName)
	if common.IsNegativeOrZero(closeQuantity) {
		log.Printf("[BINANCE] CloseSpotLong - ERROR: Calculated quantity is zero or negative: %.8f", closeQuantity)
		return nil, 0.00, fmt.Errorf("invalid close quantity: %.8f", closeQuantity)
	}

	// Store entry data from local tracking for PnL calculation

	// Place market sell order
	params := url.Values{}
	params.Set("symbol", symbol)
	params.Set("side", "SELL")
	params.Set("type", "MARKET")
	params.Set("quantity", common.FormatQuantity(closeQuantity, pairName))
	params.Set("timestamp", strconv.FormatInt(time.Now().UnixMilli(), 10))

	var orderResp struct {
		OrderID             int64  `json:"orderId"`
		ExecutedQty         string `json:"executedQty"`
		CummulativeQuoteQty string `json:"cummulativeQuoteQty"`
		Status              string `json:"status"`
		Fills               []Fill `json:"fills"`
	}

	err = b.signedRequest(ctx, "POST", b.spotBaseURL+"/api/v3/order", params, &orderResp)
	if err != nil {
		log.Printf("[BINANCE] CloseSpotLong - ERROR: Close order failed: %v", err)
		return nil, 0.00, fmt.Errorf("spot close order failed: %w", err)
	}

	// CummulativeQuoteQty is GROSS quote asset received
	grossUSDTReceived, _ := strconv.ParseFloat(orderResp.CummulativeQuoteQty, 64)
	execQty, _ := strconv.ParseFloat(orderResp.ExecutedQty, 64)

	// Calculate total fee and convert to USDT equivalent
	var totalFeeInUSDT float64
	var totalFeeInOtherAsset float64

	for _, fill := range orderResp.Fills {
		fee, _ := strconv.ParseFloat(fill.Commission, 64)

		if fill.CommissionAsset == "USDT" {
			totalFeeInUSDT += fee
		} else {
			// Fee is in other asset (e.g., BNB), need to handle appropriately
			// For sell orders, if fee is in BNB, it doesn't affect USDT received
			// But for profit calculation, we should note it
			totalFeeInOtherAsset += fee
		}
	}

	// Net USDT received (only subtract if fee was in USDT)
	avgPrice := grossUSDTReceived / execQty

	// Remove position from local tracking
	b.posMutex.Lock()
	delete(b.positions, pairName+"_spot")
	b.posMutex.Unlock()

	totalFeeForReturn := totalFeeInUSDT
	if common.IsZero(totalFeeForReturn) {
		totalFeeForReturn = totalFeeInOtherAsset
	}

	newBalance, err := b.getSpotBalance(ctx, "USDT")
	if err != nil {
		log.Printf("[BINANCE] PutSpotLong - ERROR: Failed to get USDT balance: %v", err)
		return nil, 0.00, fmt.Errorf("failed to get USDT balance: %w", err)
	}

	prevBalance := common.GetBalance(b.GetName(), "spot", "USDT")

	common.SetBalance(b.GetName(), "spot", "USDT", newBalance)

	profit := newBalance - prevBalance

	return &common.TradeResult{
		OrderID:       strconv.FormatInt(orderResp.OrderID, 10),
		ExecutedPrice: avgPrice,
		ExecutedQty:   execQty,
		Fee:           totalFeeForReturn,
		Success:       orderResp.Status == "FILLED",
	}, profit, nil
}
