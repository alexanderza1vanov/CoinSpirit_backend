package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/example/invest-portfolio-platform/backend/internal/cache"
	"github.com/example/invest-portfolio-platform/backend/internal/models"
	"github.com/example/invest-portfolio-platform/backend/internal/repository"
)

type Service struct {
	assets *repository.AssetRepository
	prices *repository.PriceRepository
	cache  *cache.RedisCache
	cmc    *CoinMarketCapClient
	moex   *MOEXClient
	gecko  *CoinGeckoClient
	ttl    time.Duration
}

func NewService(assets *repository.AssetRepository, prices *repository.PriceRepository, cache *cache.RedisCache, cfg Config) *Service {
	return &Service{
		assets: assets,
		prices: prices,
		cache:  cache,
		cmc:    NewCoinMarketCapClient(cfg.CoinMarketCapAPIKey, cfg.CoinMarketCapBaseURL),
		moex:   NewMOEXClient(cfg.MOEXISSBaseURL),
		gecko:  NewCoinGeckoClient(""),
		ttl:    cfg.CacheTTL,
	}
}

func (s *Service) RefreshPrice(ctx context.Context, assetID string) (*models.AssetPrice, string, error) {
	asset, err := s.assets.Get(ctx, assetID)
	if err != nil {
		return nil, "", err
	}
	key := fmt.Sprintf("market:last_price:%s", asset.ID)
	price, err := s.fetchExternal(ctx, *asset)
	if err == nil && price != nil && price.Price > 0 {
		s.cachePrice(ctx, key, price)
		_ = s.prices.SaveCandle(ctx, asset.ID, "1d", price.Price, price.Price, price.Price, price.Price, 0, price.UpdatedAt)
		return price, "external", nil
	}
	fallback, fallbackErr := s.prices.LatestCandle(ctx, asset.ID, "1d")
	if fallbackErr != nil {
		return nil, "", err
	}
	price = &models.AssetPrice{AssetID: asset.ID, Ticker: asset.Ticker, Price: fallback.ClosePrice, Currency: asset.CurrencyCode, Source: "postgres_fallback", UpdatedAt: fallback.RecordedAt}
	s.cachePrice(ctx, key, price)
	return price, "postgres_fallback", nil
}

func (s *Service) LatestPrice(ctx context.Context, assetID string) (*models.AssetPrice, string, error) {
	asset, err := s.assets.Get(ctx, assetID)
	if err != nil {
		return nil, "", err
	}
	key := fmt.Sprintf("market:last_price:%s", asset.ID)
	if cached, ok := s.cache.Get(ctx, key); ok {
		var price models.AssetPrice
		if json.Unmarshal([]byte(cached), &price) == nil {
			return &price, "redis", nil
		}
	}
	price, err := s.fetchExternal(ctx, *asset)
	if err == nil && price != nil && price.Price > 0 {
		s.cachePrice(ctx, key, price)
		_ = s.prices.SaveCandle(ctx, asset.ID, "1d", price.Price, price.Price, price.Price, price.Price, 0, price.UpdatedAt)
		return price, "external", nil
	}
	fallback, fallbackErr := s.prices.LatestCandle(ctx, asset.ID, "1d")
	if fallbackErr != nil {
		return nil, "", err
	}
	price = &models.AssetPrice{AssetID: asset.ID, Ticker: asset.Ticker, Price: fallback.ClosePrice, Currency: asset.CurrencyCode, Source: "postgres_fallback", UpdatedAt: fallback.RecordedAt}
	s.cachePrice(ctx, key, price)
	return price, "postgres_fallback", nil
}

func (s *Service) fetchExternal(ctx context.Context, asset models.Asset) (*models.AssetPrice, error) {
	switch strings.ToLower(asset.MarketType) {
	case "crypto":
		return s.cmc.LatestPrice(ctx, asset)
	case "moex":
		price, err := s.moex.LatestPrice(ctx, asset)
		if synced, syncErr := s.moexSyncedPriceFromCandles(ctx, asset, price); syncErr == nil && synced != nil && synced.Price > 0 {
			return synced, nil
		}
		return price, err
	default:
		return nil, fmt.Errorf("unsupported market_type %s", asset.MarketType)
	}
}

func (s *Service) moexSyncedPriceFromCandles(ctx context.Context, asset models.Asset, price *models.AssetPrice) (*models.AssetPrice, error) {
	candles, err := s.moex.Candles(ctx, asset, "1d")
	if err != nil || len(candles) == 0 {
		return price, err
	}
	last := candles[len(candles)-1]
	if last.ClosePrice <= 0 {
		return price, nil
	}
	if price == nil {
		price = &models.AssetPrice{AssetID: asset.ID, Ticker: asset.Ticker, Currency: defaultString(asset.CurrencyCode, "RUB"), Source: "moex_candles", UpdatedAt: last.RecordedAt}
	}
	// MOEX marketdata LAST can lag behind the latest candle. Use the candle close
	// as the unified current price when it materially differs, so portfolio,
	// position screen and analytics all show one price.
	diffRatio := 1.0
	if price.Price > 0 {
		diffRatio = math.Abs(last.ClosePrice-price.Price) / price.Price
	}
	if price.Price <= 0 || diffRatio > 0.0005 {
		price.Price = last.ClosePrice
		price.Source = "moex_candles_last"
		price.UpdatedAt = last.RecordedAt
	}
	if price.Currency == "" {
		price.Currency = defaultString(asset.CurrencyCode, "RUB")
	}
	if price.Change24hPercent == nil || (price.Change24hPercent != nil && *price.Change24hPercent == 0 && len(candles) > 1) {
		base := candles[0].ClosePrice
		if base > 0 {
			change := (last.ClosePrice - base) / base * 100
			price.Change24hPercent = &change
		}
	}
	return price, nil
}

func (s *Service) HistoricalCandles(ctx context.Context, asset models.Asset, timeframe string) ([]models.PriceCandle, string, error) {
	switch strings.ToLower(asset.MarketType) {
	case "moex":
		candles, err := s.moex.Candles(ctx, asset, timeframe)
		return candles, "moex", err
	case "crypto":
		candles, err := s.gecko.MarketChart(ctx, asset, timeframe)
		return candles, "coingecko", err
	default:
		return nil, "", fmt.Errorf("unsupported market_type %s", asset.MarketType)
	}
}

func normalizeMarketTimeframe(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	switch v {
	case "1d", "1w", "1m", "3m", "1y":
		return v
	case "":
		return "1d"
	default:
		return "1d"
	}
}

func (s *Service) cachePrice(ctx context.Context, key string, price *models.AssetPrice) {
	if s.ttl <= 0 {
		s.ttl = 60 * time.Second
	}
	b, err := json.Marshal(price)
	if err == nil {
		s.cache.Set(ctx, key, string(b), s.ttl)
	}
}
