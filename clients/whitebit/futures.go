package whitebit

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	"arbitrage.trade/clients/common"
)

func (w *WhitebitClient) waitForPositionClosed(ctx context.Context, market string, maxWaitTime time.Duration) error {
	deadline := time.Now().Add(maxWaitTime)
	checkInterval := 300 * time.Millisecond // Check every 300ms

	for time.Now().Before(deadline) {
		time.Sleep(checkInterval)

		position, err := w.getOpenPosition(ctx, market)
		if err != nil {
			fmt.Printf("[WHITEBIT] |waitForPositionClosed| - ERROR checking position: %v", err)
			continue // Keep trying
		}

		// Position is closed if it's nil or amount is zero
		if position == nil {
			return nil
		}

		amount, _ := strconv.ParseFloat(position.Amount, 64)
		if common.IsZero(amount) {
			return nil
		}

		fmt.Printf("[WHITEBIT] |waitForPositionClosed| - Position %s still open (amount: %s)", market, position.Amount)
	}

	return fmt.Errorf("position %s did not close within %v", market, maxWaitTime)
}

func (w *WhitebitClient) waitForPositionOpen(ctx context.Context, market string, expectedSide string, maxWaitTime time.Duration) (*CollateralPosition, error) {
	deadline := time.Now().Add(maxWaitTime)
	checkInterval := 300 * time.Millisecond // Check every 300ms

	for time.Now().Before(deadline) {
		time.Sleep(checkInterval)

		position, err := w.getOpenPosition(ctx, market)
		if err != nil {
			fmt.Printf("[WHITEBIT] |waitForPositionOpen| - ERROR checking position: %v", err)
			continue // Keep trying
		}

		if position != nil {
			amount, _ := strconv.ParseFloat(position.Amount, 64)
			if common.NotEqual(amount, 0) {
				return position, nil
			}
		}

		fmt.Printf("[WHITEBIT] |waitForPositionOpen| - Waiting for position %s to open...", market)
	}

	return nil, fmt.Errorf("position %s did not open within %v", market, maxWaitTime)
}

func (w *WhitebitClient) getCollateralBalance(ctx context.Context) (float64, error) {
	params := map[string]interface{}{}

	var balances map[string]string
	if err := w.signedRequest(ctx, "/api/v4/collateral-account/balance", params, &balances); err != nil {
		return 0, fmt.Errorf("failed to get collateral balance")
	}

	if usdtBalance, ok := balances["USDT"]; ok {
		balance, _ := strconv.ParseFloat(usdtBalance, 64)
		return balance, nil
	}

	return 0, nil
}

func (w *WhitebitClient) getOpenPosition(ctx context.Context, market string) (*CollateralPosition, error) {
	params := map[string]interface{}{}

	var positions OpenPositionsResponse
	if err := w.signedRequest(ctx, "/api/v4/collateral-account/positions/open", params, &positions); err != nil {
		return nil, err
	}

	for _, pos := range positions {
		if pos.Market == market {
			return &pos, nil
		}
	}

	return nil, nil
}

func (w *WhitebitClient) PutFuturesShort(ctx context.Context, pairName string, amountUSDT float64) (*common.TradeResult, error) {
	market := w.normalizeSymbolFutures(pairName)

	time.Sleep(100 * time.Millisecond)

	balance, err := w.getCollateralBalance(ctx)
	if err != nil {
		log.Printf("[WHITEBIT] PutFuturesShort - ERROR: Failed to get collateral balance: %v", err)
		return nil, fmt.Errorf("failed to get collateral balance: %w", err)
	}

	common.SetBalance(w.GetName(), "futures", "USDT", balance)

	time.Sleep(100 * time.Millisecond)

	price, err := w.getPrice(ctx, market)
	if err != nil {
		log.Printf("[WHITEBIT] PutFuturesShort - ERROR: Failed to get price: %v", err)
		return nil, fmt.Errorf("failed to get price: %w", err)
	}

	quantity := amountUSDT / price
	quantity = common.RoundQuantity(quantity, pairName)

	if common.IsNegativeOrZero(quantity) {
		return nil, fmt.Errorf("quantity is zero after rounding")
	}

	params := map[string]interface{}{
		"market": market,
		"side":   "sell",
		"amount": quantity,
	}

	var response MarketOrderResponse
	if err := w.signedRequest(ctx, "/api/v4/order/collateral/market", params, &response); err != nil {
		log.Printf("[WHITEBIT] PutFuturesShort - ERROR: Order failed: %v", err)
		return nil, fmt.Errorf("market order failed: %w", err)
	}

	position, err := w.waitForPositionOpen(ctx, market, "short", 10*time.Second)
	if err != nil {
		log.Printf("[WHITEBIT] PutFuturesShort - ERROR: Position did not open: %v", err)
		return nil, fmt.Errorf("position did not open: %w", err)
	}

	dealStock, _ := strconv.ParseFloat(position.Amount, 64)
	if common.IsNegative(dealStock) {
		dealStock = -dealStock // Short positions are negative
	}

	basePrice, _ := strconv.ParseFloat(position.BasePrice, 64)
	dealMoney := dealStock * basePrice

	w.mu.Lock()
	w.positions[pairName+"_futures"] = &common.Position{
		PairName:     pairName,
		Side:         "short",
		Market:       "futures",
		EntryPrice:   basePrice,
		Quantity:     dealStock,
		AmountUSDT:   dealMoney,
		OrderID:      fmt.Sprintf("%d", response.OrderID),
		ExchangeName: w.GetName(),
	}
	w.mu.Unlock()

	return &common.TradeResult{
		OrderID:       fmt.Sprintf("%d", response.OrderID),
		ExecutedPrice: basePrice,
		ExecutedQty:   dealStock,
		Fee:           0, // Fee is accounted in position PNL
		Success:       true,
	}, nil
}

