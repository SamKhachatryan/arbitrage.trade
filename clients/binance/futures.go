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

func (b *BinanceClient) getFuturesPrice(symbol string) (float64, error) {
	url := fmt.Sprintf("%s/fapi/v1/ticker/price?symbol=%s", b.futsBaseURL, symbol)

	resp, err := http.Get(url)
	if err != nil {
		log.Printf("[BINANCE] getFuturesPrice - ERROR: HTTP request failed: %v", err)
		return 0, err
	}
	defer resp.Body.Close()

	var result struct {
		Price string `json:"price"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("[BINANCE] getFuturesPrice - ERROR: JSON decode failed: %v", err)
		return 0, err
	}

	price, err := strconv.ParseFloat(result.Price, 64)
	if err != nil {
		log.Printf("[BINANCE] getFuturesPrice - ERROR: Price parse failed: %v", err)
		return 0, err
	}

	return price, nil
}

func (b *BinanceClient) getFuturesPositionRisk(ctx context.Context, symbol string) (*PositionRisk, error) {
	params := url.Values{}
	params.Set("symbol", symbol)
	params.Set("timestamp", strconv.FormatInt(time.Now().UnixMilli(), 10))

	var positions []PositionRisk
	err := b.signedRequest(ctx, "GET", b.futsBaseURL+"/fapi/v2/positionRisk", params, &positions)
	if err != nil {
		log.Printf("[BINANCE] getFuturesPositionRisk - ERROR: Request failed: %v", err)
		return nil, err
	}

	// Find the position for the symbol (BOTH side for hedge mode, or default)
	for _, pos := range positions {
		if pos.Symbol == symbol && pos.PositionAmt != 0 {
			return &pos, nil
		}
	}

	// Return empty position if none found
	return &PositionRisk{Symbol: symbol, PositionAmt: 0}, nil
}

func (b *BinanceClient) getFuturesBalance(ctx context.Context) (float64, error) {
	params := url.Values{}
	params.Set("timestamp", strconv.FormatInt(time.Now().UnixMilli(), 10))

	var accountInfo []struct {
		Asset            string `json:"asset"`
		WalletBalance    string `json:"walletBalance"`
		AvailableBalance string `json:"availableBalance"`
		UnrealizedProfit string `json:"unrealizedProfit"`
		MarginBalance    string `json:"marginBalance"`
	}

	err := b.signedRequest(ctx, "GET", b.futsBaseURL+"/fapi/v2/balance", params, &accountInfo)
	if err != nil {
		log.Printf("[BINANCE] getFuturesBalance - ERROR: Request failed: %v", err)
		return 0, err
	}

	// Find USDT balance
	for _, asset := range accountInfo {
		if asset.Asset == "USDT" {
			balance, _ := strconv.ParseFloat(asset.AvailableBalance, 64)
			return balance, nil
		}
	}

	return 0, nil
}

func (b *BinanceClient) PutFuturesShort(ctx context.Context, pairName string, amountUSDT float64) (*common.TradeResult, error) {
	symbol := b.normalizePairName(pairName, true)

	// Get current price to calculate quantity
	price, err := b.getFuturesPrice(symbol)
	if err != nil {
		log.Printf("[BINANCE] PutFuturesShort - ERROR: Failed to get futures price: %v", err)
		return nil, fmt.Errorf("failed to get futures price: %w", err)
	}

	balance, err := b.getFuturesBalance(ctx)
	if err != nil {
		log.Printf("[BINANCE] PutFuturesShort - ERROR: Failed to get USDT balance: %v", err)
		return nil, fmt.Errorf("failed to get USDT balance: %w", err)
	}

	common.SetBalance(b.GetName(), "futures", "USDT", balance)

	quantity := amountUSDT / price

	quantity = common.RoundQuantity(quantity, pairName)
	// Place market sell order (short)
	params := url.Values{}
	params.Set("symbol", symbol)
	params.Set("side", "SELL")
	params.Set("type", "MARKET")
	params.Set("quantity", common.FormatQuantity(quantity, pairName))
	params.Set("timestamp", strconv.FormatInt(time.Now().UnixMilli(), 10))

	var orderResp struct {
		OrderID     int64  `json:"orderId"`
		ExecutedQty string `json:"executedQty"`
		AvgPrice    string `json:"avgPrice"`
		Status      string `json:"status"`
	}

	err = b.signedRequest(ctx, "POST", b.futsBaseURL+"/fapi/v1/order", params, &orderResp)
	if err != nil {
		log.Printf("[BINANCE] PutFuturesShort - ERROR: Order failed: %v", err)
		return nil, fmt.Errorf("futures short order failed: %w", err)
	}

	execQty, _ := strconv.ParseFloat(orderResp.ExecutedQty, 64)
	avgPrice, _ := strconv.ParseFloat(orderResp.AvgPrice, 64)

	// Store position
	b.posMutex.Lock()
	b.positions[pairName+"_futures"] = &common.Position{
		PairName:     pairName,
		Side:         "short",
		Market:       "futures",
		EntryPrice:   avgPrice,
		Quantity:     execQty,
		AmountUSDT:   amountUSDT,
		OrderID:      strconv.FormatInt(orderResp.OrderID, 10),
		ExchangeName: b.GetName(),
	}
	b.posMutex.Unlock()

	return &common.TradeResult{
		OrderID:       strconv.FormatInt(orderResp.OrderID, 10),
		ExecutedPrice: avgPrice,
		ExecutedQty:   execQty,
		Fee:           0, // Futures API doesn't return fee in order response
		Success:       orderResp.Status == "FILLED",
	}, nil
}

func (b *BinanceClient) CloseFuturesShort(ctx context.Context, pairName string) (*common.TradeResult, float64, error) {
	symbol := b.normalizePairName(pairName, true)

	// Get actual position from Binance API
	positionRisk, err := b.getFuturesPositionRisk(ctx, symbol)
	if err != nil {
		log.Printf("[BINANCE] CloseFuturesShort - ERROR: Failed to get position risk: %v", err)
		return nil, 0.00, fmt.Errorf("failed to get position risk: %w", err)
	}

	if positionRisk.PositionAmt == 0 {
		log.Printf("[BINANCE] CloseFuturesShort - No open position found on exchange for %s", symbol)
		// Clean up local position tracking
		b.posMutex.Lock()
		delete(b.positions, pairName+"_futures")
		b.posMutex.Unlock()
		return nil, 0.00, fmt.Errorf("no open position on exchange")
	}

	// Calculate the quantity to close (absolute value of position amount)
	var closeQuantity float64
	if positionRisk.PositionAmt < 0 {
		closeQuantity = -positionRisk.PositionAmt
	} else {
		closeQuantity = positionRisk.PositionAmt
	}

	// Round quantity to step size
	closeQuantity = common.RoundQuantity(closeQuantity, pairName)

	if closeQuantity <= 0 {
		log.Printf("[BINANCE] CloseFuturesShort - ERROR: Calculated quantity is zero or negative: %.8f", closeQuantity)
		return nil, 0.00, fmt.Errorf("invalid close quantity: %.8f", closeQuantity)
	}

	// Place market buy order to close short
	params := url.Values{}
	params.Set("symbol", symbol)
	params.Set("side", "BUY")
	params.Set("type", "MARKET")
	params.Set("quantity", common.FormatQuantity(closeQuantity, pairName))
	params.Set("timestamp", strconv.FormatInt(time.Now().UnixMilli(), 10))

	var orderResp struct {
		OrderID     int64  `json:"orderId"`
		ExecutedQty string `json:"executedQty"`
		AvgPrice    string `json:"avgPrice"`
		Status      string `json:"status"`
	}

	err = b.signedRequest(ctx, "POST", b.futsBaseURL+"/fapi/v1/order", params, &orderResp)
	if err != nil {
		log.Printf("[BINANCE] CloseFuturesShort - ERROR: Close order failed: %v", err)
		return nil, 0.00, fmt.Errorf("futures close order failed: %w", err)
	}

	execQty, _ := strconv.ParseFloat(orderResp.ExecutedQty, 64)
	avgPrice, _ := strconv.ParseFloat(orderResp.AvgPrice, 64)

	// Remove position from local tracking
	b.posMutex.Lock()
	delete(b.positions, pairName+"_futures")
	b.posMutex.Unlock()

	newBalance, err := b.getFuturesBalance(ctx)
	if err != nil {
		log.Printf("[BINANCE] CloseFuturesShort - ERROR: Failed to get USDT balance: %v", err)
		return nil, 0.00, fmt.Errorf("failed to get USDT balance: %w", err)
	}

	prevBalance := common.GetBalance(b.GetName(), "futures", "USDT")

	common.SetBalance(b.GetName(), "futures", "USDT", newBalance)

	profit := newBalance - prevBalance

	return &common.TradeResult{
		OrderID:       strconv.FormatInt(orderResp.OrderID, 10),
		ExecutedPrice: avgPrice,
		ExecutedQty:   execQty,
		Fee:           0,
		Success:       orderResp.Status == "FILLED",
	}, profit, nil
}
