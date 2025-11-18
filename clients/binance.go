package clients

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// BinanceClient implements ExchangeTradeClient for Binance
type BinanceClient struct {
	apiKey      string
	apiSecret   string
	spotBaseURL string
	futsBaseURL string
	httpClient  *http.Client

	// Track open positions
	positions map[string]*Position
	posMutex  sync.RWMutex
}

// NewBinanceClient creates a new Binance trading client
func NewBinanceClient(apiKey, apiSecret string) *BinanceClient {
	return &BinanceClient{
		apiKey:      apiKey,
		apiSecret:   apiSecret,
		spotBaseURL: "https://api.binance.com",
		futsBaseURL: "https://fapi.binance.com",
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		positions: make(map[string]*Position),
	}
}

func (b *BinanceClient) GetName() string {
	return "binance"
}

// PutSpotLong buys the asset in spot market
func (b *BinanceClient) PutSpotLong(ctx context.Context, pairName string, amountUSDT float64) (*TradeResult, error) {
	symbol := b.normalizePairName(pairName, false)
	_, err := b.getSpotPrice(symbol)
	if err != nil {
		log.Printf("[BINANCE] PutSpotLong - ERROR: Failed to get spot price: %v", err)
		return nil, fmt.Errorf("failed to get spot price: %w", err)
	}
	// Note: quantity is calculated but we use quoteOrderQty to buy with exact USDT amount
	// Place market buy order
	params := url.Values{}
	params.Set("symbol", symbol)
	params.Set("side", "BUY")
	params.Set("type", "MARKET")
	params.Set("quoteOrderQty", fmt.Sprintf("%.8f", amountUSDT)) // Buy with USDT amount
	params.Set("timestamp", strconv.FormatInt(time.Now().UnixMilli(), 10))

	var orderResp struct {
		OrderID             int64  `json:"orderId"`
		ExecutedQty         string `json:"executedQty"`
		CummulativeQuoteQty string `json:"cummulativeQuoteQty"`
		Status              string `json:"status"`
		Fills               []struct {
			Price      string `json:"price"`
			Qty        string `json:"qty"`
			Commission string `json:"commission"`
		} `json:"fills"`
	}

	err = b.signedRequest(ctx, "POST", b.spotBaseURL+"/api/v3/order", params, &orderResp)
	if err != nil {
		log.Printf("[BINANCE] PutSpotLong - ERROR: Order failed: %v", err)
		return nil, fmt.Errorf("spot buy order failed: %w", err)
	}

	// Use CummulativeQuoteQty which is the actual USDT spent
	actualUSDTSpent, _ := strconv.ParseFloat(orderResp.CummulativeQuoteQty, 64)
	execQty, _ := strconv.ParseFloat(orderResp.ExecutedQty, 64)

	// Calculate average price and total fee
	var totalFee float64
	for _, fill := range orderResp.Fills {
		fee, _ := strconv.ParseFloat(fill.Commission, 64)
		totalFee += fee
	}
	avgPrice := actualUSDTSpent / execQty

	// Store position
	b.posMutex.Lock()
	b.positions[pairName+"_spot"] = &Position{
		PairName:     pairName,
		Side:         "long",
		Market:       "spot",
		EntryPrice:   avgPrice,
		Quantity:     execQty,
		AmountUSDT:   actualUSDTSpent, // Store actual USDT spent from CummulativeQuoteQty
		OrderID:      strconv.FormatInt(orderResp.OrderID, 10),
		ExchangeName: b.GetName(),
	}
	b.posMutex.Unlock()

	log.Printf("[BINANCE] PutSpotLong - Spent: %.6f USDT (bought %.8f %s, fee: %.8f in asset)", actualUSDTSpent, execQty, b.getBaseAsset(pairName), totalFee)

	return &TradeResult{
		OrderID:       strconv.FormatInt(orderResp.OrderID, 10),
		ExecutedPrice: avgPrice,
		ExecutedQty:   execQty,
		Fee:           totalFee,
		Success:       orderResp.Status == "FILLED",
		Message:       fmt.Sprintf("Spot long opened: bought %.8f at %.8f", execQty, avgPrice),
	}, nil
}

