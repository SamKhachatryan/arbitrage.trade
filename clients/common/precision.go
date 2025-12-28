package common

import (
	"fmt"
	"math"
)

type PairPrecision struct {
	QuantityPrecision int
	PricePrecision    int
}

var PairPrecisions = map[string]PairPrecision{
	// Major pairs - conservative precision for cross-exchange compatibility
	// "btc-usdt":  {QuantityPrecision: 5, PricePrecision: 2}, // ~$87k: 0.00011 BTC = ~$10
	// "eth-usdt":  {QuantityPrecision: 4, PricePrecision: 2}, // ~$3k: 0.0033 ETH = ~$10
	// "bnb-usdt":  {QuantityPrecision: 2, PricePrecision: 2}, // ~$700: 0.014 BNB = ~$10
	// "sol-usdt":  {QuantityPrecision: 2, PricePrecision: 2}, // ~$140: 0.07 SOL = ~$10
	// Major Market Caps
	"xrp-usdt":  {QuantityPrecision: 1, PricePrecision: 4},
	"doge-usdt": {QuantityPrecision: 0, PricePrecision: 5}, // Consistent at 0 decimals for qty
	"ada-usdt":  {QuantityPrecision: 0, PricePrecision: 4}, // Changed Qty to 1 for better $10 fill
	"avax-usdt": {QuantityPrecision: 0, PricePrecision: 2}, // Safe at 1 across all 5 exchanges
	"trx-usdt":  {QuantityPrecision: 0, PricePrecision: 5}, // TRX is high-supply, keep at 0
	"ton-usdt":  {QuantityPrecision: 1, PricePrecision: 3}, // Increased PricePrecision to 3 for OKX/Gate

	// Mid-cap pairs
	"link-usdt":  {QuantityPrecision: 2, PricePrecision: 3}, // Stable across platforms
	"ltc-usdt":   {QuantityPrecision: 2, PricePrecision: 2}, // Reduced Qty to 2 (some Gate pairs cap here)
	"bch-usdt":   {QuantityPrecision: 2, PricePrecision: 2}, // Reduced Qty to 2 for safety
	"uni-usdt":   {QuantityPrecision: 1, PricePrecision: 3}, // Changed Qty to 1 to allow $10 scaling
	"apt-usdt":   {QuantityPrecision: 1, PricePrecision: 3}, // Reduced Qty to 1 for WhiteBIT/Gate compatibility
	"near-usdt":  {QuantityPrecision: 1, PricePrecision: 3},
	"matic-usdt": {QuantityPrecision: 1, PricePrecision: 4}, // Changed Qty to 1 (POL/MATIC migration shifts)
	"sui-usdt":   {QuantityPrecision: 1, PricePrecision: 4},
	"icp-usdt":   {QuantityPrecision: 1, PricePrecision: 3}, // Reduced Qty to 1 for safety
	"op-usdt":    {QuantityPrecision: 1, PricePrecision: 3}, // Reduced Qty to 1
	"arb-usdt":   {QuantityPrecision: 1, PricePrecision: 4}, // Changed Qty to 1 for better granularity

	// Small-cap pairs (More Conservative)
	"xvs-usdt":   {QuantityPrecision: 2, PricePrecision: 3},
	"ach-usdt":   {QuantityPrecision: 0, PricePrecision: 5},
	"fet-usdt":   {QuantityPrecision: 1, PricePrecision: 4},
	"rndr-usdt":  {QuantityPrecision: 1, PricePrecision: 3}, // Reduced Qty to 1
	"enj-usdt":   {QuantityPrecision: 1, PricePrecision: 4},
	"cfx-usdt":   {QuantityPrecision: 0, PricePrecision: 5},
	"kas-usdt":   {QuantityPrecision: 0, PricePrecision: 5},
	"mina-usdt":  {QuantityPrecision: 1, PricePrecision: 4},
	"gala-usdt":  {QuantityPrecision: 0, PricePrecision: 5}, // Reduced Price to 5 (Binance max varies)
	"blur-usdt":  {QuantityPrecision: 0, PricePrecision: 4},
	"wojak-usdt": {QuantityPrecision: 0, PricePrecision: 6},
	"mon-usdt":   {QuantityPrecision: 1, PricePrecision: 4},
}

func GetPrecision(pairName string) PairPrecision {
	if prec, ok := PairPrecisions[pairName]; ok {
		return prec
	}
	return PairPrecision{QuantityPrecision: 8, PricePrecision: 8}
}

func FormatQuantity(qty float64, pairName string) string {
	prec := GetPrecision(pairName)
	return fmt.Sprintf("%.*f", prec.QuantityPrecision, qty)
}

func FormatPrice(price float64, pairName string) string {
	prec := GetPrecision(pairName)
	return fmt.Sprintf("%.*f", prec.PricePrecision, price)
}

func RoundQuantity(qty float64, pairName string) float64 {
	prec := GetPrecision(pairName)
	multiplier := math.Pow(10, float64(prec.QuantityPrecision))
	return math.Floor(qty*multiplier) / multiplier
}
