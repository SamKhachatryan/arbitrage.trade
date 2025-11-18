package main

import (
	"context"
	"sync"
	"time"

	"arbitrage.trade/clients"
)

func ConsiderArbitrageOpportunity(ctx context.Context, shortExchange clients.ExchangeType, shortPrice float64, longExchange clients.ExchangeType,
	longPrice float64, pairName string, diffPercent float64, amountUSDT float64) {
	// Example output when an arbitrage opportunity is detected
	// println("Short on - binance (0.236300)")

	if diffPercent < 0.2 {
		return
	}

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		clients.Execute(ctx, clients.Binance, clients.PutSpotLong, pairName, amountUSDT)
		time.Sleep(5 * time.Second)
		clients.Execute(ctx, clients.Binance, clients.CloseSpotLong, pairName, amountUSDT)
		defer wg.Done()
	}()

	// go func() {
	// 	clients.Execute(ctx, clients.Binance, clients.PutFuturesShort, pairName, amountUSDT)
	// 	time.Sleep(10 * time.Second)
	// 	clients.Execute(ctx, clients.Binance, clients.CloseFuturesShort, pairName, amountUSDT)
	// 	defer wg.Done()
	// }()

	// go func() {
	// 	defer wg.Done()
	// 	clients.Execute(ctx, clients.Bitget, clients.PutSpotLong, pairName, amountUSDT)
	// 	time.Sleep(10 * time.Second)
	// 	clients.Execute(ctx, clients.Bitget, clients.CloseSpotLong, pairName, amountUSDT)
	// }()

	// go func() {
	// 	defer wg.Done()
	// 	clients.Execute(ctx, clients.Bitget, clients.PutFuturesShort, pairName, amountUSDT)
	// 	time.Sleep(10 * time.Second)
	// 	clients.Execute(ctx, clients.Bitget, clients.CloseFuturesShort, pairName, amountUSDT)
	// }()

	wg.Wait()
}
