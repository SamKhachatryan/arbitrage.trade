package okx

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (o *OkxClient) normalizeSymbol(pairName string) string {
	// Convert "btc-usdt" to "BTC-USDT"
	return strings.ToUpper(pairName)
}

func (o *OkxClient) normalizeSymbolFutures(pairName string) string {
	// Convert "btc-usdt" to "BTC-USDT-SWAP"
	return strings.ToUpper(pairName) + "-SWAP"
}

func (o *OkxClient) signedRequest(ctx context.Context, method, endpoint, body string, result interface{}) error {
	timestamp := time.Now().UTC().Format("2006-01-02T15:04:05.999Z")

	// OKX signature: base64(HMAC-SHA256(timestamp + method + endpoint + body, secret))
	preHash := timestamp + method + endpoint + body

	h := hmac.New(sha256.New, []byte(o.apiSecret))
	h.Write([]byte(preHash))
	signature := base64.StdEncoding.EncodeToString(h.Sum(nil))

	url := o.baseURL + endpoint
	req, err := http.NewRequestWithContext(ctx, method, url, strings.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("OK-ACCESS-KEY", o.apiKey)
	req.Header.Set("OK-ACCESS-SIGN", signature)
	req.Header.Set("OK-ACCESS-TIMESTAMP", timestamp)
	req.Header.Set("OK-ACCESS-PASSPHRASE", o.passphrase)

	resp, err := o.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("okx api error: status %d, body: %s", resp.StatusCode, string(responseBody))
	}

	if result != nil {
		if err := json.Unmarshal(responseBody, result); err != nil {
			return fmt.Errorf("failed to unmarshal response: %w", err)
		}
	}

	return nil
}

func (o *OkxClient) getPrice(ctx context.Context, instId string) (float64, error) {
	url := fmt.Sprintf("%s/api/v5/market/ticker?instId=%s", o.baseURL, instId)

	resp, err := http.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	var result struct {
		Data []struct {
			Last string `json:"last"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, err
	}

	if len(result.Data) == 0 {
		return 0, fmt.Errorf("no ticker data for %s", instId)
	}

	price, _ := strconv.ParseFloat(result.Data[0].Last, 64)
	return price, nil
}
