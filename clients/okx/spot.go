package okx

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"arbitrage.trade/clients/common"
)

func (o *OkxClient) getSpotBalance(ctx context.Context, ccy string) (float64, error) {
	var result struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
		Data []struct {
			Details []Balance `json:"details"`
		} `json:"data"`
	}

	// OKX unified account endpoint - gets balance with details array
	if err := o.signedRequest(ctx, "GET", "/api/v5/account/balance?ccy="+ccy, "", &result); err != nil {
		return 0, fmt.Errorf("failed to get balance: %w", err)
	}

	if result.Code != "0" {
		return 0, fmt.Errorf("okx error code: %s, msg: %s", result.Code, result.Msg)
	}

	if len(result.Data) > 0 && len(result.Data[0].Details) > 0 {
		for _, detail := range result.Data[0].Details {
			if detail.Ccy == ccy {
				available, _ := strconv.ParseFloat(detail.AvailBal, 64)
				return available, nil
			}
		}
	}

	return 0, nil
}

func (o *OkxClient) PutSpotLong(ctx context.Context, pairName string, amountUSDT float64) (*common.TradeResult, error) {
	instId := o.normalizeSymbol(pairName)

	balance, err := o.getSpotBalance(ctx, "USDT")
	if err != nil {
		return nil, fmt.Errorf("failed to get USDT balance: %w", err)
	}

	common.SetBalance(o.GetName(), "spot", "USDT", balance)

	orderReq := map[string]interface{}{
		"instId":  instId,
		"tdMode":  "cash",
		"side":    "buy",
		"ordType": "market",
		"sz":      fmt.Sprintf("%.8f", amountUSDT),
		"tgtCcy":  "quote_ccy",
	}

	body, _ := json.Marshal(orderReq)

	var result struct {
		Code string          `json:"code"`
		Msg  string          `json:"msg"`
		Data []OrderResponse `json:"data"`
	}

	if err := o.signedRequest(ctx, "POST", "/api/v5/trade/order", string(body), &result); err != nil {
		return nil, fmt.Errorf("market order failed: %w", err)
	}

	if result.Code != "0" {
		msg := result.Msg
		if len(result.Data) > 0 && result.Data[0].SMsg != "" {
			msg = result.Data[0].SMsg
		}
		return nil, fmt.Errorf("order failed: code %s, msg: %s", result.Code, msg)
	}

	if len(result.Data) == 0 {
		return nil, fmt.Errorf("order response empty")
	}

	orderData := result.Data[0]
	orderId := orderData.OrdId

	// OKX market orders fill asynchronously, query for fill details
	time.Sleep(200 * time.Millisecond)

	var orderQueryResult struct {
		Code string `json:"code"`
		Data []struct {
			AvgPx     string `json:"avgPx"`
			AccFillSz string `json:"accFillSz"`
			Fee       string `json:"fee"`
			State     string `json:"state"`
		} `json:"data"`
	}

	queryEndpoint := fmt.Sprintf("/api/v5/trade/order?instId=%s&ordId=%s", instId, orderId)
	if err := o.signedRequest(ctx, "GET", queryEndpoint, "", &orderQueryResult); err == nil && len(orderQueryResult.Data) > 0 {
		orderData.AvgPx = orderQueryResult.Data[0].AvgPx
		orderData.AccFillSz = orderQueryResult.Data[0].AccFillSz
		orderData.Fee = orderQueryResult.Data[0].Fee
		orderData.State = orderQueryResult.Data[0].State
	}

	avgPx, _ := strconv.ParseFloat(orderData.AvgPx, 64)
	fillSz, _ := strconv.ParseFloat(orderData.AccFillSz, 64)
	fee, _ := strconv.ParseFloat(orderData.Fee, 64)

	o.mu.Lock()
	o.positions[pairName+"_spot"] = &common.Position{
		PairName:     pairName,
		Side:         "long",
		Market:       "spot",
		EntryPrice:   avgPx,
		Quantity:     fillSz,
		AmountUSDT:   amountUSDT,
		OrderID:      orderId,
		ExchangeName: o.GetName(),
	}
	o.mu.Unlock()

	return &common.TradeResult{
		OrderID:       orderId,
		ExecutedPrice: avgPx,
		ExecutedQty:   fillSz,
		Fee:           fee,
		Success:       orderData.State == "filled",
	}, nil
}

