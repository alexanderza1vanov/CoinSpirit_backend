package repository

import (
	"context"
	"strings"

	"github.com/example/invest-portfolio-platform/backend/internal/models"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AssetRepository struct{ db *pgxpool.Pool }

func NewAssetRepository(db *pgxpool.Pool) *AssetRepository { return &AssetRepository{db: db} }

type knownAsset struct {
	Ticker       string
	Name         string
	AssetType    string
	MarketType   string
	CurrencyCode string
	LotSize      float64
}

var knownAssets = []knownAsset{
	{Ticker: "BTC", Name: "Bitcoin", AssetType: "crypto", MarketType: "crypto", CurrencyCode: "USD", LotSize: 1},
	{Ticker: "ETH", Name: "Ethereum", AssetType: "crypto", MarketType: "crypto", CurrencyCode: "USD", LotSize: 1},
	{Ticker: "XRP", Name: "XRP", AssetType: "crypto", MarketType: "crypto", CurrencyCode: "USD", LotSize: 1},
	{Ticker: "SOL", Name: "Solana", AssetType: "crypto", MarketType: "crypto", CurrencyCode: "USD", LotSize: 1},
	{Ticker: "BNB", Name: "BNB", AssetType: "crypto", MarketType: "crypto", CurrencyCode: "USD", LotSize: 1},
	{Ticker: "ADA", Name: "Cardano", AssetType: "crypto", MarketType: "crypto", CurrencyCode: "USD", LotSize: 1},
	{Ticker: "DOGE", Name: "Dogecoin", AssetType: "crypto", MarketType: "crypto", CurrencyCode: "USD", LotSize: 1},
	{Ticker: "TON", Name: "Toncoin", AssetType: "crypto", MarketType: "crypto", CurrencyCode: "USD", LotSize: 1},
	{Ticker: "DOT", Name: "Polkadot", AssetType: "crypto", MarketType: "crypto", CurrencyCode: "USD", LotSize: 1},
	{Ticker: "AVAX", Name: "Avalanche", AssetType: "crypto", MarketType: "crypto", CurrencyCode: "USD", LotSize: 1},
	{Ticker: "LINK", Name: "Chainlink", AssetType: "crypto", MarketType: "crypto", CurrencyCode: "USD", LotSize: 1},
	{Ticker: "LTC", Name: "Litecoin", AssetType: "crypto", MarketType: "crypto", CurrencyCode: "USD", LotSize: 1},
	{Ticker: "SBER", Name: "Сбербанк", AssetType: "stock", MarketType: "moex", CurrencyCode: "RUB", LotSize: 1},
	{Ticker: "GAZP", Name: "Газпром", AssetType: "stock", MarketType: "moex", CurrencyCode: "RUB", LotSize: 1},
	{Ticker: "LKOH", Name: "Лукойл", AssetType: "stock", MarketType: "moex", CurrencyCode: "RUB", LotSize: 1},
	{Ticker: "YNDX", Name: "Яндекс", AssetType: "stock", MarketType: "moex", CurrencyCode: "RUB", LotSize: 1},
	{Ticker: "AFLT", Name: "Аэрофлот", AssetType: "stock", MarketType: "moex", CurrencyCode: "RUB", LotSize: 1},
	{Ticker: "GMKN", Name: "Норникель", AssetType: "stock", MarketType: "moex", CurrencyCode: "RUB", LotSize: 1},
	{Ticker: "ROSN", Name: "Роснефть", AssetType: "stock", MarketType: "moex", CurrencyCode: "RUB", LotSize: 1},
	{Ticker: "NVTK", Name: "Новатэк", AssetType: "stock", MarketType: "moex", CurrencyCode: "RUB", LotSize: 1},
	{Ticker: "TATN", Name: "Татнефть", AssetType: "stock", MarketType: "moex", CurrencyCode: "RUB", LotSize: 1},
	{Ticker: "MTSS", Name: "МТС", AssetType: "stock", MarketType: "moex", CurrencyCode: "RUB", LotSize: 1},
	{Ticker: "VTBR", Name: "ВТБ", AssetType: "stock", MarketType: "moex", CurrencyCode: "RUB", LotSize: 1},
	{Ticker: "MGNT", Name: "Магнит", AssetType: "stock", MarketType: "moex", CurrencyCode: "RUB", LotSize: 1},
}

func (r *AssetRepository) List(ctx context.Context, q string) ([]models.Asset, error) {
	q = strings.TrimSpace(q)
	if q != "" {
		if err := r.ensureKnownMatches(ctx, q); err != nil {
			return nil, err
		}
	}
	return r.listLocal(ctx, q)
}

func (r *AssetRepository) listLocal(ctx context.Context, q string) ([]models.Asset, error) {
	pattern := "%" + q + "%"
	rows, err := r.db.Query(ctx, `
		SELECT id, ticker, name, asset_type, market_type, currency_code, isin, figi, lot_size, is_active
		FROM assets
		WHERE is_active = true AND ($1 = '' OR ticker ILIKE $2 OR name ILIKE $2)
		ORDER BY
		  CASE WHEN UPPER(ticker)=UPPER($1) THEN 0 ELSE 1 END,
		  ticker
		LIMIT 50
	`, q, pattern)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]models.Asset, 0)
	for rows.Next() {
		var a models.Asset
		if err := rows.Scan(&a.ID, &a.Ticker, &a.Name, &a.AssetType, &a.MarketType, &a.CurrencyCode, &a.ISIN, &a.FIGI, &a.LotSize, &a.IsActive); err != nil {
			return nil, err
		}
		items = append(items, a)
	}
	return items, rows.Err()
}

