package bitget

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"arbitrage.trade/clients/common"
)

func (b *BitgetClient) getSpotAssetBalance(ctx context.Context, asset string) (float64, error) {
	var r struct {
		Code string `json:"code"`
		Data []struct {
			Coin      string `json:"coin"`
			Available string `json:"available"`
		} `json:"data"`
	}

	if err := b.signedRequest(ctx, "GET", "/api/v2/spot/account/assets", nil, &r); err != nil {
		return 0, err
	}

	for _, bal := range r.Data {
		if strings.EqualFold(bal.Coin, asset) {
			v, _ := strconv.ParseFloat(bal.Available, 64)
			return v, nil
		}
	}
	return 0, nil
}

func (b *BitgetClient) PutSpotLong(ctx context.Context, pairName string, amountUSDT float64) (*common.TradeResult, error) {
	symbol := b.normalizeSymbol(pairName)

	balance, err := b.getSpotAssetBalance(ctx, "USDT")
	if err != nil {
		log.Printf("[BITGET] PutSpotLong - ERROR: Failed to get USDT balance: %v", err)
		return nil, fmt.Errorf("failed to get USDT balance: %w", err)
	}

	common.SetBalance(b.GetName(), "spot", "USDT", balance)

	// Get ticker price for reference
	price, err := b.getSpotTicker(ctx, symbol)
	if err != nil {
		log.Printf("[BITGET] PutSpotLong - ticker error: %v", err)
		return nil, err
	}
	estimatedQty := amountUSDT / price

	// For market buy orders on Bitget, we might need to specify quote currency amount (USDT)
	// instead of base currency quantity (BTC). Let's try both approaches.

	qty := common.RoundQuantity(estimatedQty, pairName)
	if common.IsNegativeOrZero(qty) {
		return nil, fmt.Errorf("calculated quantity is zero after rounding")
	}

	formattedQty := common.FormatQuantity(qty, pairName)
	// For market buy, Bitget might want the USDT amount instead
	formattedAmount := fmt.Sprintf("%.4f", amountUSDT)

	log.Printf("[BITGET] PutSpotLong - symbol: %s, price: %.2f, amountUSDT: %.2f, qty: %f, formatted qty: %s, formatted amount: %s",
		symbol, price, amountUSDT, qty, formattedQty, formattedAmount)

	// Try using quote currency amount for market buy (common for CEX market orders)
	body := map[string]interface{}{
		"symbol":    symbol,
		"side":      "buy",
		"orderType": "market",
		"force":     "gtc",
		"size":      formattedAmount, // Use USDT amount for market buy
		"clientOid": fmt.Sprintf("spot_%d", time.Now().UnixNano()),
	}

	var resp struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			OrderID   string `json:"orderId"`
			ClientOid string `json:"clientOid"`
		} `json:"data"`
	}

	if err := b.signedRequest(ctx, "POST", "/api/v2/spot/trade/place-order", body, &resp); err != nil {
		log.Printf("[BITGET] PutSpotLong - order error: %v", err)
		return nil, err
	}

	if resp.Code != "00000" {
		return nil, fmt.Errorf("bitget error: %s - %s", resp.Code, resp.Msg)
	}

	// Store position (execution details would need order query in production)
	b.mu.Lock()
	b.positions[pairName+"_spot"] = &common.Position{
		PairName:     pairName,
		Side:         "long",
		Market:       "spot",
		EntryPrice:   price,
		Quantity:     qty,
		AmountUSDT:   amountUSDT,
		OrderID:      resp.Data.OrderID,
		ExchangeName: b.GetName(),
	}
	b.mu.Unlock()

	return &common.TradeResult{
		OrderID:       resp.Data.OrderID,
		ExecutedPrice: price,
		ExecutedQty:   qty,
		Success:       true,
	}, nil
}

func (b *BitgetClient) CloseSpotLong(ctx context.Context, pairName string, amountUSDT float64) (*common.TradeResult, float64, error) {
	symbol := b.normalizeSymbol(pairName)

	// Get actual asset balance
	asset := strings.TrimSuffix(symbol, "USDT")
	bal, err := b.getSpotAssetBalance(ctx, asset)
	if err != nil {
		return nil, 0.00, err
	}
	if common.IsNegativeOrZero(bal) {
		return nil, 0.00, fmt.Errorf("no balance for asset %s", asset)
	}

	qty := common.RoundQuantity(bal, pairName)
	if common.IsNegativeOrZero(qty) {
		return nil, 0.00, fmt.Errorf("rounded qty is zero")
	}

	body := map[string]interface{}{
		"symbol":    symbol,
		"side":      "sell",
		"orderType": "market",
		"force":     "gtc",
		"size":      common.FormatQuantity(qty, pairName),
		"clientOid": fmt.Sprintf("close_spot_%d", time.Now().UnixNano()),
	}

	var resp struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			OrderID   string `json:"orderId"`
			ClientOid string `json:"clientOid"`
		} `json:"data"`
	}

	if err := b.signedRequest(ctx, "POST", "/api/v2/spot/trade/place-order", body, &resp); err != nil {
		return nil, 0.00, err
	}

	if resp.Code != "00000" {
		return nil, 0.00, fmt.Errorf("bitget error: %s - %s", resp.Code, resp.Msg)
	}

	b.mu.Lock()
	delete(b.positions, pairName+"_spot")
	b.mu.Unlock()

	newBalance, err := b.getSpotAssetBalance(ctx, "USDT")
	if err != nil {
		log.Printf("[BITGET] CloseSpotLong - ERROR: Failed to get USDT balance: %v", err)
		return nil, 0.00, fmt.Errorf("failed to get USDT balance: %w", err)
	}

	prevBalance := common.GetBalance(b.GetName(), "spot", "USDT")

	common.SetBalance(b.GetName(), "spot", "USDT", newBalance)

	return &common.TradeResult{
		OrderID:     resp.Data.OrderID,
		ExecutedQty: qty,
		Success:     true,
	}, newBalance - prevBalance, nil
}
