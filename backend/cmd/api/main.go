package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/example/invest-portfolio-platform/backend/internal/cache"
	"github.com/example/invest-portfolio-platform/backend/internal/config"
	"github.com/example/invest-portfolio-platform/backend/internal/database"
	"github.com/example/invest-portfolio-platform/backend/internal/events"
	"github.com/example/invest-portfolio-platform/backend/internal/handler"
	"github.com/example/invest-portfolio-platform/backend/internal/marketdata"
	"github.com/example/invest-portfolio-platform/backend/internal/middleware"
	"github.com/example/invest-portfolio-platform/backend/internal/repository"
	"github.com/example/invest-portfolio-platform/backend/internal/worker"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := config.Load()

	pool, err := database.NewPostgresPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("postgres connection failed: %v", err)
	}
	defer pool.Close()

	redisCache := cache.NewRedisCache(cfg.RedisAddr)
	defer redisCache.Close()

	assetsRepo := repository.NewAssetRepository(pool)
	priceRepo := repository.NewPriceRepository(pool)
	marketSvc := marketdata.NewService(assetsRepo, priceRepo, redisCache, marketdata.Config{
		CoinMarketCapAPIKey:  cfg.CoinMarketCapAPIKey,
		CoinMarketCapBaseURL: cfg.CoinMarketCapBaseURL,
		MOEXISSBaseURL:       cfg.MOEXISSBaseURL,
		CacheTTL:             time.Duration(cfg.MarketDataCacheTTLSeconds) * time.Second,
	})

	router := http.NewServeMux()
	handler.RegisterHealthRoutes(router)
	handler.RegisterAuthRoutes(router, pool, cfg)
	handler.RegisterPortfolioRoutes(router, pool, cfg, marketSvc)
	handler.RegisterAssetRoutes(router, pool, cfg)
	handler.RegisterNewsRoutes(router, cfg, redisCache)
	handler.RegisterAnalyticsRoutes(router, pool, cfg, redisCache)
	handler.RegisterAlertRoutes(router, pool, cfg, redisCache)

	var eventProducer *events.Producer
	if cfg.KafkaEnabled {
		eventProducer = events.NewProducer(cfg.KafkaBrokers, cfg.KafkaClientID)
		go events.NewAlertConsumer(pool, cfg, eventProducer).Start(ctx)
		go events.NewNotificationConsumer(pool, cfg, eventProducer).Start(ctx)
		log.Printf("kafka event pipeline enabled: brokers=%v", cfg.KafkaBrokers)
	} else {
		log.Println("kafka event pipeline disabled: using direct background alert checks")
	}

	go worker.NewMarketWorker(pool, cfg, marketSvc, eventProducer).Start(ctx)

	server := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.HTTPPort),
		Handler:      middleware.Logging(middleware.CORS(router, cfg.CORSAllowedOrigins)),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("API started on :%s", cfg.HTTPPort)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Printf("server stopped with error: %v", err)
		os.Exit(1)
	}
}
