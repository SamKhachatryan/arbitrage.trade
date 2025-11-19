package binance

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
	"strings"
)

func (b *BinanceClient) getBaseAsset(pairName string) string {
	// Convert "btc-usdt" to "BTC"
	parts := strings.Split(strings.ToUpper(pairName), "-")
	if len(parts) > 0 {
		return parts[0]
	}
	return ""
}

func (b *BinanceClient) normalizePairName(pairName string, isFutures bool) string {
	// Convert "btc-usdt" to "BTCUSDT"
	parts := strings.Split(strings.ToUpper(pairName), "-")
	symbol := strings.Join(parts, "")

	return symbol
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
