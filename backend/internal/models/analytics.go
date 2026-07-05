package models

import "time"

type AssetPrice struct {
	AssetID          string    `json:"asset_id"`
	Ticker           string    `json:"ticker"`
	Price            float64   `json:"price"`
	Currency         string    `json:"currency"`
	Source           string    `json:"source"`
	UpdatedAt        time.Time `json:"updated_at"`
	Change24hPercent *float64  `json:"change_24h_percent,omitempty"`
}

type IndicatorRSI struct {
	Period int     `json:"period"`
	Value  float64 `json:"value"`
	Signal string  `json:"signal"`
}

type IndicatorMACD struct {
	MACD      float64 `json:"macd"`
	Signal    float64 `json:"signal"`
	Histogram float64 `json:"histogram"`
	Trend     string  `json:"trend"`
}

type IndicatorBollingerBands struct {
	Period int     `json:"period"`
	Upper  float64 `json:"upper"`
	Middle float64 `json:"middle"`
	Lower  float64 `json:"lower"`
}

type IndicatorMovingAverage struct {
	SMA20 float64 `json:"sma_20"`
	EMA50 float64 `json:"ema_50"`
	Trend string  `json:"trend"`
}

type AssetIndicators struct {
	AssetID        string                  `json:"asset_id"`
	Ticker         string                  `json:"ticker"`
	Timeframe      string                  `json:"timeframe"`
	RSI            IndicatorRSI            `json:"rsi"`
	MACD           IndicatorMACD           `json:"macd"`
	BollingerBands IndicatorBollingerBands `json:"bollinger_bands"`
	MovingAverage  IndicatorMovingAverage  `json:"moving_average"`
	CalculatedAt   time.Time               `json:"calculated_at"`
}

type AssetAnalyticsOverview struct {
	Asset      Asset           `json:"asset"`
	Price      AssetPrice      `json:"price"`
	Timeframe  string          `json:"timeframe"`
	Candles    []PriceCandle   `json:"candles"`
	Indicators AssetIndicators `json:"indicators"`
	Note       string          `json:"note,omitempty"`
}
