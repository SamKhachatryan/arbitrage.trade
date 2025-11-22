package okx

import (
	"net/http"
	"sync"

	"arbitrage.trade/clients/common"
)

type OkxClient struct {
	apiKey     string
	apiSecret  string
	passphrase string
	baseURL    string
	httpClient *http.Client

	positions map[string]*common.Position
	mu        sync.RWMutex
}

type OkxResponse struct {
	Code string        `json:"code"`
	Msg  string        `json:"msg"`
	Data []interface{} `json:"data"`
}

type Balance struct {
	Ccy       string `json:"ccy"`
	Bal       string `json:"bal"`
	AvailBal  string `json:"availBal"`
	FrozenBal string `json:"frozenBal"`
}

type OrderResponse struct {
	OrdId     string `json:"ordId"`
	ClOrdId   string `json:"clOrdId"`
	Tag       string `json:"tag"`
	SCode     string `json:"sCode"`
	SMsg      string `json:"sMsg"`
	AvgPx     string `json:"avgPx"`
	AccFillSz string `json:"accFillSz"`
	Fee       string `json:"fee"`
	State     string `json:"state"`
}

type PositionData struct {
	InstId   string `json:"instId"`
	Pos      string `json:"pos"`
	AvgPx    string `json:"avgPx"`
	MarkPx   string `json:"markPx"`
	Upl      string `json:"upl"`
	UplRatio string `json:"uplRatio"`
	PosSide  string `json:"posSide"`
	Lever    string `json:"lever"`
}
