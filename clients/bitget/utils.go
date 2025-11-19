package bitget

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
	"time"
)

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

func (b *BitgetClient) normalizeSymbol(pairName string) string {
	// Convert "btc-usdt" to "BTCUSDT"
	return strings.ToUpper(strings.ReplaceAll(pairName, "-", ""))
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
