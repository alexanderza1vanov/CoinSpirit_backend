package repository

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/example/invest-portfolio-platform/backend/internal/models"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PortfolioRepository struct{ db *pgxpool.Pool }

func NewPortfolioRepository(db *pgxpool.Pool) *PortfolioRepository {
	return &PortfolioRepository{db: db}
}

func (r *PortfolioRepository) Create(ctx context.Context, userID string, name string, baseCurrency string, description string) (*models.Portfolio, error) {
	if baseCurrency == "" {
		baseCurrency = "USD"
	}
	row := r.db.QueryRow(ctx, `
		INSERT INTO portfolios (user_id, name, base_currency, description)
		VALUES ($1, $2, $3, $4)
		RETURNING id, user_id, name, base_currency, COALESCE(description, ''), created_at, updated_at
	`, userID, name, baseCurrency, description)
	var p models.Portfolio
	err := row.Scan(&p.ID, &p.UserID, &p.Name, &p.BaseCurrency, &p.Description, &p.CreatedAt, &p.UpdatedAt)
	return &p, err
}

func (r *PortfolioRepository) ListByUser(ctx context.Context, userID string) ([]models.Portfolio, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, user_id, name, base_currency, COALESCE(description, ''), created_at, updated_at
		FROM portfolios WHERE user_id = $1 ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]models.Portfolio, 0)
	for rows.Next() {
		var p models.Portfolio
		if err := rows.Scan(&p.ID, &p.UserID, &p.Name, &p.BaseCurrency, &p.Description, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, p)
	}
	return items, rows.Err()
}

