package models

import "time"

type Asset struct {
	ID               string   `json:"id"`
	Ticker           string   `json:"ticker"`
	Name             string   `json:"name"`
	AssetType        string   `json:"asset_type"`
	MarketType       string   `json:"market_type"`
	CurrencyCode     string   `json:"currency_code"`
	ISIN             *string  `json:"isin,omitempty"`
	FIGI             *string  `json:"figi,omitempty"`
	LotSize          float64  `json:"lot_size"`
	IsActive         bool     `json:"is_active"`
	CurrentPrice     *float64 `json:"current_price,omitempty"`
	Change24hPercent *float64 `json:"change_24h_percent,omitempty"`
}

type PriceCandle struct {
	ID         string    `json:"id"`
	AssetID    string    `json:"asset_id"`
	Timeframe  string    `json:"timeframe"`
	OpenPrice  float64   `json:"open_price"`
	HighPrice  float64   `json:"high_price"`
	LowPrice   float64   `json:"low_price"`
	ClosePrice float64   `json:"close_price"`
	Volume     float64   `json:"volume"`
	RecordedAt time.Time `json:"recorded_at"`
}