// PutFuturesShort opens a short position in futures market
func (b *BinanceClient) PutFuturesShort(ctx context.Context, pairName string, amountUSDT float64) (*TradeResult, error) {
	symbol := b.normalizePairName(pairName, true)

	// Get current price to calculate quantity
	price, err := b.getFuturesPrice(symbol)
	if err != nil {
		log.Printf("[BINANCE] PutFuturesShort - ERROR: Failed to get futures price: %v", err)
		return nil, fmt.Errorf("failed to get futures price: %w", err)
	}

	quantity := amountUSDT / price

	quantity = RoundQuantity(quantity, pairName)
	// Place market sell order (short)
	params := url.Values{}
	params.Set("symbol", symbol)
	params.Set("side", "SELL")
	params.Set("type", "MARKET")
	params.Set("quantity", FormatQuantity(quantity, pairName))
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
	b.positions[pairName+"_futures"] = &Position{
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

	return &TradeResult{
		OrderID:       strconv.FormatInt(orderResp.OrderID, 10),
		ExecutedPrice: avgPrice,
		ExecutedQty:   execQty,
		Fee:           0, // Futures API doesn't return fee in order response
		Success:       orderResp.Status == "FILLED",
		Message:       fmt.Sprintf("Futures short opened: sold %.3f at %.2f", execQty, avgPrice),
	}, nil
}

type Fill struct {
	Price      string
	Qty        string
	Commission string
}

// CloseSpotLong sells the asset back to USDT
func (b *BinanceClient) CloseSpotLong(ctx context.Context, pairName string, amountUSDT float64) (*TradeResult, error) {
	symbol := b.normalizePairName(pairName, false)

	// Extract base asset from symbol (e.g., "BTCUSDT" -> "BTC")
	baseAsset := b.getBaseAsset(pairName)

	// Get actual balance from Binance API
	balance, err := b.getSpotBalance(ctx, baseAsset)
	if err != nil {
		log.Printf("[BINANCE] CloseSpotLong - ERROR: Failed to get balance: %v", err)
		return nil, fmt.Errorf("failed to get balance: %w", err)
	}

	if balance == 0 {
		log.Printf("[BINANCE] CloseSpotLong - No balance found on exchange for %s", baseAsset)
		// Clean up local position tracking
		b.posMutex.Lock()
		delete(b.positions, pairName+"_spot")
		b.posMutex.Unlock()
		return nil, fmt.Errorf("no balance on exchange for %s", baseAsset)
	}

	closeQuantity := RoundQuantity(balance, pairName)

	if closeQuantity <= 0 {
		log.Printf("[BINANCE] CloseSpotLong - ERROR: Calculated quantity is zero or negative: %.8f", closeQuantity)
		return nil, fmt.Errorf("invalid close quantity: %.8f", closeQuantity)
	}

	// Store entry price from local tracking for PnL calculation
	var entryPrice float64
	var buySpent float64
	b.posMutex.RLock()
	if position, exists := b.positions[pairName+"_spot"]; exists {
		entryPrice = position.EntryPrice
		buySpent = position.AmountUSDT // Actual USDT spent on buy
	} else {
		// Fallback to parameter if position not in memory
		buySpent = amountUSDT
		log.Printf("[BINANCE] CloseSpotLong - WARNING: Position not found in memory, using parameter amountUSDT: %.6f", amountUSDT)
	}
	b.posMutex.RUnlock()

	// Place market sell order
	params := url.Values{}
	params.Set("symbol", symbol)
	params.Set("side", "SELL")
	params.Set("type", "MARKET")
	params.Set("quantity", FormatQuantity(closeQuantity, pairName))
	params.Set("timestamp", strconv.FormatInt(time.Now().UnixMilli(), 10))

	var orderResp struct {
		OrderID             int64  `json:"orderId"`
		ExecutedQty         string `json:"executedQty"`
		CummulativeQuoteQty string `json:"cummulativeQuoteQty"`
		Status              string `json:"status"`
		Fills               []Fill `json:"fills"`
	}

	err = b.signedRequest(ctx, "POST", b.spotBaseURL+"/api/v3/order", params, &orderResp)
	if err != nil {
		log.Printf("[BINANCE] CloseSpotLong - ERROR: Close order failed: %v", err)
		return nil, fmt.Errorf("spot close order failed: %w", err)
	}

	// Use CummulativeQuoteQty which is the actual USDT received
	actualUSDTReceived, _ := strconv.ParseFloat(orderResp.CummulativeQuoteQty, 64)
	execQty, _ := strconv.ParseFloat(orderResp.ExecutedQty, 64)

	// Calculate total fee (in USDT on sell side)
	var totalFee float64
	for _, fill := range orderResp.Fills {
		fee, _ := strconv.ParseFloat(fill.Commission, 64)
		totalFee += fee
	}

	// Net USDT received = CummulativeQuoteQty - fees
	netSell := actualUSDTReceived - totalFee
	profit := netSell - buySpent

	log.Printf("[BINANCE] CloseSpotLong - Buy spent: %.6f USDT, Sell received: %.6f USDT (gross: %.6f, fee: %.6f), Net Profit: %.6f USDT",
		buySpent, netSell, actualUSDTReceived, totalFee, profit)

	avgPrice := actualUSDTReceived / execQty

	// Remove position from local tracking
	b.posMutex.Lock()
	delete(b.positions, pairName+"_spot")
	b.posMutex.Unlock()

	// Calculate PnL if we have entry price
	var pnl float64
	var pnlMsg string
	if entryPrice > 0 {
		pnl = (avgPrice - entryPrice) / entryPrice * 100
		pnlMsg = fmt.Sprintf(" (PnL: %.2f%%)", pnl)
	}

	return &TradeResult{
		OrderID:       strconv.FormatInt(orderResp.OrderID, 10),
		ExecutedPrice: avgPrice,
		ExecutedQty:   execQty,
		Fee:           totalFee,
		Success:       orderResp.Status == "FILLED",
		Message:       fmt.Sprintf("Spot long closed: sold %.8f at %.8f%s", execQty, avgPrice, pnlMsg),
	}, nil
}

// CloseFuturesShort closes the short position by buying back
func (b *BinanceClient) CloseFuturesShort(ctx context.Context, pairName string) (*TradeResult, error) {
	symbol := b.normalizePairName(pairName, true)

	// Get actual position from Binance API
	positionRisk, err := b.getFuturesPositionRisk(ctx, symbol)
	if err != nil {
		log.Printf("[BINANCE] CloseFuturesShort - ERROR: Failed to get position risk: %v", err)
		return nil, fmt.Errorf("failed to get position risk: %w", err)
	}

	if positionRisk.PositionAmt == 0 {
		log.Printf("[BINANCE] CloseFuturesShort - No open position found on exchange for %s", symbol)
		// Clean up local position tracking
		b.posMutex.Lock()
		delete(b.positions, pairName+"_futures")
		b.posMutex.Unlock()
		return nil, fmt.Errorf("no open position on exchange")
	}

	// Calculate the quantity to close (absolute value of position amount)
	var closeQuantity float64
	if positionRisk.PositionAmt < 0 {
		closeQuantity = -positionRisk.PositionAmt
	} else {
		closeQuantity = positionRisk.PositionAmt
	}

	// Round quantity to step size
	closeQuantity = RoundQuantity(closeQuantity, pairName)

	if closeQuantity <= 0 {
		log.Printf("[BINANCE] CloseFuturesShort - ERROR: Calculated quantity is zero or negative: %.8f", closeQuantity)
		return nil, fmt.Errorf("invalid close quantity: %.8f", closeQuantity)
	}

	// Place market buy order to close short
	params := url.Values{}
	params.Set("symbol", symbol)
	params.Set("side", "BUY")
	params.Set("type", "MARKET")
	params.Set("quantity", FormatQuantity(closeQuantity, pairName))
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
		return nil, fmt.Errorf("futures close order failed: %w", err)
	}

	execQty, _ := strconv.ParseFloat(orderResp.ExecutedQty, 64)
	avgPrice, _ := strconv.ParseFloat(orderResp.AvgPrice, 64)

	// Remove position from local tracking
	b.posMutex.Lock()
	delete(b.positions, pairName+"_futures")
	b.posMutex.Unlock()

	pnl := (positionRisk.EntryPrice - avgPrice) / positionRisk.EntryPrice * 100

	return &TradeResult{
		OrderID:       strconv.FormatInt(orderResp.OrderID, 10),
		ExecutedPrice: avgPrice,
		ExecutedQty:   execQty,
		Fee:           0,
		Success:       orderResp.Status == "FILLED",
		Message:       fmt.Sprintf("Futures short closed: bought %.3f at %.2f (PnL: %.2f%%)", execQty, avgPrice, pnl),
	}, nil
}

