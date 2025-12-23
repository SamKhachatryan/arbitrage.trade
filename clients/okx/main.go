package okx

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"arbitrage.trade/clients/common"
)

func NewOkxClient(apiKey, apiSecret, passphrase string) *OkxClient {
	client := &OkxClient{
		apiKey:     apiKey,
		apiSecret:  apiSecret,
		passphrase: passphrase,
		baseURL:    "https://www.okx.com",
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		positions: make(map[string]*common.Position),
	}

	// Initialize account settings
	ctx := context.Background()
	if err := client.initializeAccount(ctx); err != nil {
		log.Printf("⚠️  [OKX] Failed to initialize account settings: %v", err)
		log.Printf("💡 [OKX] Please manually configure: Account Mode = Single-currency margin, Position Mode = Net mode")
	}

	return client
}

// initializeAccount sets up the OKX account with proper trading settings
func (o *OkxClient) initializeAccount(ctx context.Context) error {
	// Set position mode to net_mode (long/short mode) instead of hedge mode
	positionModeReq := map[string]interface{}{
		"posMode": "net_mode", // net_mode for combined positions, long_short_mode for hedge
	}
	posBody, _ := json.Marshal(positionModeReq)

	var posResult struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
	}

	// Try to set position mode - may fail if already set, which is fine
	if err := o.signedRequest(ctx, "POST", "/api/v5/account/set-position-mode", string(posBody), &posResult); err == nil {
		if posResult.Code == "0" {
			log.Println("✅ [OKX] Position mode set to net_mode")
		} else if posResult.Code == "59104" {
			// 59104 = already in this mode
			log.Println("✅ [OKX] Position mode already set to net_mode")
		} else if posResult.Code == "59000" {
			// 59000 = Setting failed (usually means already configured or has active positions)
			log.Println("✅ [OKX] Position mode likely already configured (code 59000)")
		} else {
			log.Printf("⚠️  [OKX] Position mode response: %s - %s", posResult.Code, posResult.Msg)
		}
	}

	return nil
}

func (o *OkxClient) GetName() string {
	return "okx"
}