func (o *OkxClient) CloseSpotLong(ctx context.Context, pairName string, amountUSDT float64) (*common.TradeResult, float64, error) {
	instId := o.normalizeSymbol(pairName)

	o.mu.RLock()
	_, exists := o.positions[pairName+"_spot"]
	o.mu.RUnlock()

	if !exists {
		return nil, 0.0, fmt.Errorf("no position found for %s", pairName)
	}

	baseAsset := strings.Split(instId, "-")[0]
	balance, err := o.getSpotBalance(ctx, baseAsset)
	if err != nil {
		return nil, 0.0, fmt.Errorf("failed to get %s balance: %w", baseAsset, err)
	}

	if common.IsNegativeOrZero(balance) {
		return nil, 0.0, fmt.Errorf("no %s balance to sell", baseAsset)
	}

	sellQuantity := common.RoundQuantity(balance, pairName)

	orderReq := map[string]interface{}{
		"instId":  instId,
		"tdMode":  "cash",
		"side":    "sell",
		"ordType": "market",
		"sz":      common.FormatQuantity(sellQuantity, pairName),
	}

	body, _ := json.Marshal(orderReq)

	var result struct {
		Code string          `json:"code"`
		Msg  string          `json:"msg"`
		Data []OrderResponse `json:"data"`
	}

	if err := o.signedRequest(ctx, "POST", "/api/v5/trade/order", string(body), &result); err != nil {
		return nil, 0.0, fmt.Errorf("market order failed: %w", err)
	}

	if result.Code != "0" {
		msg := result.Msg
		if len(result.Data) > 0 && result.Data[0].SMsg != "" {
			msg = result.Data[0].SMsg
		}
		return nil, 0.0, fmt.Errorf("order failed: code %s, msg: %s", result.Code, msg)
	}

	if len(result.Data) == 0 {
		return nil, 0.0, fmt.Errorf("order response empty")
	}

	orderData := result.Data[0]
	orderId := orderData.OrdId

	// OKX market orders fill asynchronously, query for fill details
	time.Sleep(200 * time.Millisecond)

	var orderQueryResult struct {
		Code string `json:"code"`
		Data []struct {
			AvgPx     string `json:"avgPx"`
			AccFillSz string `json:"accFillSz"`
			Fee       string `json:"fee"`
			State     string `json:"state"`
		} `json:"data"`
	}

	queryEndpoint := fmt.Sprintf("/api/v5/trade/order?instId=%s&ordId=%s", instId, orderId)
	if err := o.signedRequest(ctx, "GET", queryEndpoint, "", &orderQueryResult); err == nil && len(orderQueryResult.Data) > 0 {
		orderData.AvgPx = orderQueryResult.Data[0].AvgPx
		orderData.AccFillSz = orderQueryResult.Data[0].AccFillSz
		orderData.Fee = orderQueryResult.Data[0].Fee
		orderData.State = orderQueryResult.Data[0].State
	}

	avgPx, _ := strconv.ParseFloat(orderData.AvgPx, 64)
	fillSz, _ := strconv.ParseFloat(orderData.AccFillSz, 64)
	fee, _ := strconv.ParseFloat(orderData.Fee, 64)

	o.mu.Lock()
	delete(o.positions, pairName+"_spot")
	o.mu.Unlock()

	newBalance, err := o.getSpotBalance(ctx, "USDT")
	if err != nil {
		return nil, 0.0, fmt.Errorf("failed to get USDT balance: %w", err)
	}

	prevBalance := common.GetBalance(o.GetName(), "spot", "USDT")
	common.SetBalance(o.GetName(), "spot", "USDT", newBalance)

	profit := newBalance - prevBalance

	return &common.TradeResult{
		OrderID:       orderId,
		ExecutedPrice: avgPx,
		ExecutedQty:   fillSz,
		Fee:           fee,
		Success:       orderData.State == "filled",
	}, profit, nil
}
