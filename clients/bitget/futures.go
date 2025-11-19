package bitget

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"arbitrage.trade/clients/common"
)

func (b *BitgetClient) getFuturesTicker(symbol string) (float64, error) {
	url := fmt.Sprintf("%s/api/v2/mix/market/ticker?symbol=%s&productType=USDT-FUTURES", b.baseURL, symbol)

	resp, err := http.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	var r struct {
		Code string `json:"code"`
		Data []struct {
			LastPr string `json:"lastPr"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return 0, err
	}
	p, _ := strconv.ParseFloat(r.Data[0].LastPr, 64)

	return p, nil
}

func (b *BitgetClient) getFuturesBalance(ctx context.Context) (float64, error) {
	var r struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
		Data []struct {
			MarginCoin string `json:"marginCoin"`
			Available  string `json:"available"`
		} `json:"data"`
	}

	body := map[string]interface{}{
		"productType": "USDT-FUTURES",
	}

	if err := b.signedRequest(ctx, "GET", "/api/v2/mix/account/accounts", body, &r); err != nil {
		log.Printf("[BITGET] getFuturesBalance - ERROR: Request failed: %v", err)
		return 0, err
	}

	if r.Code != "00000" {
		log.Printf("[BITGET] getFuturesBalance - API error: %s - %s", r.Code, r.Msg)
		return 0, fmt.Errorf("bitget error: %s - %s", r.Code, r.Msg)
	}

	// Find USDT balance
	for _, account := range r.Data {
		if account.MarginCoin == "USDT" {
			balance, _ := strconv.ParseFloat(account.Available, 64)
			return balance, nil
		}
	}

	return 0, nil
}

func (b *BitgetClient) PutFuturesShort(ctx context.Context, pairName string, amountUSDT float64) (*common.TradeResult, error) {
	symbol := b.normalizeSymbol(pairName)

	balance, err := b.getFuturesBalance(ctx)
	if err != nil {
		log.Printf("[BITGET] PutFuturesShort - ERROR: Failed to get USDT balance: %v", err)
		return nil, fmt.Errorf("failed to get USDT balance: %w", err)
	}

	common.SetBalance(b.GetName(), "futures", "USDT", balance)

	price, err := b.getFuturesTicker(symbol)
	if err != nil {
		return nil, err
	}
	quantity := amountUSDT / price
	quantity = common.RoundQuantity(quantity, pairName)
	if quantity <= 0 {
		return nil, fmt.Errorf("calculated futures quantity is zero")
	}

	body := map[string]interface{}{
		"symbol":      symbol,
		"productType": "USDT-FUTURES",
		"marginMode":  "crossed",
		"marginCoin":  "USDT",
		"size":        common.FormatQuantity(quantity, pairName),
		"side":        "sell",
		"tradeSide":   "open",
		"orderType":   "market",
		"holdSide":    "short",
		"clientOid":   fmt.Sprintf("fut_%d", time.Now().UnixNano()),
	}

	var resp struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			OrderID   string `json:"orderId"`
			ClientOid string `json:"clientOid"`
		} `json:"data"`
	}

	if err := b.signedRequest(ctx, "POST", "/api/v2/mix/order/place-order", body, &resp); err != nil {
		return nil, err
	}

	if resp.Code != "00000" {
		return nil, fmt.Errorf("bitget error: %s - %s", resp.Code, resp.Msg)
	}

	b.mu.Lock()
	b.positions[pairName+"_futures"] = &common.Position{
		PairName:     pairName,
		Side:         "short",
		Market:       "futures",
		EntryPrice:   price,
		Quantity:     quantity,
		AmountUSDT:   amountUSDT,
		OrderID:      resp.Data.OrderID,
		ExchangeName: b.GetName(),
	}
	b.mu.Unlock()

	return &common.TradeResult{
		OrderID:       resp.Data.OrderID,
		ExecutedPrice: price,
		ExecutedQty:   quantity,
		Success:       true,
	}, nil
}

func (b *BitgetClient) getFuturesPositionInfo(ctx context.Context, symbol string, holdSide string) (*FuturesPositionInfo, error) {
	var r struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
		Data []struct {
			Symbol    string `json:"symbol"`
			Total     string `json:"total"`
			Available string `json:"available"`
			OpenAvg   string `json:"openAvgPrice"`
			HoldSide  string `json:"holdSide"`
		} `json:"data"`
	}

	body := map[string]interface{}{
		"symbol":      symbol,
		"productType": "USDT-FUTURES",
		"marginCoin":  "USDT",
		"holdSide":    holdSide, // Must specify which side we're querying
	}

	if err := b.signedRequest(ctx, "GET", "/api/v2/mix/position/single-position", body, &r); err != nil {
		return nil, err
	}

	if r.Code != "00000" {
		log.Printf("[BITGET] getFuturesPositionInfo - API error: %s - %s", r.Code, r.Msg)
		return nil, fmt.Errorf("bitget error: %s - %s", r.Code, r.Msg)
	}

	if len(r.Data) == 0 {
		log.Printf("[BITGET] getFuturesPositionInfo - No position found")
		return &FuturesPositionInfo{HoldSide: holdSide}, nil
	}

	total, _ := strconv.ParseFloat(r.Data[0].Total, 64)
	entry, _ := strconv.ParseFloat(r.Data[0].OpenAvg, 64)

	info := &FuturesPositionInfo{
		Total:    total,
		Entry:    entry,
		HoldSide: r.Data[0].HoldSide,
	}

	return info, nil
}

func (b *BitgetClient) CloseFuturesShort(ctx context.Context, pairName string) (*common.TradeResult, float64, error) {
	symbol := b.normalizeSymbol(pairName)

	// Get the actual position to verify it exists and get holdSide
	posInfo, err := b.getFuturesPositionInfo(ctx, symbol, "short")
	if err != nil {
		return nil, 0.00, err
	}
	if posInfo.Total == 0 {
		return nil, 0.00, fmt.Errorf("no open futures position for %s", symbol)
	}

	closeQty := posInfo.Total
	if closeQty < 0 {
		closeQty = -closeQty
	}

	closeQty = common.RoundQuantity(closeQty, pairName)
	if closeQty <= 0 {
		return nil, 0.00, fmt.Errorf("rounded close qty is zero")
	}

	body := map[string]interface{}{
		"symbol":      symbol,
		"productType": "USDT-FUTURES",
		"marginMode":  "crossed",
		"marginCoin":  "USDT",
		"size":        common.FormatQuantity(closeQty, pairName),
		"side":        "sell",
		"tradeSide":   "close",
		"orderType":   "market",
		"holdSide":    posInfo.HoldSide,
		"clientOid":   fmt.Sprintf("close_fut_%d", time.Now().UnixNano()),
	}

	var resp struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			OrderID   string `json:"orderId"`
			ClientOid string `json:"clientOid"`
		} `json:"data"`
	}

	if err := b.signedRequest(ctx, "POST", "/api/v2/mix/order/place-order", body, &resp); err != nil {
		return nil, 0.00, err
	}

	if resp.Code != "00000" {
		return nil, 0.00, fmt.Errorf("bitget error: %s - %s", resp.Code, resp.Msg)
	}

	b.mu.Lock()
	delete(b.positions, pairName+"_futures")
	b.mu.Unlock()

	newBalance, err := b.getFuturesBalance(ctx)
	if err != nil {
		log.Printf("[BITGET] CloseFuturesShort - ERROR: Failed to get USDT balance: %v", err)
		return nil, 0.00, fmt.Errorf("failed to get USDT balance: %w", err)
	}

	prevBalance := common.GetBalance(b.GetName(), "futures", "USDT")

	common.SetBalance(b.GetName(), "futures", "USDT", newBalance)

	return &common.TradeResult{
		OrderID:     resp.Data.OrderID,
		ExecutedQty: closeQty,
		Success:     true,
	}, newBalance - prevBalance, nil
}