func (r *PortfolioRepository) GetByIDForUser(ctx context.Context, id string, userID string) (*models.Portfolio, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, user_id, name, base_currency, COALESCE(description, ''), created_at, updated_at
		FROM portfolios WHERE id = $1 AND user_id = $2
	`, id, userID)
	var p models.Portfolio
	err := row.Scan(&p.ID, &p.UserID, &p.Name, &p.BaseCurrency, &p.Description, &p.CreatedAt, &p.UpdatedAt)
	return &p, err
}

func (r *PortfolioRepository) AddTransaction(ctx context.Context, userID string, portfolioID string, assetID string, txType string, quantity float64, unitPrice float64, feeAmount float64, transactionAt time.Time, note string) (*models.Transaction, error) {
	if txType != "buy" && txType != "sell" {
		return nil, errors.New("transaction_type must be buy or sell")
	}
	if quantity <= 0 || unitPrice < 0 || feeAmount < 0 {
		return nil, errors.New("invalid amounts")
	}
	pgTx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer func() { _ = pgTx.Rollback(ctx) }()
	var portfolioOwner string
	if err := pgTx.QueryRow(ctx, `SELECT user_id FROM portfolios WHERE id=$1`, portfolioID).Scan(&portfolioOwner); err != nil {
		return nil, err
	}
	if portfolioOwner != userID {
		return nil, errors.New("portfolio not found")
	}
	var currentQty, currentAvg float64
	err = pgTx.QueryRow(ctx, `SELECT quantity, avg_buy_price FROM portfolio_positions WHERE portfolio_id=$1 AND asset_id=$2 FOR UPDATE`, portfolioID, assetID).Scan(&currentQty, &currentAvg)
	if errors.Is(err, pgx.ErrNoRows) {
		currentQty, currentAvg = 0, 0
	} else if err != nil {
		return nil, err
	}

	newQty := currentQty
	newAvg := currentAvg
	if txType == "buy" {
		newQty = currentQty + quantity
		if newQty > 0 {
			newAvg = ((currentQty * currentAvg) + (quantity * unitPrice) + feeAmount) / newQty
		}
	} else {
		if quantity > currentQty {
			return nil, errors.New("sell quantity exceeds current position")
		}
		newQty = currentQty - quantity
		if math.Abs(newQty) < 0.00000001 {
			newQty, newAvg = 0, 0
		}
	}
	totalAmount := quantity*unitPrice + feeAmount
	row := pgTx.QueryRow(ctx, `
		INSERT INTO transactions (portfolio_id, asset_id, transaction_type, quantity, unit_price, fee_amount, total_amount, transaction_at, note)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		RETURNING id, portfolio_id, asset_id, transaction_type, quantity, unit_price, fee_amount, total_amount, transaction_at, COALESCE(note,''), created_at
	`, portfolioID, assetID, txType, quantity, unitPrice, feeAmount, totalAmount, transactionAt, note)
	var out models.Transaction
	if err := row.Scan(&out.ID, &out.PortfolioID, &out.AssetID, &out.TransactionType, &out.Quantity, &out.UnitPrice, &out.FeeAmount, &out.TotalAmount, &out.TransactionAt, &out.Note, &out.CreatedAt); err != nil {
		return nil, err
	}
	_, err = pgTx.Exec(ctx, `
		INSERT INTO portfolio_positions (portfolio_id, asset_id, quantity, avg_buy_price)
		VALUES ($1,$2,$3,$4)
		ON CONFLICT (portfolio_id, asset_id) DO UPDATE SET quantity=EXCLUDED.quantity, avg_buy_price=EXCLUDED.avg_buy_price, updated_at=NOW()
	`, portfolioID, assetID, newQty, newAvg)
	if err != nil {
		return nil, err
	}
	if err := pgTx.Commit(ctx); err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *PortfolioRepository) UpdateTransaction(ctx context.Context, userID string, portfolioID string, transactionID string, txType string, quantity float64, unitPrice float64, feeAmount float64, transactionAt time.Time, note string) (*models.Transaction, error) {
	if txType != "buy" && txType != "sell" {
		return nil, errors.New("transaction_type must be buy or sell")
	}
	if quantity <= 0 || unitPrice < 0 || feeAmount < 0 {
		return nil, errors.New("invalid amounts")
	}

	pgTx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer func() { _ = pgTx.Rollback(ctx) }()

	var portfolioOwner string
	if err := pgTx.QueryRow(ctx, `SELECT user_id FROM portfolios WHERE id=$1`, portfolioID).Scan(&portfolioOwner); err != nil {
		return nil, err
	}
	if portfolioOwner != userID {
		return nil, errors.New("portfolio not found")
	}

	var assetID string
	if err := pgTx.QueryRow(ctx, `SELECT asset_id FROM transactions WHERE id=$1 AND portfolio_id=$2 FOR UPDATE`, transactionID, portfolioID).Scan(&assetID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.New("transaction not found")
		}
		return nil, err
	}

	totalAmount := quantity*unitPrice + feeAmount
	row := pgTx.QueryRow(ctx, `
		UPDATE transactions
		SET transaction_type=$1, quantity=$2, unit_price=$3, fee_amount=$4, total_amount=$5, transaction_at=$6, note=$7
		WHERE id=$8 AND portfolio_id=$9
		RETURNING id, portfolio_id, asset_id, transaction_type, quantity, unit_price, fee_amount, total_amount, transaction_at, COALESCE(note,''), created_at
	`, txType, quantity, unitPrice, feeAmount, totalAmount, transactionAt, note, transactionID, portfolioID)

	var out models.Transaction
	if err := row.Scan(&out.ID, &out.PortfolioID, &out.AssetID, &out.TransactionType, &out.Quantity, &out.UnitPrice, &out.FeeAmount, &out.TotalAmount, &out.TransactionAt, &out.Note, &out.CreatedAt); err != nil {
		return nil, err
	}

	if err := r.recalculatePosition(ctx, pgTx, portfolioID, assetID); err != nil {
		return nil, err
	}

	if err := pgTx.Commit(ctx); err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *PortfolioRepository) DeleteTransaction(ctx context.Context, userID string, portfolioID string, transactionID string) error {
	pgTx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = pgTx.Rollback(ctx) }()

	var portfolioOwner string
	if err := pgTx.QueryRow(ctx, `SELECT user_id FROM portfolios WHERE id=$1`, portfolioID).Scan(&portfolioOwner); err != nil {
		return err
	}
	if portfolioOwner != userID {
		return errors.New("portfolio not found")
	}

	var assetID string
	if err := pgTx.QueryRow(ctx, `
		DELETE FROM transactions
		WHERE id=$1 AND portfolio_id=$2
		RETURNING asset_id
	`, transactionID, portfolioID).Scan(&assetID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return errors.New("transaction not found")
		}
		return err
	}

	if err := r.recalculatePosition(ctx, pgTx, portfolioID, assetID); err != nil {
		return err
	}

	return pgTx.Commit(ctx)
}

func (r *PortfolioRepository) recalculatePosition(ctx context.Context, tx pgx.Tx, portfolioID string, assetID string) error {
	rows, err := tx.Query(ctx, `
		SELECT transaction_type, quantity, unit_price, fee_amount
		FROM transactions
		WHERE portfolio_id=$1 AND asset_id=$2
		ORDER BY transaction_at ASC, created_at ASC, id ASC
	`, portfolioID, assetID)
	if err != nil {
		return err
	}
	defer rows.Close()

	var qty float64
	var avg float64
	for rows.Next() {
		var txType string
		var q, price, fee float64
		if err := rows.Scan(&txType, &q, &price, &fee); err != nil {
			return err
		}
		switch txType {
		case "buy":
			newQty := qty + q
			if newQty > 0 {
				avg = ((qty * avg) + (q * price) + fee) / newQty
			}
			qty = newQty
		case "sell":
			if q > qty+0.00000001 {
				return errors.New("sell quantity exceeds current position after edit")
			}
			qty -= q
			if math.Abs(qty) < 0.00000001 {
				qty, avg = 0, 0
			}
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	if qty <= 0 {
		_, err = tx.Exec(ctx, `DELETE FROM portfolio_positions WHERE portfolio_id=$1 AND asset_id=$2`, portfolioID, assetID)
		return err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO portfolio_positions (portfolio_id, asset_id, quantity, avg_buy_price)
		VALUES ($1,$2,$3,$4)
		ON CONFLICT (portfolio_id, asset_id) DO UPDATE SET quantity=EXCLUDED.quantity, avg_buy_price=EXCLUDED.avg_buy_price, updated_at=NOW()
	`, portfolioID, assetID, qty, avg)
	return err
}