// Helper: normalize pair name to Binance format
func (b *BinanceClient) normalizePairName(pairName string, isFutures bool) string {
	// Convert "btc-usdt" to "BTCUSDT"
	parts := strings.Split(strings.ToUpper(pairName), "-")
	symbol := strings.Join(parts, "")

	// Futures symbols may need adjustment (some use different naming)
	// For most cases on Binance, futures perpetual contracts are the same as spot

	return symbol
}

// Helper: get current spot price
func (b *BinanceClient) getSpotPrice(symbol string) (float64, error) {
	url := fmt.Sprintf("%s/api/v3/ticker/price?symbol=%s", b.spotBaseURL, symbol)

	resp, err := http.Get(url)
	if err != nil {
		log.Printf("[BINANCE] getSpotPrice - ERROR: HTTP request failed: %v", err)
		return 0, err
	}
	defer resp.Body.Close()

	var result struct {
		Price string `json:"price"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("[BINANCE] getSpotPrice - ERROR: JSON decode failed: %v", err)
		return 0, err
	}

	price, err := strconv.ParseFloat(result.Price, 64)
	if err != nil {
		log.Printf("[BINANCE] getSpotPrice - ERROR: Price parse failed: %v", err)
		return 0, err
	}

	return price, nil
}

// Helper: get current futures price
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

// Helper: extract base asset from pair name
func (b *BinanceClient) getBaseAsset(pairName string) string {
	// Convert "btc-usdt" to "BTC"
	parts := strings.Split(strings.ToUpper(pairName), "-")
	if len(parts) > 0 {
		return parts[0]
	}
	return ""
}

// AccountBalance represents account balance from Binance
type AccountBalance struct {
	Asset  string `json:"asset"`
	Free   string `json:"free"`
	Locked string `json:"locked"`
}

// AccountInfo represents account information from Binance
type AccountInfo struct {
	Balances []AccountBalance `json:"balances"`
}

// Helper: get spot balance for an asset
func (b *BinanceClient) getSpotBalance(ctx context.Context, asset string) (float64, error) {
	params := url.Values{}
	params.Set("timestamp", strconv.FormatInt(time.Now().UnixMilli(), 10))

	var accountInfo AccountInfo
	err := b.signedRequest(ctx, "GET", b.spotBaseURL+"/api/v3/account", params, &accountInfo)
	if err != nil {
		log.Printf("[BINANCE] getSpotBalance - ERROR: Request failed: %v", err)
		return 0, err
	}

	// Find the balance for the asset
	for _, balance := range accountInfo.Balances {
		if balance.Asset == asset {
			free, _ := strconv.ParseFloat(balance.Free, 64)
			// Return free balance (available to sell)
			return free, nil
		}
	}

	return 0, nil
}

type PositionRisk struct {
	Symbol           string  `json:"symbol"`
	PositionAmt      float64 `json:"positionAmt,string"`
	EntryPrice       float64 `json:"entryPrice,string"`
	MarkPrice        float64 `json:"markPrice,string"`
	UnrealizedProfit float64 `json:"unRealizedProfit,string"`
	LiquidationPrice float64 `json:"liquidationPrice,string"`
	Leverage         float64 `json:"leverage,string"`
	PositionSide     string  `json:"positionSide"`
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

func (b *BinanceClient) signedRequest(ctx context.Context, method, endpoint string, params url.Values, result interface{}) error {
	// Sign the request
	queryString := params.Encode()
	h := hmac.New(sha256.New, []byte(b.apiSecret))
	h.Write([]byte(queryString))
	signature := hex.EncodeToString(h.Sum(nil))

	queryString += "&signature=" + signature

	var req *http.Request
	var err error

	if method == "POST" {
		req, err = http.NewRequestWithContext(ctx, method, endpoint, strings.NewReader(queryString))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		req, err = http.NewRequestWithContext(ctx, method, endpoint+"?"+queryString, nil)
		if err != nil {
			return err
		}
	}

	req.Header.Set("X-MBX-APIKEY", b.apiKey)

	resp, err := b.httpClient.Do(req)
	if err != nil {
		log.Printf("[BINANCE] signedRequest - ERROR: HTTP request failed: %v", err)
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[BINANCE] signedRequest - ERROR: Failed to read response body: %v", err)
		return err
	}

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
		}
		json.Unmarshal(body, &errResp)
		return fmt.Errorf("binance API error %d: %s", errResp.Code, errResp.Msg)
	}

	err = json.Unmarshal(body, result)
	if err != nil {
		return err
	}

	return nil
}
