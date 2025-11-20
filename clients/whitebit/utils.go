package whitebit

import (
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

func (w *WhitebitClient) normalizeSymbol(pairName string) string {
	// Convert "btc-usdt" to "BTC_USDT"
	parts := strings.Split(strings.ToUpper(pairName), "-")
	return strings.Join(parts, "_")
}

func (w *WhitebitClient) normalizeSymbolFutures(pairName string) string {
	// Convert "btc-usdt" to "BTC_USDT"
	perpPairName := strings.Replace(pairName, "-usdt", "-perp", 1)
	parts := strings.Split(strings.ToUpper(perpPairName), "-")
	return strings.Join(parts, "_")
}

func (w *WhitebitClient) signedRequest(ctx context.Context, endpoint string, params map[string]interface{}, result interface{}) error {
	// Acquire rate limit token - blocks until available
	<-w.rateLimiter
	defer func() {
		// Release token after request completes
		time.Sleep(50 * time.Millisecond) // Small delay between requests
		w.rateLimiter <- struct{}{}
	}()

	nonce := time.Now().UnixMilli()

	params["request"] = endpoint
	params["nonce"] = nonce

	bodyBytes, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("failed to marshal params: %w", err)
	}

	payload := base64.StdEncoding.EncodeToString(bodyBytes)

	h := hmac.New(sha512.New, []byte(w.apiSecret))
	h.Write([]byte(payload))
	signature := hex.EncodeToString(h.Sum(nil))

	req, err := http.NewRequestWithContext(ctx, "POST", w.baseURL+endpoint, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-TXC-APIKEY", w.apiKey)
	req.Header.Set("X-TXC-PAYLOAD", payload)
	req.Header.Set("X-TXC-SIGNATURE", signature)

	resp, err := w.httpClient.Do(req)
	if err != nil {
		log.Printf("[WHITEBIT] signedRequest - HTTP error: %v", err)
		return fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("whitebit api error: status %d, body: %s", resp.StatusCode, string(body))
	}

	if result != nil {
		// var prettyJson bytes.Buffer
		// if err := json.Indent(&prettyJson, body, "", "  "); err != nil {
		// 	return fmt.Errorf("failed to indent json: %w", err)
		// }

		// fmt.Println(prettyJson.String())

		if err := json.Unmarshal(body, result); err != nil {
			return fmt.Errorf("failed to unmarshal response: %w", err)
		}
	}

	return nil
}

func (w *WhitebitClient) getPrice(ctx context.Context, market string) (float64, error) {
	url := fmt.Sprintf("%s/api/v4/public/ticker", w.baseURL)

	resp, err := http.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	var tickers map[string]struct {
		LastPrice string `json:"last_price"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tickers); err != nil {
		return 0, err
	}

	ticker, ok := tickers[market]
	if !ok {
		return 0, fmt.Errorf("market %s not found", market)
	}

	var price float64
	fmt.Sscanf(ticker.LastPrice, "%f", &price)
	return price, nil
}
