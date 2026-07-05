package repository

import (
	"context"
	"time"

	"github.com/example/invest-portfolio-platform/backend/internal/models"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PriceRepository struct{ db *pgxpool.Pool }

func NewPriceRepository(db *pgxpool.Pool) *PriceRepository { return &PriceRepository{db: db} }

func (r *PriceRepository) ListCandles(ctx context.Context, assetID string, timeframe string, limit int) ([]models.PriceCandle, error) {
	if timeframe == "" {
		timeframe = "1d"
	}
	if limit <= 0 {
		limit = 200
	}
	rows, err := r.db.Query(ctx, `
		SELECT id, asset_id, timeframe, open_price, high_price, low_price, close_price, volume, recorded_at
		FROM price_history
		WHERE asset_id=$1 AND timeframe=$2
		ORDER BY recorded_at ASC
		LIMIT $3
	`, assetID, timeframe, limit)
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

func (r *PriceRepository) ListCandlesSince(ctx context.Context, assetID string, start time.Time, limit int) ([]models.PriceCandle, error) {
	if limit <= 0 {
		limit = 500
	}
	rows, err := r.db.Query(ctx, `
		SELECT id, asset_id, timeframe, open_price, high_price, low_price, close_price, volume, recorded_at
		FROM price_history
		WHERE asset_id=$1 AND recorded_at >= $2
		ORDER BY recorded_at ASC
		LIMIT $3
	`, assetID, start, limit)
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

func (r *PriceRepository) LatestCandle(ctx context.Context, assetID string, timeframe string) (*models.PriceCandle, error) {
	if timeframe == "" {
		timeframe = "1d"
	}
	row := r.db.QueryRow(ctx, `
		SELECT id, asset_id, timeframe, open_price, high_price, low_price, close_price, volume, recorded_at
		FROM price_history
		WHERE asset_id=$1 AND timeframe=$2
		ORDER BY recorded_at DESC
		LIMIT 1
	`, assetID, timeframe)
	var c models.PriceCandle
	if err := row.Scan(&c.ID, &c.AssetID, &c.Timeframe, &c.OpenPrice, &c.HighPrice, &c.LowPrice, &c.ClosePrice, &c.Volume, &c.RecordedAt); err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *PriceRepository) SaveCandle(ctx context.Context, assetID string, timeframe string, open float64, high float64, low float64, close float64, volume float64, recordedAt time.Time) error {
	if timeframe == "" {
		timeframe = "1d"
	}
	_, err := r.db.Exec(ctx, `
		INSERT INTO price_history (asset_id, timeframe, open_price, high_price, low_price, close_price, volume, recorded_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
	`, assetID, timeframe, open, high, low, close, volume, recordedAt)
	return err
}
