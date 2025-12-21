package whitebit

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	"arbitrage.trade/clients/common"
)

func (w *WhitebitClient) getSpotBalance(ctx context.Context, ticker string) (float64, error) {
	params := map[string]interface{}{
		"ticker": ticker,
	}

	var balances BalanceResponse
	if err := w.signedRequest(ctx, "/api/v4/trade-account/balance", params, &balances); err != nil {
		return 0, err
	}

	available, _ := strconv.ParseFloat(balances.Available, 64)
	return available, nil
}

func (w *WhitebitClient) PutSpotLong(ctx context.Context, pairName string, amountUSDT float64) (*common.TradeResult, error) {
	market := w.normalizeSymbol(pairName)

	balance, err := w.getSpotBalance(ctx, "USDT")
	if err != nil {
		log.Printf("[WHITEBIT] PutSpotLong - ERROR: Failed to get USDT balance: %v", err)
		return nil, fmt.Errorf("failed to get USDT balance: %w", err)
	}

	common.SetBalance(w.GetName(), "spot", "USDT", balance)

	params := map[string]interface{}{
		"market": market,
		"side":   "buy",
		"amount": amountUSDT,
	}

	var response MarketOrderResponse
	if err := w.signedRequest(ctx, "/api/v4/order/market", params, &response); err != nil {
		log.Printf("[WHITEBIT] PutSpotLong - ERROR: Order failed: %v", err)
		return nil, fmt.Errorf("market order failed: %w", err)
	}

	dealStock, _ := strconv.ParseFloat(response.DealStock, 64)
	dealMoney, _ := strconv.ParseFloat(response.DealMoney, 64)
	dealFee, _ := strconv.ParseFloat(response.DealFee, 64)

	actualPrice := 0.0
	if common.IsPositive(dealStock) {
		actualPrice = dealMoney / dealStock
	}

	w.mu.Lock()
	w.positions[pairName+"_spot"] = &common.Position{
		PairName:     pairName,
		Side:         "long",
		Market:       "spot",
		EntryPrice:   actualPrice,
		Quantity:     dealStock,
		AmountUSDT:   dealMoney,
		OrderID:      fmt.Sprintf("%d", response.OrderID),
		ExchangeName: w.GetName(),
	}
	w.mu.Unlock()

	return &common.TradeResult{
		OrderID:       fmt.Sprintf("%d", response.OrderID),
		ExecutedPrice: actualPrice,
		ExecutedQty:   dealStock,
		Fee:           dealFee,
		Success:       response.Status == "FILLED",
	}, nil
}

func (w *WhitebitClient) CloseSpotLong(ctx context.Context, pairName string, amountUSDT float64) (*common.TradeResult, float64, error) {
	market := w.normalizeSymbol(pairName)

	w.mu.RLock()
	_, exists := w.positions[pairName+"_spot"]
	w.mu.RUnlock()

	if !exists {
		return nil, 0.0, fmt.Errorf("no position found for %s", pairName)
	}

	// Get base asset ticker (e.g., BTC from BTC_USDT)
	baseAsset := strings.Split(market, "_")[0]

	balance, err := w.getSpotBalance(ctx, baseAsset)
	if err != nil {
		log.Printf("[WHITEBIT] CloseSpotLong - ERROR: Failed to get %s balance: %v", baseAsset, err)
		return nil, 0.0, fmt.Errorf("failed to get %s balance: %w", baseAsset, err)
	}

	if common.IsNegativeOrZero(balance) {
		return nil, 0.0, fmt.Errorf("no %s balance to sell", baseAsset)
	}

	sellQuantity := common.RoundQuantity(balance, pairName)

	params := map[string]interface{}{
		"market": market,
		"side":   "sell",
		"amount": common.FormatQuantity(sellQuantity, pairName),
	}

	var response MarketOrderResponse
	if err := w.signedRequest(ctx, "/api/v4/order/market", params, &response); err != nil {
		log.Printf("[WHITEBIT] CloseSpotLong - ERROR: Order failed: %v", err)
		return nil, 0.0, fmt.Errorf("market order failed: %w", err)
	}

	w.mu.Lock()
	delete(w.positions, pairName+"_spot")
	w.mu.Unlock()

	newBalance, err := w.getSpotBalance(ctx, "USDT")
	if err != nil {
		log.Printf("[WHITEBIT] CloseSpotLong - ERROR: Failed to get USDT balance: %v", err)
		return nil, 0.0, fmt.Errorf("failed to get USDT balance: %w", err)
	}

	prevBalance := common.GetBalance(w.GetName(), "spot", "USDT")
	common.SetBalance(w.GetName(), "spot", "USDT", newBalance)

	profit := newBalance - prevBalance

	dealStock, _ := strconv.ParseFloat(response.DealStock, 64)
	dealMoney, _ := strconv.ParseFloat(response.DealMoney, 64)
	dealFee, _ := strconv.ParseFloat(response.DealFee, 64)

	actualPrice := 0.0
	if common.IsPositive(dealStock) {
		actualPrice = dealMoney / dealStock
	}

	return &common.TradeResult{
		OrderID:       fmt.Sprintf("%d", response.OrderID),
		ExecutedPrice: actualPrice,
		ExecutedQty:   dealStock,
		Fee:           dealFee,
		Success:       response.Status == "FILLED",
	}, profit, nil
}