func (r *PortfolioRepository) ListTransactions(ctx context.Context, userID string, portfolioID string) ([]models.Transaction, error) {
	rows, err := r.db.Query(ctx, `
		SELECT t.id, t.portfolio_id, t.asset_id, a.ticker, a.name, t.transaction_type, t.quantity, t.unit_price, t.fee_amount, t.total_amount, t.transaction_at, COALESCE(t.note,''), t.created_at
		FROM transactions t
		JOIN portfolios p ON p.id=t.portfolio_id
		JOIN assets a ON a.id=t.asset_id
		WHERE t.portfolio_id=$1 AND p.user_id=$2
		ORDER BY t.transaction_at DESC, t.created_at DESC
	`, portfolioID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]models.Transaction, 0)
	for rows.Next() {
		var tx models.Transaction
		if err := rows.Scan(&tx.ID, &tx.PortfolioID, &tx.AssetID, &tx.Ticker, &tx.AssetName, &tx.TransactionType, &tx.Quantity, &tx.UnitPrice, &tx.FeeAmount, &tx.TotalAmount, &tx.TransactionAt, &tx.Note, &tx.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, tx)
	}
	return items, rows.Err()
}

func (r *PortfolioRepository) ListPositions(ctx context.Context, userID string, portfolioID string) ([]models.Position, error) {
	rows, err := r.db.Query(ctx, `
		WITH latest_prices AS (
			SELECT DISTINCT ON (asset_id) asset_id, close_price, recorded_at
			FROM price_history
			ORDER BY asset_id, recorded_at DESC
		),
		prev_24h AS (
			SELECT DISTINCT ON (ph.asset_id) ph.asset_id, ph.close_price
			FROM price_history ph
			JOIN latest_prices lp ON lp.asset_id = ph.asset_id
			WHERE ph.recorded_at <= lp.recorded_at - INTERVAL '24 hours'
			ORDER BY ph.asset_id, ph.recorded_at DESC
		),
		prev_any AS (
			SELECT asset_id, close_price
			FROM (
				SELECT asset_id, close_price, ROW_NUMBER() OVER (PARTITION BY asset_id ORDER BY recorded_at DESC) rn
				FROM price_history
			) x
			WHERE rn = 2
		)
		SELECT
			pp.id,
			pp.portfolio_id,
			pp.asset_id,
			a.ticker,
			a.name,
			a.currency_code,
			pp.quantity,
			pp.avg_buy_price,
			COALESCE(lp.close_price, pp.avg_buy_price) AS current_price,
			CASE
				WHEN COALESCE(p24.close_price, pa.close_price) > 0 THEN
					(COALESCE(lp.close_price, pp.avg_buy_price) - COALESCE(p24.close_price, pa.close_price)) / COALESCE(p24.close_price, pa.close_price) * 100
				ELSE NULL
			END AS change_24h_percent
		FROM portfolio_positions pp
		JOIN portfolios p ON p.id=pp.portfolio_id
		JOIN assets a ON a.id=pp.asset_id
		LEFT JOIN latest_prices lp ON lp.asset_id=pp.asset_id
		LEFT JOIN prev_24h p24 ON p24.asset_id=pp.asset_id
		LEFT JOIN prev_any pa ON pa.asset_id=pp.asset_id
		WHERE pp.portfolio_id=$1 AND p.user_id=$2 AND pp.quantity > 0
		ORDER BY a.ticker
	`, portfolioID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]models.Position, 0)
	for rows.Next() {
		var pos models.Position
		if err := rows.Scan(&pos.ID, &pos.PortfolioID, &pos.AssetID, &pos.Ticker, &pos.AssetName, &pos.CurrencyCode, &pos.Quantity, &pos.AvgBuyPrice, &pos.CurrentPrice, &pos.Change24hPercent); err != nil {
			return nil, err
		}
		pos.InvestedValue = pos.Quantity * pos.AvgBuyPrice
		pos.CurrentValue = pos.Quantity * pos.CurrentPrice
		pos.ProfitLoss = pos.CurrentValue - pos.InvestedValue
		if pos.InvestedValue > 0 {
			pos.ProfitPercent = pos.ProfitLoss / pos.InvestedValue * 100
		}
		items = append(items, pos)
	}
	return items, rows.Err()
}

func (r *PortfolioRepository) Summary(ctx context.Context, userID string, portfolioID string) (*models.PortfolioSummary, error) {
	p, err := r.GetByIDForUser(ctx, portfolioID, userID)
	if err != nil {
		return nil, err
	}
	positions, err := r.ListPositions(ctx, userID, portfolioID)
	if err != nil {
		return nil, err
	}
	s := &models.PortfolioSummary{PortfolioID: portfolioID, BaseCurrency: p.BaseCurrency, Positions: positions, PositionsCount: len(positions)}
	for _, pos := range positions {
		s.InvestedValue += pos.InvestedValue
		s.CurrentValue += pos.CurrentValue
	}
	s.ProfitLoss = s.CurrentValue - s.InvestedValue
	if s.InvestedValue > 0 {
		s.ProfitPercent = s.ProfitLoss / s.InvestedValue * 100
	}
	return s, nil
}
