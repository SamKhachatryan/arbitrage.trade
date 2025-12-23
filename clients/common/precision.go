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
	"xrp-usdt":  {QuantityPrecision: 1, PricePrecision: 4}, // ~$2: 5.0 XRP = ~$10
	"doge-usdt": {QuantityPrecision: 0, PricePrecision: 5}, // ~$0.17: 59 DOGE = ~$10
	"ada-usdt":  {QuantityPrecision: 0, PricePrecision: 4}, // ~$0.70: 14 ADA = ~$10
	"avax-usdt": {QuantityPrecision: 2, PricePrecision: 3}, // ~$25: 0.40 AVAX = ~$10
	"trx-usdt":  {QuantityPrecision: 0, PricePrecision: 5}, // ~$0.24: 42 TRX = ~$10
	"ton-usdt":  {QuantityPrecision: 2, PricePrecision: 3}, // ~$3.50: 2.86 TON = ~$10

	// Mid-cap pairs
	"link-usdt":  {QuantityPrecision: 2, PricePrecision: 3}, // ~$14: 0.71 LINK = ~$10
	"ltc-usdt":   {QuantityPrecision: 3, PricePrecision: 2}, // ~$100: 0.100 LTC = ~$10
	"bch-usdt":   {QuantityPrecision: 3, PricePrecision: 2}, // ~$350: 0.029 BCH = ~$10
	"uni-usdt":   {QuantityPrecision: 0, PricePrecision: 3}, // ~$8: 1 UNI = ~$8
	"apt-usdt":   {QuantityPrecision: 2, PricePrecision: 3}, // ~$6: 1.67 APT = ~$10
	"near-usdt":  {QuantityPrecision: 1, PricePrecision: 3}, // ~$3.50: 2.9 NEAR = ~$10
	"matic-usdt": {QuantityPrecision: 0, PricePrecision: 4}, // ~$0.35: 29 MATIC = ~$10
	"sui-usdt":   {QuantityPrecision: 1, PricePrecision: 4}, // ~$2.50: 4.0 SUI = ~$10
	"icp-usdt":   {QuantityPrecision: 2, PricePrecision: 3}, // ~$7: 1.43 ICP = ~$10
	"op-usdt":    {QuantityPrecision: 2, PricePrecision: 3}, // ~$1.20: 8.33 OP = ~$10
	"arb-usdt":   {QuantityPrecision: 0, PricePrecision: 4}, // ~$0.50: 20 ARB = ~$10

	// Small-cap pairs - more conservative precision
	"xvs-usdt":   {QuantityPrecision: 2, PricePrecision: 3},
	"ach-usdt":   {QuantityPrecision: 0, PricePrecision: 5},
	"fet-usdt":   {QuantityPrecision: 1, PricePrecision: 4},
	"rndr-usdt":  {QuantityPrecision: 2, PricePrecision: 3},
	"enj-usdt":   {QuantityPrecision: 1, PricePrecision: 4},
	"cfx-usdt":   {QuantityPrecision: 0, PricePrecision: 5},
	"kas-usdt":   {QuantityPrecision: 0, PricePrecision: 5},
	"mina-usdt":  {QuantityPrecision: 1, PricePrecision: 4},
	"gala-usdt":  {QuantityPrecision: 0, PricePrecision: 6},
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
