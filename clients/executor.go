package clients

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"arbitrage.trade/clients/binance"
	"arbitrage.trade/clients/bitget"
	"arbitrage.trade/clients/common"
	"arbitrage.trade/clients/gate"
	"arbitrage.trade/clients/okx"
	"arbitrage.trade/clients/whitebit"
	"arbitrage.trade/redis"
)

var (
	// Singleton clients - reuse the same instance to maintain position state
	clientInstances = make(map[common.ExchangeType]common.ExchangeTradeClient)
	clientMutex     sync.RWMutex
)

var exchangeRegistry = map[common.ExchangeType]func(string, string) common.ExchangeTradeClient{
	common.Binance: func(key, secret string) common.ExchangeTradeClient {
		return binance.NewBinanceClient(key, secret)
	},
	common.Bitget: func(key, secret string) common.ExchangeTradeClient {
		passphrase := os.Getenv("BITGET_PASSPHRASE")
		return bitget.NewBitgetClient(key, secret, passphrase)
	},
	common.Whitebit: func(key, secret string) common.ExchangeTradeClient {
		return whitebit.NewWhitebitClient(key, secret)
	},
	common.Gate: func(key, secret string) common.ExchangeTradeClient {
		return gate.NewGateClient(key, secret)
	},
	common.Okx: func(key, secret string) common.ExchangeTradeClient {
		passphrase := os.Getenv("OKX_PASSPHRASE")
		return okx.NewOkxClient(key, secret, passphrase)
	},
}

// getOrCreateClient returns a singleton client instance for the given exchange
func getOrCreateClient(exchange common.ExchangeType) (common.ExchangeTradeClient, error) {
	clientMutex.RLock()
	if client, exists := clientInstances[exchange]; exists {
		clientMutex.RUnlock()
		return client, nil
	}
	clientMutex.RUnlock()

	// Need to create new client
	clientMutex.Lock()
	defer clientMutex.Unlock()

	// Double-check after acquiring write lock
	if client, exists := clientInstances[exchange]; exists {
		return client, nil
	}

	constructor, ok := exchangeRegistry[exchange]
	if !ok {
		return nil, fmt.Errorf("unknown exchange: %s", exchange)
	}

	keyEnv := fmt.Sprintf("%s_API_KEY", strings.ToUpper(string(exchange)))
	secretEnv := fmt.Sprintf("%s_API_SECRET", strings.ToUpper(string(exchange)))

	apiKey := os.Getenv(keyEnv)
	apiSecret := os.Getenv(secretEnv)

	if apiKey == "" || apiSecret == "" {
		return nil, fmt.Errorf("missing API credentials for %s", exchange)
	}

	client := constructor(apiKey, apiSecret)
	clientInstances[exchange] = client
	return client, nil
}

func Execute(ctx context.Context, exchange common.ExchangeType, command common.OrderType, pairName string, amountUSDT float64) (float64, error) {
	fmt.Printf("[%s] |%s| - Starting\n", exchange, command)

	client, err := getOrCreateClient(exchange)
	profit := 0.00

	if err != nil {
		return 0.00, err
	}

	// Determine trade details for Redis publishing
	var side, action string
	switch command {
	case common.PutSpotLong:
		side = "spot_long"
		action = "open"
	case common.CloseSpotLong:
		side = "spot_long"
		action = "close"
	case common.PutFuturesShort:
		side = "futures_short"
		action = "open"
	case common.CloseFuturesShort:
		side = "futures_short"
		action = "close"
	}

	switch command {
	case common.PutSpotLong:
		_, err = client.PutSpotLong(ctx, pairName, amountUSDT)
	case common.CloseSpotLong:
		_, profit, err = client.CloseSpotLong(ctx, pairName, amountUSDT)
	case common.PutFuturesShort:
		_, err = client.PutFuturesShort(ctx, pairName, amountUSDT)
	case common.CloseFuturesShort:
		_, profit, err = client.CloseFuturesShort(ctx, pairName)
	default:
		return 0.00, fmt.Errorf("unknown command: %s", command)
	}

	if err != nil {
		fmt.Printf("[%s] |%s| - Failed: %s\n", exchange, command, err)
	} else {
		fmt.Printf("[%s] |%s| - Succeeded\n", exchange, command)

		// Publish successful trade execution to Redis
		redis.PublishTradeExecution(redis.TradeExecution{
			Exchange:  string(exchange),
			Pair:      pairName,
			Side:      side,
			Action:    action,
			Amount:    amountUSDT,
			Price:     0, // Price will be added from position context if needed
			SpreadPct: 0, // Spread will be added from position context if needed
			Timestamp: time.Now(),
		})
	}

	return profit, err
}
