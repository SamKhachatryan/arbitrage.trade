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
	"btc-usdt":   {QuantityPrecision: 6, PricePrecision: 2},
	"eth-usdt":   {QuantityPrecision: 5, PricePrecision: 2},
	"sol-usdt":   {QuantityPrecision: 3, PricePrecision: 3},
	"doge-usdt":  {QuantityPrecision: 1, PricePrecision: 6},
	"xrp-usdt":   {QuantityPrecision: 2, PricePrecision: 4},
	"ton-usdt":   {QuantityPrecision: 3, PricePrecision: 4},
	"ada-usdt":   {QuantityPrecision: 1, PricePrecision: 5},
	"link-usdt":  {QuantityPrecision: 3, PricePrecision: 3},
	"arb-usdt":   {QuantityPrecision: 0, PricePrecision: 4},
	"op-usdt":    {QuantityPrecision: 3, PricePrecision: 4},
	"ltc-usdt":   {QuantityPrecision: 4, PricePrecision: 2},
	"bch-usdt":   {QuantityPrecision: 4, PricePrecision: 2},
	"uni-usdt":   {QuantityPrecision: 0, PricePrecision: 3},
	"avax-usdt":  {QuantityPrecision: 3, PricePrecision: 3},
	"apt-usdt":   {QuantityPrecision: 3, PricePrecision: 3},
	"near-usdt":  {QuantityPrecision: 2, PricePrecision: 4},
	"matic-usdt": {QuantityPrecision: 1, PricePrecision: 5},
	"sui-usdt":   {QuantityPrecision: 2, PricePrecision: 4},
	"icp-usdt":   {QuantityPrecision: 3, PricePrecision: 3},
	"xvs-usdt":   {QuantityPrecision: 3, PricePrecision: 3},
	"ach-usdt":   {QuantityPrecision: 0, PricePrecision: 5},
	"fet-usdt":   {QuantityPrecision: 2, PricePrecision: 4},
	"rndr-usdt":  {QuantityPrecision: 3, PricePrecision: 4},
	"enj-usdt":   {QuantityPrecision: 2, PricePrecision: 5},
	"cfx-usdt":   {QuantityPrecision: 1, PricePrecision: 5},
	"kas-usdt":   {QuantityPrecision: 1, PricePrecision: 6},
	"mina-usdt":  {QuantityPrecision: 2, PricePrecision: 5},
	"gala-usdt":  {QuantityPrecision: 1, PricePrecision: 6},
	"blur-usdt":  {QuantityPrecision: 0, PricePrecision: 5},
	"wojak-usdt": {QuantityPrecision: 1, PricePrecision: 7},
	"bnb-usdt":   {QuantityPrecision: 4, PricePrecision: 2},
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
