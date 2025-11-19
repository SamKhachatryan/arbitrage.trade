package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"arbitrage.trade/clients"
	"arbitrage.trade/clients/common"
)

func ConsiderArbitrageOpportunity(ctx context.Context, shortExchange common.ExchangeType, shortPrice float64, longExchange common.ExchangeType,
	longPrice float64, pairName string, diffPercent float64, amountUSDT float64) {
	// Example output when an arbitrage opportunity is detected
	// println("Short on - binance (0.236300)")

	if diffPercent < 0.1 {
		return
	}

	var wg sync.WaitGroup
	wg.Add(2)

	spotProfit := 0.00
	futuresProfit := 0.00

	go func() {
		clients.Execute(ctx, shortExchange, common.PutFuturesShort, pairName, amountUSDT)
		time.Sleep(10 * time.Second)
		futuresProfit, _ = clients.Execute(ctx, shortExchange, common.CloseFuturesShort, pairName, amountUSDT)
		defer wg.Done()
	}()

	go func() {
		clients.Execute(ctx, longExchange, common.PutSpotLong, pairName, amountUSDT)
		time.Sleep(10 * time.Second)
		spotProfit, _ = clients.Execute(ctx, longExchange, common.CloseSpotLong, pairName, amountUSDT)
		defer wg.Done()
	}()

	wg.Wait()
	fmt.Printf("Result (%s): %f Spot Profit - %f, Futures Profit - %f\n", pairName, spotProfit+futuresProfit, spotProfit, futuresProfit)
}
