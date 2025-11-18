package clients

import (
	"context"
	"fmt"
	"os"
	"strings"
)

var exchangeRegistry = map[ExchangeType]func(string, string) ExchangeTradeClient{
	Binance: func(key, secret string) ExchangeTradeClient {
		return NewBinanceClient(key, secret)
	},
	Bitget: func(key, secret string) ExchangeTradeClient {
		passphrase := os.Getenv("BITGET_PASSPHRASE")
		return NewBitgetClient(key, secret, passphrase)
	},
}

func Execute(ctx context.Context, exchange ExchangeType, command OrderType, pairName string, amountUSDT float64) error {
	fmt.Printf("[%s] |%s| - Starting\n", exchange, command)

	constructor, ok := exchangeRegistry[exchange]
	if !ok {
		return fmt.Errorf("unknown exchange: %s", exchange)
	}

	keyEnv := fmt.Sprintf("%s_API_KEY", strings.ToUpper(string(exchange)))
	secretEnv := fmt.Sprintf("%s_API_SECRET", strings.ToUpper(string(exchange)))

	apiKey := os.Getenv(keyEnv)
	apiSecret := os.Getenv(secretEnv)

	if apiKey == "" || apiSecret == "" {
		return fmt.Errorf("missing API credentials for %s", exchange)
	}

	client := constructor(apiKey, apiSecret)

	var err error

	switch command {
	case PutSpotLong:
		_, err = client.PutSpotLong(ctx, pairName, amountUSDT)
	case CloseSpotLong:
		_, err = client.CloseSpotLong(ctx, pairName, amountUSDT)
	case PutFuturesShort:
		_, err = client.PutFuturesShort(ctx, pairName, amountUSDT)
	case CloseFuturesShort:
		_, err = client.CloseFuturesShort(ctx, pairName)
	default:
		return fmt.Errorf("unknown command: %s", command)
	}

	if err != nil {
		fmt.Printf("[%s] |%s| - Failed: %s\n", exchange, command, err)
	} else {
		fmt.Printf("[%s] |%s| - Succeeded\n", exchange, command)
	}

	return err
}

// func ExecuteArbitrageCycle(ctx context.Context, exchange string, pairName string, amountUSDT float64) error {
// 	log.Printf("[%s] Starting arbitrage cycle for %s", exchange, pairName)

// 	if err := Execute(ctx, exchange, "open-spot", pairName, amountUSDT); err != nil {
// 		return fmt.Errorf("open spot failed: %w", err)
// 	}

// 	if err := Execute(ctx, exchange, "open-futures", pairName, amountUSDT); err != nil {
// 		log.Printf("[%s] Futures failed, closing spot", exchange)
// 		Execute(ctx, exchange, "close-spot", pairName, amountUSDT)
// 		return fmt.Errorf("open futures failed: %w", err)
// 	}

// 	if err := Execute(ctx, exchange, "close-spot", pairName, amountUSDT); err != nil {
// 		return fmt.Errorf("close spot failed: %w", err)
// 	}

// 	if err := Execute(ctx, exchange, "close-futures", pairName, amountUSDT); err != nil {
// 		return fmt.Errorf("close futures failed: %w", err)
// 	}

// 	return nil
// }
