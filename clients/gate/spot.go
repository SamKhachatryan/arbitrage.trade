package gate

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"arbitrage.trade/clients/common"
)

func (g *GateClient) getSpotBalance(ctx context.Context, currency string) (float64, error) {
	var balances []SpotBalance
	if err := g.signedRequest(ctx, "GET", "/api/v4/spot/accounts", "", &balances); err != nil {
		return 0, fmt.Errorf("failed to get spot balance: %w", err)
	}

	for _, bal := range balances {
		if bal.Currency == currency {
			available, _ := strconv.ParseFloat(bal.Available, 64)
			return available, nil
		}
	}

	return 0, nil
}

func (g *GateClient) PutSpotLong(ctx context.Context, pairName string, amountUSDT float64) (*common.TradeResult, error) {
	symbol := g.normalizeSymbol(pairName)

	balance, err := g.getSpotBalance(ctx, "USDT")
	if err != nil {
		return nil, fmt.Errorf("failed to get USDT balance: %w", err)
	}

	common.SetBalance(g.GetName(), "spot", "USDT", balance)

	orderBody := fmt.Sprintf(`{
		"currency_pair": "%s",
		"side": "buy",
		"amount": "%.8f",
		"type": "market"
	}`, symbol, amountUSDT)

	var response SpotOrderResponse
	if err := g.signedRequest(ctx, "POST", "/api/v4/spot/orders", orderBody, &response); err != nil {
		return nil, fmt.Errorf("market order failed: %w", err)
	}

	filledTotal, _ := strconv.ParseFloat(response.FilledTotal, 64)
	amount, _ := strconv.ParseFloat(response.Amount, 64)
	avgPrice, _ := strconv.ParseFloat(response.AvgDealPrice, 64)
	fee, _ := strconv.ParseFloat(response.Fee, 64)

	g.mu.Lock()
	g.positions[pairName+"_spot"] = &common.Position{
		PairName:     pairName,
		Side:         "long",
		Market:       "spot",
		EntryPrice:   avgPrice,
		Quantity:     amount,
		AmountUSDT:   filledTotal,
		OrderID:      response.ID,
		ExchangeName: g.GetName(),
	}
	g.mu.Unlock()

	return &common.TradeResult{
		OrderID:       response.ID,
		ExecutedPrice: avgPrice,
		ExecutedQty:   amount,
		Fee:           fee,
		Success:       response.Status == "closed",
	}, nil
}

func (g *GateClient) CloseSpotLong(ctx context.Context, pairName string, amountUSDT float64) (*common.TradeResult, float64, error) {
	symbol := g.normalizeSymbol(pairName)

	g.mu.RLock()
	_, exists := g.positions[pairName+"_spot"]
	g.mu.RUnlock()

	if !exists {
		return nil, 0.0, fmt.Errorf("no position found for %s", pairName)
	}

	baseAsset := strings.Split(symbol, "_")[0]
	balance, err := g.getSpotBalance(ctx, baseAsset)
	if err != nil {
		return nil, 0.0, fmt.Errorf("failed to get %s balance: %w", baseAsset, err)
	}

	if balance <= 0 {
		return nil, 0.0, fmt.Errorf("no %s balance to sell", baseAsset)
	}

	sellQuantity := common.RoundQuantity(balance, pairName)

	orderBody := fmt.Sprintf(`{
		"currency_pair": "%s",
		"side": "sell",
		"amount": "%s",
		"type": "market"
	}`, symbol, common.FormatQuantity(sellQuantity, pairName))

	var response SpotOrderResponse
	if err := g.signedRequest(ctx, "POST", "/api/v4/spot/orders", orderBody, &response); err != nil {
		return nil, 0.0, fmt.Errorf("market order failed: %w", err)
	}

	g.mu.Lock()
	delete(g.positions, pairName+"_spot")
	g.mu.Unlock()

	newBalance, err := g.getSpotBalance(ctx, "USDT")
	if err != nil {
		return nil, 0.0, fmt.Errorf("failed to get USDT balance: %w", err)
	}

	prevBalance := common.GetBalance(g.GetName(), "spot", "USDT")
	common.SetBalance(g.GetName(), "spot", "USDT", newBalance)

	profit := newBalance - prevBalance

	amount, _ := strconv.ParseFloat(response.Amount, 64)
	avgPrice, _ := strconv.ParseFloat(response.AvgDealPrice, 64)
	fee, _ := strconv.ParseFloat(response.Fee, 64)

	return &common.TradeResult{
		OrderID:       response.ID,
		ExecutedPrice: avgPrice,
		ExecutedQty:   amount,
		Fee:           fee,
		Success:       response.Status == "closed",
	}, profit, nil
}