func (r *AssetRepository) ensureKnownMatches(ctx context.Context, q string) error {
	query := strings.ToUpper(strings.TrimSpace(q))
	for _, a := range knownAssets {
		if !strings.Contains(strings.ToUpper(a.Ticker), query) && !strings.Contains(strings.ToUpper(a.Name), query) {
			continue
		}
		if _, err := r.ensureKnownAsset(ctx, a); err != nil {
			return err
		}
	}
	return nil
}

func (r *AssetRepository) ensureKnownAsset(ctx context.Context, item knownAsset) (*models.Asset, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, ticker, name, asset_type, market_type, currency_code, isin, figi, lot_size, is_active
		FROM assets WHERE UPPER(ticker)=UPPER($1) AND market_type=$2 AND is_active=true LIMIT 1
	`, item.Ticker, item.MarketType)
	var existing models.Asset
	if err := row.Scan(&existing.ID, &existing.Ticker, &existing.Name, &existing.AssetType, &existing.MarketType, &existing.CurrencyCode, &existing.ISIN, &existing.FIGI, &existing.LotSize, &existing.IsActive); err == nil {
		return &existing, nil
	} else if err != pgx.ErrNoRows {
		return nil, err
	}

	insert := r.db.QueryRow(ctx, `
		INSERT INTO assets (ticker, name, asset_type, market_type, currency_code, lot_size, is_active)
		VALUES ($1,$2,$3,$4,$5,$6,true)
		RETURNING id, ticker, name, asset_type, market_type, currency_code, isin, figi, lot_size, is_active
	`, item.Ticker, item.Name, item.AssetType, item.MarketType, item.CurrencyCode, item.LotSize)
	var a models.Asset
	if err := insert.Scan(&a.ID, &a.Ticker, &a.Name, &a.AssetType, &a.MarketType, &a.CurrencyCode, &a.ISIN, &a.FIGI, &a.LotSize, &a.IsActive); err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *AssetRepository) Get(ctx context.Context, id string) (*models.Asset, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, ticker, name, asset_type, market_type, currency_code, isin, figi, lot_size, is_active
		FROM assets WHERE id=$1 AND is_active=true
	`, id)
	var a models.Asset
	err := row.Scan(&a.ID, &a.Ticker, &a.Name, &a.AssetType, &a.MarketType, &a.CurrencyCode, &a.ISIN, &a.FIGI, &a.LotSize, &a.IsActive)
	return &a, err
}

func (r *AssetRepository) PriceHistory(ctx context.Context, assetID string, timeframe string) ([]models.PriceCandle, error) {
	if timeframe == "" {
		timeframe = "1d"
	}
	rows, err := r.db.Query(ctx, `
		SELECT id, asset_id, timeframe, open_price, high_price, low_price, close_price, volume, recorded_at
		FROM price_history WHERE asset_id=$1 AND timeframe=$2 ORDER BY recorded_at DESC LIMIT 200
	`, assetID, timeframe)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]models.PriceCandle, 0)
	for rows.Next() {
		var c models.PriceCandle
		if err := rows.Scan(&c.ID, &c.AssetID, &c.Timeframe, &c.OpenPrice, &c.HighPrice, &c.LowPrice, &c.ClosePrice, &c.Volume, &c.RecordedAt); err != nil {
			return nil, err
		}
		items = append(items, c)
	}
	return items, rows.Err()
}
