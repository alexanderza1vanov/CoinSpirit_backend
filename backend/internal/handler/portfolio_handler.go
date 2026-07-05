package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/example/invest-portfolio-platform/backend/internal/config"
	"github.com/example/invest-portfolio-platform/backend/internal/marketdata"
	"github.com/example/invest-portfolio-platform/backend/internal/middleware"
	"github.com/example/invest-portfolio-platform/backend/internal/repository"
	"github.com/jackc/pgx/v5/pgxpool"
)

type createPortfolioRequest struct {
	Name         string `json:"name"`
	BaseCurrency string `json:"base_currency"`
	Description  string `json:"description"`
}

type createTransactionRequest struct {
	AssetID         string  `json:"asset_id"`
	TransactionType string  `json:"transaction_type"`
	Quantity        float64 `json:"quantity"`
	UnitPrice       float64 `json:"unit_price"`
	FeeAmount       float64 `json:"fee_amount"`
	TransactionAt   string  `json:"transaction_at"`
	Note            string  `json:"note"`
}

func RegisterPortfolioRoutes(router *http.ServeMux, pool *pgxpool.Pool, cfg config.Config, marketSvc *marketdata.Service) {
	portfolios := repository.NewPortfolioRepository(pool)

	router.Handle("GET /portfolios", middleware.JWTAuth(cfg.JWTAccessSecret, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := middleware.ClaimsFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "missing claims")
			return
		}
		items, err := portfolios.ListByUser(r.Context(), claims.UserID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load portfolios")
			return
		}
		writeJSON(w, http.StatusOK, items)
	})))

	router.Handle("POST /portfolios", middleware.JWTAuth(cfg.JWTAccessSecret, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := middleware.ClaimsFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "missing claims")
			return
		}
		var req createPortfolioRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		if req.Name == "" {
			writeError(w, http.StatusBadRequest, "name is required")
			return
		}
		item, err := portfolios.Create(r.Context(), claims.UserID, req.Name, req.BaseCurrency, req.Description)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to create portfolio")
			return
		}
		writeJSON(w, http.StatusCreated, item)
	})))

	router.Handle("GET /portfolios/{id}", middleware.JWTAuth(cfg.JWTAccessSecret, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := middleware.ClaimsFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "missing claims")
			return
		}
		item, err := portfolios.GetByIDForUser(r.Context(), r.PathValue("id"), claims.UserID)
		if err != nil {
			writeError(w, http.StatusNotFound, "portfolio not found")
			return
		}
		writeJSON(w, http.StatusOK, item)
	})))

	router.Handle("GET /portfolios/{id}/positions", middleware.JWTAuth(cfg.JWTAccessSecret, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := middleware.ClaimsFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "missing claims")
			return
		}
		refreshPortfolioMarketPrices(r.Context(), pool, claims.UserID, r.PathValue("id"), marketSvc)
		items, err := portfolios.ListPositions(r.Context(), claims.UserID, r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load positions")
			return
		}
		writeJSON(w, http.StatusOK, items)
	})))

	router.Handle("GET /portfolios/{id}/summary", middleware.JWTAuth(cfg.JWTAccessSecret, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := middleware.ClaimsFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "missing claims")
			return
		}
		refreshPortfolioMarketPrices(r.Context(), pool, claims.UserID, r.PathValue("id"), marketSvc)
		item, err := portfolios.Summary(r.Context(), claims.UserID, r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusNotFound, "portfolio not found")
			return
		}
		writeJSON(w, http.StatusOK, item)
	})))

	router.Handle("GET /portfolios/{id}/transactions", middleware.JWTAuth(cfg.JWTAccessSecret, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := middleware.ClaimsFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "missing claims")
			return
		}
		items, err := portfolios.ListTransactions(r.Context(), claims.UserID, r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load transactions")
			return
		}
		writeJSON(w, http.StatusOK, items)
	})))

	router.Handle("POST /portfolios/{id}/transactions", middleware.JWTAuth(cfg.JWTAccessSecret, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := middleware.ClaimsFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "missing claims")
			return
		}
		var req createTransactionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		when := time.Now()
		if req.TransactionAt != "" {
			parsed, err := time.Parse(time.RFC3339, req.TransactionAt)
			if err != nil {
				writeError(w, http.StatusBadRequest, "transaction_at must be RFC3339")
				return
			}
			when = parsed
		}
		item, err := portfolios.AddTransaction(r.Context(), claims.UserID, r.PathValue("id"), req.AssetID, req.TransactionType, req.Quantity, req.UnitPrice, req.FeeAmount, when, req.Note)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, item)
	})))

	router.Handle("PUT /portfolios/{id}/transactions/{transactionId}", middleware.JWTAuth(cfg.JWTAccessSecret, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := middleware.ClaimsFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "missing claims")
			return
		}
		var req createTransactionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		when := time.Now()
		if req.TransactionAt != "" {
			parsed, err := time.Parse(time.RFC3339, req.TransactionAt)
			if err != nil {
				writeError(w, http.StatusBadRequest, "transaction_at must be RFC3339")
				return
			}
			when = parsed
		}
		item, err := portfolios.UpdateTransaction(r.Context(), claims.UserID, r.PathValue("id"), r.PathValue("transactionId"), req.TransactionType, req.Quantity, req.UnitPrice, req.FeeAmount, when, req.Note)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, item)
	})))

	router.Handle("DELETE /portfolios/{id}/transactions/{transactionId}", middleware.JWTAuth(cfg.JWTAccessSecret, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := middleware.ClaimsFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "missing claims")
			return
		}
		if err := portfolios.DeleteTransaction(r.Context(), claims.UserID, r.PathValue("id"), r.PathValue("transactionId")); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})))

}

func refreshPortfolioMarketPrices(ctx context.Context, pool *pgxpool.Pool, userID string, portfolioID string, marketSvc *marketdata.Service) {
	if marketSvc == nil {
		return
	}
	rows, err := pool.Query(ctx, `
		SELECT DISTINCT pp.asset_id
		FROM portfolio_positions pp
		JOIN portfolios p ON p.id = pp.portfolio_id
		WHERE pp.portfolio_id = $1 AND p.user_id = $2 AND pp.quantity > 0
	`, portfolioID, userID)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var assetID string
		if err := rows.Scan(&assetID); err == nil {
			_, _, _ = marketSvc.RefreshPrice(ctx, assetID)
		}
	}
}
