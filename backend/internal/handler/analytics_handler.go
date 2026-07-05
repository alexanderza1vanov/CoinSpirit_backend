package handler

import (
	"net/http"
	"time"

	"github.com/example/invest-portfolio-platform/backend/internal/analytics"
	"github.com/example/invest-portfolio-platform/backend/internal/cache"
	"github.com/example/invest-portfolio-platform/backend/internal/config"
	"github.com/example/invest-portfolio-platform/backend/internal/marketdata"
	"github.com/example/invest-portfolio-platform/backend/internal/repository"
	"github.com/jackc/pgx/v5/pgxpool"
)

func RegisterAnalyticsRoutes(router *http.ServeMux, pool *pgxpool.Pool, cfg config.Config, redisCache *cache.RedisCache) {
	assets := repository.NewAssetRepository(pool)
	prices := repository.NewPriceRepository(pool)
	market := marketdata.NewService(assets, prices, redisCache, marketdata.Config{
		CoinMarketCapAPIKey:  cfg.CoinMarketCapAPIKey,
		CoinMarketCapBaseURL: cfg.CoinMarketCapBaseURL,
		MOEXISSBaseURL:       cfg.MOEXISSBaseURL,
		CacheTTL:             time.Duration(cfg.MarketDataCacheTTLSeconds) * time.Second,
	})
	analyticsService := analytics.NewService(assets, prices, market)

	router.HandleFunc("GET /assets/{id}/price", func(w http.ResponseWriter, r *http.Request) {
		price, _, err := market.LatestPrice(r.Context(), r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load asset price")
			return
		}
		writeJSON(w, http.StatusOK, price)
	})

	router.HandleFunc("GET /analytics/assets/{id}/chart", func(w http.ResponseWriter, r *http.Request) {
		items, err := analyticsService.Chart(r.Context(), r.PathValue("id"), r.URL.Query().Get("timeframe"))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to build chart")
			return
		}
		writeJSON(w, http.StatusOK, items)
	})

	router.HandleFunc("GET /analytics/assets/{id}/indicators", func(w http.ResponseWriter, r *http.Request) {
		items, err := analyticsService.Indicators(r.Context(), r.PathValue("id"), r.URL.Query().Get("timeframe"))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to calculate indicators")
			return
		}
		writeJSON(w, http.StatusOK, items)
	})

	router.HandleFunc("GET /analytics/assets/{id}/overview", func(w http.ResponseWriter, r *http.Request) {
		item, err := analyticsService.Overview(r.Context(), r.PathValue("id"), r.URL.Query().Get("timeframe"))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to build analytics overview")
			return
		}
		writeJSON(w, http.StatusOK, item)
	})
}
