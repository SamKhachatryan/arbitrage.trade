package gate

import (
	"context"
	"fmt"
	"strconv"

	"arbitrage.trade/clients/common"
)

func (g *GateClient) getFuturesBalance(ctx context.Context) (float64, error) {
	var balances []FuturesBalance
	if err := g.signedRequest(ctx, "GET", "/api/v4/futures/usdt/accounts", "", &balances); err != nil {
		return 0, fmt.Errorf("failed to get futures balance: %w", err)
	}

	for _, bal := range balances {
		if bal.Currency == "USDT" {
			available, _ := strconv.ParseFloat(bal.Available, 64)
			return available, nil
		}
	}

	return 0, nil
}

func (g *GateClient) getFuturesPosition(ctx context.Context, contract string) (*FuturesPosition, error) {
	var positions []FuturesPosition
	if err := g.signedRequest(ctx, "GET", fmt.Sprintf("/api/v4/futures/usdt/positions?contract=%s", contract), "", &positions); err != nil {
		return nil, err
	}

	for _, pos := range positions {
		if pos.Contract == contract && common.NotEqual(float64(pos.Size), 0) {
			return &pos, nil
		}
	}

	return nil, nil
}

func (g *GateClient) PutFuturesShort(ctx context.Context, pairName string, amountUSDT float64) (*common.TradeResult, error) {
	contract := g.normalizeSymbolFutures(pairName)

	balance, err := g.getFuturesBalance(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get futures balance: %w", err)
	}

	common.SetBalance(g.GetName(), "futures", "USDT", balance)

	price, err := g.getPrice(ctx, g.normalizeSymbol(pairName))
	if err != nil {
		return nil, fmt.Errorf("failed to get price: %w", err)
	}

	quantity := amountUSDT / price
	size := -int64(quantity) // Negative for short

	orderBody := fmt.Sprintf(`{
		"contract": "%s",
		"size": %d,
		"tif": "ioc",
		"reduce_only": false
	}`, contract, size)

	var response FuturesOrderResponse
	if err := g.signedRequest(ctx, "POST", "/api/v4/futures/usdt/orders", orderBody, &response); err != nil {
		return nil, fmt.Errorf("market order failed: %w", err)
	}

	fillPrice, _ := strconv.ParseFloat(response.FillPrice, 64)
	actualSize := float64(response.Size)
	if common.IsNegative(actualSize) {
		actualSize = -actualSize
	}
	fee, _ := strconv.ParseFloat(response.TkfFee, 64)

	g.mu.Lock()
	g.positions[pairName+"_futures"] = &common.Position{
		PairName:     pairName,
		Side:         "short",
		Market:       "futures",
		EntryPrice:   fillPrice,
		Quantity:     actualSize,
		AmountUSDT:   actualSize * fillPrice,
		OrderID:      strconv.FormatInt(response.ID, 10),
		ExchangeName: g.GetName(),
	}
	g.mu.Unlock()

	return &common.TradeResult{
		OrderID:       strconv.FormatInt(response.ID, 10),
		ExecutedPrice: fillPrice,
		ExecutedQty:   actualSize,
		Fee:           fee,
		Success:       response.Status == "finished",
	}, nil
}

func (g *GateClient) CloseFuturesShort(ctx context.Context, pairName string) (*common.TradeResult, float64, error) {
	contract := g.normalizeSymbolFutures(pairName)

	position, err := g.getFuturesPosition(ctx, contract)
	if err != nil {
		return nil, 0.0, fmt.Errorf("failed to get position: %w", err)
	}

	if position == nil || common.IsZero(float64(position.Size)) {
		g.mu.Lock()
		delete(g.positions, pairName+"_futures")
		g.mu.Unlock()
		return nil, 0.0, fmt.Errorf("no open position on exchange")
	}

	closeSize := -position.Size // Opposite side to close

	orderBody := fmt.Sprintf(`{
		"contract": "%s",
		"size": %d,
		"tif": "ioc",
		"reduce_only": true
	}`, contract, closeSize)

	var response FuturesOrderResponse
	if err := g.signedRequest(ctx, "POST", "/api/v4/futures/usdt/orders", orderBody, &response); err != nil {
		return nil, 0.0, fmt.Errorf("close order failed: %w", err)
	}

	g.mu.Lock()
	delete(g.positions, pairName+"_futures")
	g.mu.Unlock()

	newBalance, err := g.getFuturesBalance(ctx)
	if err != nil {
		return nil, 0.0, fmt.Errorf("failed to get futures balance: %w", err)
	}

	prevBalance := common.GetBalance(g.GetName(), "futures", "USDT")
	common.SetBalance(g.GetName(), "futures", "USDT", newBalance)

	profit := newBalance - prevBalance

	fillPrice, _ := strconv.ParseFloat(response.FillPrice, 64)
	actualSize := float64(response.Size)
	if common.IsNegative(actualSize) {
		actualSize = -actualSize
	}
	fee, _ := strconv.ParseFloat(response.TkfFee, 64)

	return &common.TradeResult{
		OrderID:       strconv.FormatInt(response.ID, 10),
		ExecutedPrice: fillPrice,
		ExecutedQty:   actualSize,
		Fee:           fee,
		Success:       response.Status == "finished",
	}, profit, nil
}
