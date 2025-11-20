package clients

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"arbitrage.trade/clients/binance"
	"arbitrage.trade/clients/bitget"
	"arbitrage.trade/clients/common"
	"arbitrage.trade/clients/whitebit"
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
	}

	return profit, err
}
