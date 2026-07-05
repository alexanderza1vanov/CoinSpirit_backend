package models

import "time"

type Transaction struct {
	ID              string    `json:"id"`
	PortfolioID     string    `json:"portfolio_id"`
	AssetID         string    `json:"asset_id"`
	Ticker          string    `json:"ticker,omitempty"`
	AssetName       string    `json:"asset_name,omitempty"`
	TransactionType string    `json:"transaction_type"`
	Quantity        float64   `json:"quantity"`
	UnitPrice       float64   `json:"unit_price"`
	FeeAmount       float64   `json:"fee_amount"`
	TotalAmount     float64   `json:"total_amount"`
	TransactionAt   time.Time `json:"transaction_at"`
	Note            string    `json:"note"`
	CreatedAt       time.Time `json:"created_at"`
}

type Position struct {
	ID               string   `json:"id"`
	PortfolioID      string   `json:"portfolio_id"`
	AssetID          string   `json:"asset_id"`
	Ticker           string   `json:"ticker"`
	AssetName        string   `json:"asset_name"`
	CurrencyCode     string   `json:"currency_code"`
	Quantity         float64  `json:"quantity"`
	AvgBuyPrice      float64  `json:"avg_buy_price"`
	CurrentPrice     float64  `json:"current_price"`
	InvestedValue    float64  `json:"invested_value"`
	CurrentValue     float64  `json:"current_value"`
	ProfitLoss       float64  `json:"profit_loss"`
	ProfitPercent    float64  `json:"profit_percent"`
	Change24hPercent *float64 `json:"change_24h_percent,omitempty"`
}

type PortfolioSummary struct {
	PortfolioID    string     `json:"portfolio_id"`
	BaseCurrency   string     `json:"base_currency"`
	InvestedValue  float64    `json:"invested_value"`
	CurrentValue   float64    `json:"current_value"`
	ProfitLoss     float64    `json:"profit_loss"`
	ProfitPercent  float64    `json:"profit_percent"`
	PositionsCount int        `json:"positions_count"`
	Positions      []Position `json:"positions"`
}
