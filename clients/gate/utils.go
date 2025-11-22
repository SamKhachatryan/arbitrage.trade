package gate

import (
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (g *GateClient) normalizeSymbol(pairName string) string {
	// Convert "btc-usdt" to "BTC_USDT"
	parts := strings.Split(strings.ToUpper(pairName), "-")
	return strings.Join(parts, "_")
}

func (g *GateClient) normalizeSymbolFutures(pairName string) string {
	// Convert "btc-usdt" to "BTC_USDT"
	parts := strings.Split(strings.ToUpper(pairName), "-")
	return strings.Join(parts, "_")
}

func (g *GateClient) signedRequest(ctx context.Context, method, endpoint string, body string, result interface{}) error {
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)

	// Gate.io signature: HMAC-SHA512(method + '\n' + endpoint + '\n' + query_string + '\n' + body_hash + '\n' + timestamp)
	bodyHash := sha512.Sum512([]byte(body))
	bodyHashHex := hex.EncodeToString(bodyHash[:])

	signString := fmt.Sprintf("%s\n%s\n\n%s\n%s", method, endpoint, bodyHashHex, timestamp)

	h := hmac.New(sha512.New, []byte(g.apiSecret))
	h.Write([]byte(signString))
	signature := hex.EncodeToString(h.Sum(nil))

	url := g.baseURL + endpoint
	req, err := http.NewRequestWithContext(ctx, method, url, strings.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("KEY", g.apiKey)
	req.Header.Set("SIGN", signature)
	req.Header.Set("Timestamp", timestamp)

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("gate api error: status %d, body: %s", resp.StatusCode, string(responseBody))
	}

	if result != nil {
		if err := json.Unmarshal(responseBody, result); err != nil {
			return fmt.Errorf("failed to unmarshal response: %w", err)
		}
	}

	return nil
}

func (g *GateClient) getPrice(ctx context.Context, symbol string) (float64, error) {
	url := fmt.Sprintf("%s/api/v4/spot/tickers?currency_pair=%s", g.baseURL, symbol)

	resp, err := http.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	var tickers []struct {
		Last string `json:"last"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tickers); err != nil {
		return 0, err
	}

	if len(tickers) == 0 {
		return 0, fmt.Errorf("no ticker data for %s", symbol)
	}

	price, _ := strconv.ParseFloat(tickers[0].Last, 64)
	return price, nil
}
