package clients

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// BitgetClient implements ExchangeTradeClient for Bitget using v2 API
type BitgetClient struct {
	apiKey     string
	apiSecret  string
	passphrase string
	baseURL    string
	httpClient *http.Client
	positions  map[string]*Position
	mu         sync.RWMutex
}

func NewBitgetClient(apiKey, apiSecret, passphrase string) *BitgetClient {
	return &BitgetClient{
		apiKey:     apiKey,
		apiSecret:  apiSecret,
		passphrase: passphrase,
		baseURL:    "https://api.bitget.com",
		httpClient: &http.Client{Timeout: 30 * time.Second},
		positions:  make(map[string]*Position),
	}
}

func (b *BitgetClient) GetName() string { return "bitget" }

// PutSpotLong places a market buy on spot
func (b *BitgetClient) PutSpotLong(ctx context.Context, pairName string, amountUSDT float64) (*TradeResult, error) {
	symbol := b.normalizeSymbol(pairName)

	// Get ticker price for reference
	price, err := b.getSpotTicker(ctx, symbol)
	if err != nil {
		log.Printf("[BITGET] PutSpotLong - ticker error: %v", err)
		return nil, err
	}
	estimatedQty := amountUSDT / price

	qty := RoundQuantity(estimatedQty, pairName)
	if qty <= 0 {
		return nil, fmt.Errorf("calculated quantity is zero after rounding")
	}

	body := map[string]interface{}{
		"symbol":    symbol,
		"side":      "buy",
		"orderType": "market",
		"force":     "gtc",
		"size":      FormatQuantity(qty, pairName),
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
	b.positions[pairName+"_spot"] = &Position{
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

	return &TradeResult{
		OrderID:       resp.Data.OrderID,
		ExecutedPrice: price,
		ExecutedQty:   qty,
		Success:       true,
		Message:       fmt.Sprintf("Spot buy placed: %.8f at %.8f", qty, price),
	}, nil
}

// PutFuturesShort opens a futures short using v2 API
func (b *BitgetClient) PutFuturesShort(ctx context.Context, pairName string, amountUSDT float64) (*TradeResult, error) {
	symbol := b.normalizeFuturesSymbol(pairName)

	price, err := b.getFuturesTicker(ctx, symbol)
	if err != nil {
		return nil, err
	}
	quantity := amountUSDT / price
	quantity = RoundQuantity(quantity, pairName)
	if quantity <= 0 {
		return nil, fmt.Errorf("calculated futures quantity is zero")
	}

	body := map[string]interface{}{
		"symbol":      symbol,
		"productType": "USDT-FUTURES",
		"marginMode":  "crossed",
		"marginCoin":  "USDT",
		"size":        FormatQuantity(quantity, pairName),
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
	b.positions[pairName+"_futures"] = &Position{
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

	return &TradeResult{
		OrderID:       resp.Data.OrderID,
		ExecutedPrice: price,
		ExecutedQty:   quantity,
		Success:       true,
		Message:       fmt.Sprintf("Futures short placed: %.8f at %.2f", quantity, price),
	}, nil
}

// CloseSpotLong sells back the asset using actual balance
func (b *BitgetClient) CloseSpotLong(ctx context.Context, pairName string, amountUSDT float64) (*TradeResult, error) {
	symbol := b.normalizeSymbol(pairName)

	// Get actual asset balance
	asset := strings.TrimSuffix(symbol, "USDT")
	bal, err := b.getSpotAssetBalance(ctx, asset)
	if err != nil {
		return nil, err
	}
	if bal <= 0 {
		return nil, fmt.Errorf("no balance for asset %s", asset)
	}

	qty := RoundQuantity(bal, pairName)
	if qty <= 0 {
		return nil, fmt.Errorf("rounded qty is zero")
	}

	body := map[string]interface{}{
		"symbol":    symbol,
		"side":      "sell",
		"orderType": "market",
		"force":     "gtc",
		"size":      FormatQuantity(qty, pairName),
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
		return nil, err
	}

	if resp.Code != "00000" {
		return nil, fmt.Errorf("bitget error: %s - %s", resp.Code, resp.Msg)
	}

	b.mu.Lock()
	delete(b.positions, pairName+"_spot")
	b.mu.Unlock()

	return &TradeResult{
		OrderID:     resp.Data.OrderID,
		ExecutedQty: qty,
		Success:     true,
		Message:     fmt.Sprintf("Spot closed: %.8f", qty),
	}, nil
}

// CloseFuturesShort closes futures using actual position
func (b *BitgetClient) CloseFuturesShort(ctx context.Context, pairName string) (*TradeResult, error) {
	symbol := b.normalizeFuturesSymbol(pairName)

	// Get the actual position to verify it exists and get holdSide
	posInfo, err := b.getFuturesPositionInfo(ctx, symbol, "short")
	if err != nil {
		return nil, err
	}
	if posInfo.Total == 0 {
		return nil, fmt.Errorf("no open futures position for %s", symbol)
	}

	closeQty := posInfo.Total
	if closeQty < 0 {
		closeQty = -closeQty
	}

	closeQty = RoundQuantity(closeQty, pairName)
	if closeQty <= 0 {
		return nil, fmt.Errorf("rounded close qty is zero")
	}

	body := map[string]interface{}{
		"symbol":      symbol,
		"productType": "USDT-FUTURES",
		"marginMode":  "crossed",
		"marginCoin":  "USDT",
		"size":        FormatQuantity(closeQty, pairName),
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
		return nil, err
	}

	if resp.Code != "00000" {
		return nil, fmt.Errorf("bitget error: %s - %s", resp.Code, resp.Msg)
	}

	b.mu.Lock()
	delete(b.positions, pairName+"_futures")
	b.mu.Unlock()

	return &TradeResult{
		OrderID:     resp.Data.OrderID,
		ExecutedQty: closeQty,
		Success:     true,
		Message:     fmt.Sprintf("Futures closed: %.8f", closeQty),
	}, nil
}

// --- Helper Functions ---

func (b *BitgetClient) normalizeSymbol(pairName string) string {
	// Convert "btc-usdt" to "BTCUSDT"
	return strings.ToUpper(strings.ReplaceAll(pairName, "-", ""))
}

func (b *BitgetClient) normalizeFuturesSymbol(pairName string) string {
	// Convert "btc-usdt" to "BTCUSDT"
	return strings.ToUpper(strings.ReplaceAll(pairName, "-", ""))
}

func (b *BitgetClient) getSpotTicker(ctx context.Context, symbol string) (float64, error) {
	url := fmt.Sprintf("%s/api/v2/spot/market/tickers?symbol=%s", b.baseURL, symbol)

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
	if len(r.Data) == 0 {
		return 0, fmt.Errorf("no ticker data")
	}
	p, _ := strconv.ParseFloat(r.Data[0].LastPr, 64)

	return p, nil
}

func (b *BitgetClient) getFuturesTicker(ctx context.Context, symbol string) (float64, error) {
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

type FuturesPositionInfo struct {
	Total    float64
	Entry    float64
	HoldSide string
}

func (b *BitgetClient) signedRequest(ctx context.Context, method, path string, body interface{}, out interface{}) error {
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)

	var bodyStr string
	if body != nil {
		if method == "GET" {
			// For GET requests, convert body to query string
			if m, ok := body.(map[string]interface{}); ok {
				params := []string{}
				for k, v := range m {
					params = append(params, fmt.Sprintf("%s=%v", k, v))
				}
				if len(params) > 0 {
					bodyStr = ""
					path = path + "?" + strings.Join(params, "&")
				}
			}
		} else {
			// For POST requests, use JSON body
			bodyBytes, _ := json.Marshal(body)
			bodyStr = string(bodyBytes)
		}
	}

	// Bitget signature format: timestamp + method + path + body
	preHash := timestamp + method + path + bodyStr

	mac := hmac.New(sha256.New, []byte(b.apiSecret))
	mac.Write([]byte(preHash))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	url := b.baseURL + path
	var req *http.Request
	var err error

	if method == "GET" || bodyStr == "" {
		req, err = http.NewRequestWithContext(ctx, method, url, nil)
	} else {
		req, err = http.NewRequestWithContext(ctx, method, url, strings.NewReader(bodyStr))
	}

	if err != nil {
		return err
	}

	// Bitget v2 API headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("ACCESS-KEY", b.apiKey)
	req.Header.Set("ACCESS-SIGN", signature)
	req.Header.Set("ACCESS-TIMESTAMP", timestamp)
	req.Header.Set("ACCESS-PASSPHRASE", b.passphrase)
	req.Header.Set("locale", "en-US")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		log.Printf("[BITGET] signedRequest - HTTP error: %v", err)
		return err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return fmt.Errorf("bitget api error: status %d, body: %s", resp.StatusCode, string(respBody))
	}

	if out != nil {
		return json.Unmarshal(respBody, out)
	}
	return nil
}
