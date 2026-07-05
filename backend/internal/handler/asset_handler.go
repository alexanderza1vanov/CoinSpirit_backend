package handler

import (
	"net/http"
	"time"

	"github.com/example/invest-portfolio-platform/backend/internal/cache"
	"github.com/example/invest-portfolio-platform/backend/internal/config"
	"github.com/example/invest-portfolio-platform/backend/internal/marketdata"
	"github.com/example/invest-portfolio-platform/backend/internal/models"
	"github.com/example/invest-portfolio-platform/backend/internal/repository"
	"github.com/jackc/pgx/v5/pgxpool"
)

func RegisterAssetRoutes(router *http.ServeMux, pool *pgxpool.Pool, cfg config.Config) {
	assets := repository.NewAssetRepository(pool)
	prices := repository.NewPriceRepository(pool)
	redisCache := cache.NewRedisCache(cfg.RedisAddr)
	market := marketdata.NewService(assets, prices, redisCache, marketdata.Config{
		CoinMarketCapAPIKey:  cfg.CoinMarketCapAPIKey,
		CoinMarketCapBaseURL: cfg.CoinMarketCapBaseURL,
		MOEXISSBaseURL:       cfg.MOEXISSBaseURL,
		CacheTTL:             time.Duration(cfg.MarketDataCacheTTLSeconds) * time.Second,
	})

	enrichAssets := func(r *http.Request, items []models.Asset) []models.Asset {
		for i := range items {
			price, _, err := market.LatestPrice(r.Context(), items[i].ID)
			if err != nil || price == nil || price.Price <= 0 {
				continue
			}
			items[i].CurrentPrice = &price.Price
			items[i].Change24hPercent = price.Change24hPercent
		}
		return items
	}

	router.HandleFunc("GET /assets", func(w http.ResponseWriter, r *http.Request) {
		items, err := assets.List(r.Context(), r.URL.Query().Get("q"))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load assets")
			return
		}
		writeJSON(w, http.StatusOK, enrichAssets(r, items))
	})
	router.HandleFunc("GET /assets/search", func(w http.ResponseWriter, r *http.Request) {
		items, err := assets.List(r.Context(), r.URL.Query().Get("q"))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to search assets")
			return
		}
		writeJSON(w, http.StatusOK, enrichAssets(r, items))
	})
	router.HandleFunc("GET /assets/{id}", func(w http.ResponseWriter, r *http.Request) {
		item, err := assets.Get(r.Context(), r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusNotFound, "asset not found")
			return
		}
		if price, _, err := market.LatestPrice(r.Context(), item.ID); err == nil && price != nil && price.Price > 0 {
			item.CurrentPrice = &price.Price
			item.Change24hPercent = price.Change24hPercent
		}
		writeJSON(w, http.StatusOK, item)
	})
	router.HandleFunc("GET /assets/{id}/price-history", func(w http.ResponseWriter, r *http.Request) {
		items, err := assets.PriceHistory(r.Context(), r.PathValue("id"), r.URL.Query().Get("timeframe"))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load price history")
			return
		}
		writeJSON(w, http.StatusOK, items)
	})
}
