package main

import (
	"context"
	"fmt"
	"log"
	"time"
)

// ExampleUsage demonstrates how to use the ExchangeTradeClient
func ExampleUsage() {
	// Initialize Binance client
	binanceClient := NewBinanceClient("your-api-key", "your-api-secret")

	// You can also create clients for other exchanges implementing the same interface
	// Example: var krakenClient ExchangeTradeClient = NewKrakenClient(...)

	ctx := context.Background()
	pairName := "btc-usdt"
	amountUSDT := 100.0 // Trade with $100

	// Execute arbitrage strategy
	err := executeArbitrage(ctx, binanceClient, pairName, amountUSDT)
	if err != nil {
		log.Printf("Arbitrage execution failed: %v", err)
	}
}

// executeArbitrage executes a complete arbitrage cycle
func executeArbitrage(ctx context.Context, client ExchangeTradeClient, pairName string, amountUSDT float64) error {
	log.Printf("[%s] Starting arbitrage for %s with $%.2f", client.GetName(), pairName, amountUSDT)

	// Step 1: Open spot long position (buy asset with USDT)
	spotResult, err := client.PutSpotLong(ctx, pairName, amountUSDT)
	if err != nil {
		return fmt.Errorf("failed to open spot long: %w", err)
	}
	log.Printf("[SPOT LONG] %s", spotResult.Message)
	log.Printf("  Order ID: %s, Price: %.8f, Qty: %.8f, Fee: %.8f",
		spotResult.OrderID, spotResult.ExecutedPrice, spotResult.ExecutedQty, spotResult.Fee)

	// Step 2: Open futures short position
	futuresResult, err := client.PutFuturesShort(ctx, pairName, amountUSDT)
	if err != nil {
		// If futures short fails, close the spot position
		log.Printf("Failed to open futures short: %v. Closing spot position...", err)
		if closeResult, closeErr := client.CloseSpotLong(ctx, pairName); closeErr != nil {
			log.Printf("Failed to close spot: %v", closeErr)
		} else {
			log.Printf("[SPOT CLOSE] %s", closeResult.Message)
		}
		return fmt.Errorf("failed to open futures short: %w", err)
	}
	log.Printf("[FUTURES SHORT] %s", futuresResult.Message)
	log.Printf("  Order ID: %s, Price: %.2f, Qty: %.3f",
		futuresResult.OrderID, futuresResult.ExecutedPrice, futuresResult.ExecutedQty)

	// Wait for price convergence or based on your strategy
	log.Printf("Positions opened. Waiting for convergence...")
	time.Sleep(30 * time.Second) // Example wait time

	// Step 3: Close both positions
	// Close spot long first (sell asset back to USDT)
	spotCloseResult, err := client.CloseSpotLong(ctx, pairName)
	if err != nil {
		log.Printf("Failed to close spot long: %v", err)
	} else {
		log.Printf("[SPOT CLOSE] %s", spotCloseResult.Message)
		log.Printf("  Order ID: %s, Price: %.8f, Qty: %.8f",
			spotCloseResult.OrderID, spotCloseResult.ExecutedPrice, spotCloseResult.ExecutedQty)
	}

	// Close futures short
	futuresCloseResult, err := client.CloseFuturesShort(ctx, pairName)
	if err != nil {
		log.Printf("Failed to close futures short: %v", err)
	} else {
		log.Printf("[FUTURES CLOSE] %s", futuresCloseResult.Message)
		log.Printf("  Order ID: %s, Price: %.2f, Qty: %.3f",
			futuresCloseResult.OrderID, futuresCloseResult.ExecutedPrice, futuresCloseResult.ExecutedQty)
	}

	log.Printf("[%s] Arbitrage cycle completed for %s", client.GetName(), pairName)
	return nil
}

// MultiExchangeArbitrage demonstrates using multiple exchanges
func MultiExchangeArbitrage() {
	// Create clients for different exchanges
	binanceClient := NewBinanceClient("binance-key", "binance-secret")
	// var bybitClient ExchangeTradeClient = NewBybitClient("bybit-key", "bybit-secret")
	// var okxClient ExchangeTradeClient = NewOKXClient("okx-key", "okx-secret")

	clients := []ExchangeTradeClient{
		binanceClient,
		// bybitClient,
		// okxClient,
	}

	ctx := context.Background()

	// Execute arbitrage on the exchange with best opportunity
	for _, client := range clients {
		log.Printf("Attempting arbitrage on %s", client.GetName())
		err := executeArbitrage(ctx, client, "eth-usdt", 200.0)
		if err != nil {
			log.Printf("Failed on %s: %v", client.GetName(), err)
			continue
		}
		break // Success, no need to try other exchanges
	}
}
