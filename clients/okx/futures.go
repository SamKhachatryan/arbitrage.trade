package okx

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"arbitrage.trade/clients/common"
)

func (o *OkxClient) getFuturesBalance(ctx context.Context) (float64, error) {
	var result struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
		Data []struct {
			TotalEq string `json:"totalEq"`
			Details []struct {
				Ccy           string `json:"ccy"`
				AvailBal      string `json:"availBal"`
				AvailEq       string `json:"availEq"`
				CashBal       string `json:"cashBal"`
				Eq            string `json:"eq"`
				FrozenBal     string `json:"frozenBal"`
				OrderFrozen   string `json:"ordFrozen"`
				DisEq         string `json:"disEq"`
				AvailOrderBal string `json:"availOrderBal,omitempty"`
			} `json:"details"`
		} `json:"data"`
	}

	if err := o.signedRequest(ctx, "GET", "/api/v5/account/balance?ccy=USDT", "", &result); err != nil {
		return 0, fmt.Errorf("failed to get futures balance: %w", err)
	}

	if result.Code != "0" {
		return 0, fmt.Errorf("okx error code: %s, msg: %s", result.Code, result.Msg)
	}

	if len(result.Data) > 0 && len(result.Data[0].Details) > 0 {
		for _, detail := range result.Data[0].Details {
			if detail.Ccy == "USDT" {
				// Try availEq first (available equity for trading), then availBal
				available := 0.0
				if detail.AvailEq != "" {
					available, _ = strconv.ParseFloat(detail.AvailEq, 64)
				}
				if available == 0 && detail.AvailBal != "" {
					available, _ = strconv.ParseFloat(detail.AvailBal, 64)
				}
				if available == 0 && detail.CashBal != "" {
					available, _ = strconv.ParseFloat(detail.CashBal, 64)
				}
				return available, nil
			}
		}
	}

	return 0, nil
}

func (o *OkxClient) getFuturesPosition(ctx context.Context, instId string) (*PositionData, error) {
	var result struct {
		Code string         `json:"code"`
		Data []PositionData `json:"data"`
	}

	endpoint := fmt.Sprintf("/api/v5/account/positions?instId=%s", instId)
	if err := o.signedRequest(ctx, "GET", endpoint, "", &result); err != nil {
		return nil, err
	}

	if result.Code != "0" {
		return nil, fmt.Errorf("okx error code: %s", result.Code)
	}

	for _, pos := range result.Data {
		if pos.InstId == instId && pos.Pos != "0" {
			return &pos, nil
		}
	}

	return nil, nil
}

func (o *OkxClient) PutFuturesShort(ctx context.Context, pairName string, amountUSDT float64) (*common.TradeResult, error) {
	instId := o.normalizeSymbolFutures(pairName)

	// Set leverage to 10x for this instrument
	leverageReq := map[string]interface{}{
		"instId":  instId,
		"lever":   "10",
		"mgnMode": "cross",
	}
	leverageBody, _ := json.Marshal(leverageReq)

	var leverageResult struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
	}

	// Try to set leverage, ignore error if already set
	_ = o.signedRequest(ctx, "POST", "/api/v5/account/set-leverage", string(leverageBody), &leverageResult)

	balance, err := o.getFuturesBalance(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get futures balance: %w", err)
	}

	common.SetBalance(o.GetName(), "futures", "USDT", balance)

	// OKX SWAP contracts use USDT as the contract size
	// For most USDT perpetuals, 1 contract = 1 USDT
	quantity := amountUSDT
	if quantity < 1 {
		quantity = 1
	}

	orderReq := map[string]interface{}{
		"instId":  instId,
		"tdMode":  "cross",
		"side":    "sell",
		"ordType": "market",
		"sz":      fmt.Sprintf("%.0f", quantity),
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
	o.positions[pairName+"_futures"] = &common.Position{
		PairName:     pairName,
		Side:         "short",
		Market:       "futures",
		EntryPrice:   avgPx,
		Quantity:     fillSz,
		AmountUSDT:   fillSz * avgPx,
		OrderID:      orderId,
		ExchangeName: o.GetName(),
	}
	o.mu.Unlock()

	return &common.TradeResult{
		OrderID:       orderData.OrdId,
		ExecutedPrice: avgPx,
		ExecutedQty:   fillSz,
		Fee:           fee,
		Success:       orderData.State == "filled",
	}, nil
}

func (o *OkxClient) CloseFuturesShort(ctx context.Context, pairName string) (*common.TradeResult, float64, error) {
	instId := o.normalizeSymbolFutures(pairName)

	position, err := o.getFuturesPosition(ctx, instId)
	if err != nil {
		return nil, 0.0, fmt.Errorf("failed to get position: %w", err)
	}

	if position == nil {
		o.mu.Lock()
		delete(o.positions, pairName+"_futures")
		o.mu.Unlock()
		return nil, 0.0, fmt.Errorf("no open position on exchange")
	}

	pos, _ := strconv.ParseFloat(position.Pos, 64)
	closeQuantity := pos
	if closeQuantity < 0 {
		closeQuantity = -closeQuantity
	}

	if closeQuantity <= 0 {
		return nil, 0.0, fmt.Errorf("no position to close")
	}

	prevBalance := common.GetBalance(o.GetName(), "futures", "USDT")

	orderReq := map[string]interface{}{
		"instId":  instId,
		"tdMode":  "cross",
		"side":    "buy",
		"ordType": "market",
		"sz":      fmt.Sprintf("%.0f", closeQuantity),
	}

	body, _ := json.Marshal(orderReq)

	var result struct {
		Code string          `json:"code"`
		Msg  string          `json:"msg"`
		Data []OrderResponse `json:"data"`
	}

	if err := o.signedRequest(ctx, "POST", "/api/v5/trade/order", string(body), &result); err != nil {
		return nil, 0.0, fmt.Errorf("close order failed: %w", err)
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

	time.Sleep(200 * time.Millisecond)

	var orderQueryResult struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
		Data []struct {
			OrdId     string `json:"ordId"`
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

	if fee < 0 {
		fee = -fee
	}

	newBalance, _ := o.getFuturesBalance(ctx)
	profit := newBalance - prevBalance

	o.mu.Lock()
	delete(o.positions, pairName+"_futures")
	o.mu.Unlock()

	return &common.TradeResult{
		OrderID:       orderData.OrdId,
		ExecutedPrice: avgPx,
		ExecutedQty:   fillSz,
		Fee:           fee,
		Success:       orderData.State == "filled",
	}, profit, nil
}
