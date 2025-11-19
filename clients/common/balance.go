package common

import "sync"

type AssetBalances map[string]float64
type MarketBalances map[string]AssetBalances
type ExchangeBalances map[string]MarketBalances

var (
	Balances           = ExchangeBalances{}
	balanceMutexes     = make(map[string]*sync.RWMutex)
	balanceMutexesLock sync.Mutex
)

func getBalanceMutex(exchange, market string) *sync.RWMutex {
	key := exchange + ":" + market
	balanceMutexesLock.Lock()
	defer balanceMutexesLock.Unlock()

	if _, ok := balanceMutexes[key]; !ok {
		balanceMutexes[key] = &sync.RWMutex{}
	}
	return balanceMutexes[key]
}

func SetBalance(exchange, market, asset string, value float64) {
	mu := getBalanceMutex(exchange, market)
	mu.Lock()
	defer mu.Unlock()

	if _, ok := Balances[exchange]; !ok {
		Balances[exchange] = MarketBalances{}
	}
	if _, ok := Balances[exchange][market]; !ok {
		Balances[exchange][market] = AssetBalances{}
	}
	Balances[exchange][market][asset] = value
}

func GetBalance(exchange, market, asset string) float64 {
	mu := getBalanceMutex(exchange, market)
	mu.RLock()
	defer mu.RUnlock()

	if ex, ok := Balances[exchange]; ok {
		if mk, ok := ex[market]; ok {
			if val, ok := mk[asset]; ok {
				return val
			}
		}
	}
	return 0.00
}