func (w *WhitebitClient) CloseFuturesShort(ctx context.Context, pairName string) (*common.TradeResult, float64, error) {
	market := w.normalizeSymbolFutures(pairName)

	time.Sleep(100 * time.Millisecond)

	position, err := w.getOpenPosition(ctx, market)
	if err != nil {
		log.Printf("[WHITEBIT] CloseFuturesShort - ERROR: Failed to get position: %v", err)
		return nil, 0.0, fmt.Errorf("failed to get position: %w", err)
	}

	if position == nil {
		log.Printf("[WHITEBIT] CloseFuturesShort - No open position found for %s", market)
		w.mu.Lock()
		delete(w.positions, pairName+"_futures")
		w.mu.Unlock()
		return nil, 0.0, fmt.Errorf("no open position on exchange")
	}

	amount, _ := strconv.ParseFloat(position.Amount, 64)
	if common.IsNegative(amount) {
		amount = -amount
	}

	closeQuantity := common.RoundQuantity(amount, pairName)

	if common.IsNegativeOrZero(closeQuantity) {
		return nil, 0.0, fmt.Errorf("calculated quantity is zero after rounding")
	}

	params := map[string]interface{}{
		"market": market,
		"side":   "buy",
		"amount": common.FormatQuantity(closeQuantity, pairName),
	}

	var response MarketOrderResponse
	if err := w.signedRequest(ctx, "/api/v4/order/collateral/market", params, &response); err != nil {
		log.Printf("[WHITEBIT] CloseFuturesShort - ERROR: Close order failed: %v", err)
		return nil, 0.0, fmt.Errorf("collateral close order failed: %w", err)
	}

	// Wait for the position to close (max 10 seconds)
	err = w.waitForPositionClosed(ctx, market, 10*time.Second)
	if err != nil {
		log.Printf("[WHITEBIT] CloseFuturesShort - ERROR: Position did not close: %v", err)
		return nil, 0.0, fmt.Errorf("position did not close: %w", err)
	}

	w.mu.Lock()
	delete(w.positions, pairName+"_futures")
	w.mu.Unlock()

	time.Sleep(100 * time.Millisecond)

	newBalance, err := w.getCollateralBalance(ctx)
	if err != nil {
		log.Printf("[WHITEBIT] CloseFuturesShort - ERROR: Failed to get collateral balance: %v", err)
		return nil, 0.0, fmt.Errorf("failed to get collateral balance: %w", err)
	}

	prevBalance := common.GetBalance(w.GetName(), "futures", "USDT")
	common.SetBalance(w.GetName(), "futures", "USDT", newBalance)

	profit := newBalance - prevBalance

	// Use the close order response data
	dealStock, _ := strconv.ParseFloat(response.DealStock, 64)
	dealMoney, _ := strconv.ParseFloat(response.DealMoney, 64)

	actualPrice := 0.0
	if common.IsPositive(dealStock) {
		actualPrice = dealMoney / dealStock
	}

	return &common.TradeResult{
		OrderID:       fmt.Sprintf("%d", response.OrderID),
		ExecutedPrice: actualPrice,
		ExecutedQty:   dealStock,
		Fee:           0, // Fee included in profit calculation
		Success:       true,
	}, profit, nil
}
